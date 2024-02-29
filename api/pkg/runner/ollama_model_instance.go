package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/jmorganca/ollama/api"
	"github.com/jmorganca/ollama/format"
	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

var (
	_ ModelInstance = &OllamaModelInstance{}
)

func NewOllamaModelInstance(ctx context.Context, cfg *ModelInstanceConfig) (*OllamaModelInstance, error) {
	if cfg.InitialSession.LoraDir != "" {
		// TODO: prepare model adapter
		log.Warn().Msg("LoraDir is not supported for OllamaModelInstance, need to implement adapter modelfile")
	}

	aiModel, err := model.GetModel(cfg.InitialSession.ModelName)
	if err != nil {
		return nil, err
	}

	i := &OllamaModelInstance{
		ctx:             ctx,
		id:              system.GenerateUUID(),
		finishCh:        make(chan bool),
		workCh:          make(chan *types.Session, 1),
		model:           aiModel,
		responseHandler: cfg.ResponseHandler,
		filter: types.SessionFilter{
			ModelName: cfg.InitialSession.ModelName,
			Mode:      cfg.InitialSession.Mode,
			LoraDir:   cfg.InitialSession.LoraDir,
			Type:      cfg.InitialSession.Type,
		},
		runnerOptions: cfg.RunnerOptions,
		jobHistory:    []*types.SessionSummary{},
		lastActivity:  time.Now(),
	}

	return i, nil
}

type OllamaModelInstance struct {
	id string

	model  model.Model
	filter types.SessionFilter

	runnerOptions RunnerOptions

	finishCh chan bool

	workCh chan *types.Session

	// client is the model client
	client *openai.Client

	ollamaClient *ollamaClient

	// Streaming response handler
	responseHandler func(res *types.RunnerTaskResponse) error

	// we create a cancel context for the running process
	// which is derived from the main runner context
	ctx context.Context

	// the command we are currently executing
	currentCommand *exec.Cmd

	// the session that meant this model booted in the first place
	// used to know which lora type file we should download before
	// trying to start this model's python process
	initialSession *types.Session

	// the session currently running on this model
	currentSession *types.Session

	// if there is a value here - it will be fed into the running python
	// process next - it acts as a buffer for a session we want to run right away
	nextSession *types.Session

	// this is the session that we are preparing to run next
	// if there is a value here - then we return nil
	// because there is a task running (e.g. downloading files)
	// that we need to complete before we want this session to run
	queuedSession *types.Session

	// the timestamp of when this model instance either completed a job
	// or a new job was pulled and allocated
	// we use this timestamp to cleanup non-active model instances
	lastActivity time.Time

	// a history of the session IDs
	jobHistory []*types.SessionSummary
}

func (i *OllamaModelInstance) Start(session *types.Session) error {
	i.initialSession = session

	ollamaPath, err := exec.LookPath("ollama")
	if err != nil {
		return fmt.Errorf("ollama not found in PATH")
	}

	// Get random free port
	port, err := freeport.GetFreePort()
	if err != nil {
		return fmt.Errorf("error getting free port: %s", err.Error())
	}

	config := openai.DefaultConfig("ollama")
	config.BaseURL = fmt.Sprintf("http://localhost:%d/v1", port)

	i.client = openai.NewClientWithConfig(config)

	cmd := exec.CommandContext(i.ctx, ollamaPath, "serve")
	// Getting base env (HOME, etc)
	cmd.Env = append(cmd.Env,
		os.Environ()...,
	)

	ollamaHost := fmt.Sprintf("0.0.0.0:%d", port)

	cmd.Env = append(cmd.Env,
		"HTTP_PROXY="+os.Getenv("HTTP_PROXY"),
		"HTTPS_PROXY="+os.Getenv("HTTPS_PROXY"),
		"OLLAMA_HOST="+ollamaHost,
		"OLLAMA_MODELS="+i.runnerOptions.CacheDir,
	)

	cmd.Stdout = os.Stdout

	// this buffer is so we can keep the last 10kb of stderr so if
	// there is an error we can send it to the api
	stderrBuf := system.NewLimitedBuffer(1024 * 10)

	stderrWriters := []io.Writer{os.Stderr, stderrBuf}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	// stream stderr to os.Stderr (so we can see it in the logs)
	// and also the error buffer we will use to post the error to the api
	go func() {
		_, err := io.Copy(io.MultiWriter(stderrWriters...), stderrPipe)
		if err != nil {
			log.Error().Msgf("Error copying stderr: %v", err)
		}
	}()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting Ollama model instance: %s", err.Error())
	}

	i.currentCommand = cmd

	go func() {
		defer close(i.finishCh)
		if err := cmd.Wait(); err != nil {
			log.Error().Msgf("Ollama model instance exited with error: %s", err.Error())

			errMsg := string(stderrBuf.Bytes())
			if i.currentSession != nil {
				i.errorSession(i.currentSession, fmt.Errorf("%s from cmd - %s", err.Error(), errMsg))
			}

			return
		}

		log.Info().Msgf("🟢 Ollama model instance stopped, exit code=%d", cmd.ProcessState.ExitCode())
	}()

	// Wait for the server to start
	startCtx, cancel := context.WithTimeout(i.ctx, 10*time.Second)
	defer cancel()

	ollamaClient, err := newOllamaClient(ollamaHost)
	if err != nil {
		return fmt.Errorf("error creating Ollama client: %s", err.Error())
	}

	i.ollamaClient = ollamaClient

WAIT:
	for {
		select {
		case <-startCtx.Done():
			return fmt.Errorf("timeout waiting for Ollama model instance to start")
		default:
			resp, err := http.DefaultClient.Get(fmt.Sprintf("http://localhost:%d", port))
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				break WAIT
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// TODO: make this dynamic
	err = i.ollamaClient.Pull(i.ctx, &api.PullRequest{
		Model: string(session.ModelName),
	}, func(progress api.ProgressResponse) error {
		log.Info().Msgf("🟢 Pulling model %s (%d/%d)", session.ModelName, progress.Completed, progress.Total)
		return nil
	})

	if err != nil {
		return fmt.Errorf("error pulling model: %s", err.Error())
	}

	go func() {
		for {
			select {
			case <-i.ctx.Done():
				log.Info().Msgf("🟢 stopping Ollama model instance")
				return
			case session := <-i.workCh:
				i.currentSession = session
				i.lastActivity = time.Now()

				err = i.processInteraction(session)
				if err != nil {
					log.Error().
						Str("session_id", session.ID).
						Err(err).
						Msg("error processing interaction")
				}

				i.currentSession = nil
			}
		}
	}()

	return nil
}

func (i *OllamaModelInstance) Stop() error {
	if i.currentCommand == nil {
		return fmt.Errorf("no Ollama process to stop")
	}
	log.Info().Msgf("🟢 stop Ollama model instance")
	if err := syscall.Kill(-i.currentCommand.Process.Pid, syscall.SIGKILL); err != nil {
		log.Error().Msgf("error stopping Ollama model instance: %s", err.Error())
		return err
	}
	log.Info().Msgf("🟢 stopped Ollama instance")
	return nil
}

func (i *OllamaModelInstance) ID() string {
	return i.id
}

func (i *OllamaModelInstance) Filter() types.SessionFilter {
	return i.filter
}

func (i *OllamaModelInstance) Stale() bool {
	return time.Since(i.lastActivity) > i.runnerOptions.Config.Runtimes.Ollama.InstanceTTL
}

func (i *OllamaModelInstance) Model() model.Model {
	return i.model
}

func (i *OllamaModelInstance) GetState() (*types.ModelInstanceState, error) {
	if i.initialSession == nil {
		return nil, fmt.Errorf("no initial session")
	}
	currentSession := i.currentSession
	if currentSession == nil {
		currentSession = i.queuedSession
	}
	// this can happen when the session has downloaded and is ready
	// but the python is still booting up
	if currentSession == nil {
		currentSession = i.nextSession
	}

	var sessionSummary *types.SessionSummary
	var err error

	if currentSession != nil {
		sessionSummary, err = data.GetSessionSummary(currentSession)
		if err != nil {
			return nil, err
		}
	}

	stale := false
	if i.lastActivity.IsZero() {
		stale = false
	} else if time.Since(i.lastActivity) > i.runnerOptions.Config.Runtimes.Ollama.InstanceTTL {
		stale = true
	}

	return &types.ModelInstanceState{
		ID:               i.id,
		ModelName:        i.initialSession.ModelName,
		Mode:             i.initialSession.Mode,
		LoraDir:          i.initialSession.LoraDir,
		InitialSessionID: i.initialSession.ID,
		CurrentSession:   sessionSummary,
		JobHistory:       i.jobHistory,
		Timeout:          int(i.runnerOptions.Config.Runtimes.Ollama.InstanceTTL.Seconds()),
		LastActivity:     int(i.lastActivity.Unix()),
		Stale:            stale,
		MemoryUsage:      i.model.GetMemoryRequirements(i.initialSession.Mode),
	}, nil
}

func (i *OllamaModelInstance) NextSession() *types.Session {
	return i.nextSession
}

func (i *OllamaModelInstance) AssignSessionTask(ctx context.Context, session *types.Session) (*types.RunnerTask, error) {
	// Noop for ollama model instance, this is only needed for the axolotl model instance
	return &types.RunnerTask{}, nil
}

func (i *OllamaModelInstance) SetNextSession(session *types.Session) {
	i.nextSession = session
}

func (i *OllamaModelInstance) QueueSession(session *types.Session, isInitialSession bool) {
	err := i.addJobToHistory(session)
	if err != nil {
		log.Error().Err(err).Msg("error adding job to history")
	}

	// TODO: for finetuned model serving, this is where
	// the queued session would be set while we download
	// the adapter and load it into the server

	i.queuedSession = nil
	i.nextSession = session

	i.workCh <- session
}

func (i *OllamaModelInstance) processInteraction(session *types.Session) error {
	var messages []openai.ChatCompletionMessage

	// Adjust length
	var interactions []*types.Interaction
	if len(session.Interactions) > 10 {
		first, err := data.GetFirstUserInteraction(session.Interactions)
		if err != nil {
			log.Err(err).Msg("error getting first user interaction")
		} else {
			interactions = append(interactions, first)
			interactions = append(interactions, data.GetLastInteractions(session, 10)...)
		}
	} else {
		interactions = session.Interactions
	}

	if session.Metadata.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: session.Metadata.SystemPrompt,
		})
	}

	for _, interaction := range interactions {
		switch interaction.Creator {
		case types.CreatorTypeUser:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: interaction.Message,
			})
		case types.CreatorTypeSystem:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: interaction.Message,
			})
		}
	}

	// Adding current message
	stream, err := i.client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{
		Model:    string(session.ModelName),
		Stream:   true,
		Messages: messages,
	})
	if err != nil {
		return fmt.Errorf("failed to get response from inference API: %w", err)
	}

	defer stream.Close()

	var buf string

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			log.Info().Msg("stream finished")
			i.responseProcessor(buf, true)
			return nil
		}

		if err != nil {
			log.Error().Err(err).Msg("stream error")
			i.errorSession(session, err)
			return err
		}

		buf += response.Choices[0].Delta.Content

		i.responseProcessor(response.Choices[0].Delta.Content, false)
	}
}

func (i *OllamaModelInstance) responseProcessor(content string, done bool) {
	if i.currentSession == nil {
		log.Error().Msgf("no current session")
		return
	}

	var err error

	systemInteraction, err := data.GetSystemInteraction(i.currentSession)
	if err != nil {
		log.Error().Msgf("error getting system interaction: %s", err.Error())
		return
	}

	resp := &types.RunnerTaskResponse{
		SessionID:     i.currentSession.ID,
		InteractionID: systemInteraction.ID,
		Owner:         i.currentSession.Owner,
		Done:          done,
		Message:       content,
	}

	if done {
		resp.Type = types.WorkerTaskResponseTypeResult
	} else {
		resp.Type = types.WorkerTaskResponseTypeStream
	}

	err = i.responseHandler(resp)
	if err != nil {
		log.Error().Msgf("error writing event: %s", err.Error())
		return
	}
}

func (i *OllamaModelInstance) GetQueuedSession() *types.Session {
	return nil
}

func (i *OllamaModelInstance) Done() <-chan bool {
	return i.finishCh
}

func (i *OllamaModelInstance) addJobToHistory(session *types.Session) error {
	summary, err := data.GetSessionSummary(session)
	if err != nil {
		return err
	}

	// put the job at the start of the array
	i.jobHistory = append([]*types.SessionSummary{summary}, i.jobHistory...)
	if len(i.jobHistory) > i.runnerOptions.JobHistoryBufferSize {
		i.jobHistory = i.jobHistory[:len(i.jobHistory)-1]
	}

	return nil
}

func (i *OllamaModelInstance) errorSession(session *types.Session, err error) {
	apiUpdateErr := i.responseHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: session.ID,
		Error:     err.Error(),
	})

	if apiUpdateErr != nil {
		log.Error().Msgf("Error reporting error to api: %v\n", apiUpdateErr.Error())
	}
}

type ollamaClient struct {
	base *url.URL
	http *http.Client
}

func newOllamaClient(hostport string) (*ollamaClient, error) {
	defaultPort := "11434"

	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host, port = "127.0.0.1", defaultPort
		if ip := net.ParseIP(strings.Trim(hostport, "[]")); ip != nil {
			host = ip.String()
		} else if hostport != "" {
			host = hostport
		}
	}

	return &ollamaClient{
		base: &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(host, port),
		},
		http: http.DefaultClient,
	}, nil
}

func (c *ollamaClient) Pull(ctx context.Context, req *api.PullRequest, fn api.PullProgressFunc) error {
	return c.stream(ctx, http.MethodPost, "/api/pull", req, func(bts []byte) error {
		var resp api.ProgressResponse
		if err := json.Unmarshal(bts, &resp); err != nil {
			return err
		}

		return fn(resp)
	})
}

const maxBufferSize = 512 * format.KiloByte

func (c *ollamaClient) stream(ctx context.Context, method, path string, data any, fn func([]byte) error) error {
	var buf *bytes.Buffer
	if data != nil {
		bts, err := json.Marshal(data)
		if err != nil {
			return err
		}

		buf = bytes.NewBuffer(bts)
	}

	requestURL := c.base.JoinPath(path)
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), buf)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/x-ndjson")

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	// increase the buffer size to avoid running out of space
	scanBuf := make([]byte, 0, maxBufferSize)
	scanner.Buffer(scanBuf, maxBufferSize)
	for scanner.Scan() {
		var errorResponse struct {
			Error string `json:"error,omitempty"`
		}

		bts := scanner.Bytes()
		if err := json.Unmarshal(bts, &errorResponse); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}

		if errorResponse.Error != "" {
			return fmt.Errorf(errorResponse.Error)
		}

		if response.StatusCode >= http.StatusBadRequest {
			return api.StatusError{
				StatusCode:   response.StatusCode,
				Status:       response.Status,
				ErrorMessage: errorResponse.Error,
			}
		}

		if err := fn(bts); err != nil {
			return err
		}
	}

	return nil
}
