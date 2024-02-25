package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

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
	} else {
		cfg.InitialSession.LoraDir = types.LORA_DIR_NONE
	}

	i := &OllamaModelInstance{
		id:              system.GenerateUUID(),
		finishCh:        make(chan bool),
		responseHandler: cfg.ResponseHandler,
		filter: types.SessionFilter{
			ModelName: cfg.InitialSession.ModelName,
			Mode:      cfg.InitialSession.Mode,
			LoraDir:   cfg.InitialSession.LoraDir,
			Type:      cfg.InitialSession.Type,
		},
		runnerOptions: cfg.RunnerOptions,
	}

	return i, nil
}

type OllamaModelInstance struct {
	id string

	model  model.Model
	filter types.SessionFilter

	runnerOptions RunnerOptions

	finishCh chan bool

	// client is the model client
	client *openai.Client

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
	lastActivityTimestamp int64

	// a history of the session IDs
	jobHistory []*types.SessionSummary
}

func (i *OllamaModelInstance) Start(session *types.Session) error {
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

	cmd := exec.Command(ollamaPath)
	cmd.Env = []string{
		"HTTP_PROXY=" + os.Getenv("HTTP_PROXY"),
		"HTTPS_PROXY=" + os.Getenv("HTTPS_PROXY"),
		"OLLAMA_HOST=" + fmt.Sprintf("0.0.0.0:%d", port),
		"OLLAMA_MODELS=" + i.runnerOptions.CacheDir,
	}

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

func (i *OllamaModelInstance) LastActivityTimestamp() int64 {
	return i.lastActivityTimestamp
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
	if i.lastActivityTimestamp == 0 {
		stale = false
	} else if i.lastActivityTimestamp+int64(i.runnerOptions.ModelInstanceTimeoutSeconds) < time.Now().Unix() {
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
		Timeout:          int(i.runnerOptions.ModelInstanceTimeoutSeconds),
		LastActivity:     int(i.lastActivityTimestamp),
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
	i.currentSession = session
	i.lastActivityTimestamp = time.Now().Unix()

	err = i.processInteraction(session)
	if err != nil {
		log.Error().
			Str("session_id", session.ID).
			Err(err).
			Msg("error processing interaction")
	}

	i.currentSession = nil
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

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			log.Info().Msg("stream finished")
			i.responseProcessor(response.Choices[0].Delta.Content, true)
			return nil
		}

		if err != nil {
			log.Error().Err(err).Msg("stream error")
			i.errorSession(session, err)
			return err
		}

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
