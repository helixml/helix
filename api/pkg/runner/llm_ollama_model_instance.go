package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/jmorganca/ollama/api"
	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

type InferenceModelInstanceConfig struct {
	RunnerOptions RunnerOptions

	// Get next chat completion request
	GetNextRequest func() (*types.RunnerLLMInferenceRequest, error)

	// Response writer
	ResponseHandler func(res *types.RunnerTaskResponse) error
}

var (
	_ ModelInstance = &OllamaInferenceModelInstance{}
)

func NewOllamaInferenceModelInstance(ctx context.Context, cfg *InferenceModelInstanceConfig, request *types.RunnerLLMInferenceRequest) (*OllamaInferenceModelInstance, error) {
	modelName := types.ModelName(request.Request.Model)

	aiModel, err := model.GetModel(modelName)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	i := &OllamaInferenceModelInstance{
		ctx:             ctx,
		cancel:          cancel,
		id:              system.GenerateUUID(),
		finishCh:        make(chan bool),
		workCh:          make(chan *types.RunnerLLMInferenceRequest, 1),
		model:           aiModel,
		modelName:       modelName,
		initialRequest:  request,
		responseHandler: cfg.ResponseHandler,
		getNextRequest:  cfg.GetNextRequest,
		runnerOptions:   cfg.RunnerOptions,
		jobHistory:      []*types.SessionSummary{},
		lastActivity:    time.Now(),
	}

	// Enqueue the first request
	go func() {
		i.workCh <- request
	}()

	return i, nil
}

type OllamaInferenceModelInstance struct {
	id string

	model     model.Model
	modelName types.ModelName

	runnerOptions RunnerOptions

	finishCh chan bool

	workCh chan *types.RunnerLLMInferenceRequest

	// client is the model client
	client *openai.Client

	ollamaClient *ollamaClient

	// Streaming response handler
	responseHandler func(res *types.RunnerTaskResponse) error

	// Pulls the next session from the API
	getNextRequest func() (*types.RunnerLLMInferenceRequest, error)

	// we create a cancel context for the running process
	// which is derived from the main runner context
	ctx    context.Context
	cancel context.CancelFunc

	// the command we are currently executing
	currentCommand *exec.Cmd

	// the request that meant this model booted in the first place
	initialRequest *types.RunnerLLMInferenceRequest

	// // the session currently running on this model
	currentRequest *types.RunnerLLMInferenceRequest

	// the timestamp of when this model instance either completed a job
	// or a new job was pulled and allocated
	// we use this timestamp to cleanup non-active model instances
	lastActivity time.Time

	// a history of the session IDs
	jobHistory []*types.SessionSummary
}

// Warmup starts Ollama server and pulls the models
func (i *OllamaInferenceModelInstance) Warmup(ctx context.Context) error {
	err := i.startOllamaServer(ctx)
	if err != nil {
		return err
	}

	return i.warmup(ctx)
}

func (i *OllamaInferenceModelInstance) Start(ctx context.Context) error {
	err := i.startOllamaServer(ctx)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-i.ctx.Done():
				log.Info().Msgf("🟢 Ollama model instance has stopped, closing channel listener")
				return
			case req, ok := <-i.workCh:
				if !ok {
					log.Info().Msg("🟢 workCh closed, exiting")
					return
				}
				log.Info().Str("session_id", req.SessionID).Msg("🟢 processing request")

				i.currentRequest = req
				i.lastActivity = time.Now()

				err := i.processInteraction(req)
				if err != nil {
					log.Error().
						Str("session_id", req.SessionID).
						Err(err).
						Msg("error processing request")
					i.errorSession(req, err)
					if strings.Contains(err.Error(), "connection refused") {
						log.Error().Msg("detected connection refused, exiting and hoping we get restarted - see https://github.com/helixml/helix/issues/242")
						os.Exit(1)
					}
				} else {
					log.Info().
						Str("session_id", req.SessionID).
						Bool("stream", req.Request.Stream).
						Msg("🟢 request processed")
				}

				i.currentRequest = nil
			default:
				// Get next chat request
				req, err := i.getNextRequest()
				if err != nil {
					log.Error().Err(err).Msg("error getting next request")
					time.Sleep(300 * time.Millisecond)
					continue
				}

				if req == nil {
					log.Trace().Msg("no next request")
					time.Sleep(300 * time.Millisecond)
					continue
				}

				log.Info().Str("session_id", req.SessionID).Msg("🟢 enqueuing request")

				i.workCh <- req
			}
		}
	}()

	return nil
}

func (i *OllamaInferenceModelInstance) warmup(ctx context.Context) error {
	var err error
	var wg sync.WaitGroup

	wg.Add(len(i.runnerOptions.Config.Runtimes.Ollama.WarmupModels))

	for _, modelName := range i.runnerOptions.Config.Runtimes.Ollama.WarmupModels {
		go func(modelName string) {
			defer wg.Done()

			log.Info().Msgf("🟢 Pulling model %s", modelName)

			err = i.ollamaClient.Pull(i.ctx, &api.PullRequest{
				Model: modelName,
			}, func(progress api.ProgressResponse) error {
				log.Info().Msgf("🟢 Pulling model %s (%d/%d)", modelName, progress.Completed, progress.Total)
				return nil
			})

			if err != nil {
				log.Error().Msgf("error pulling model: %s", err.Error())
				return
			}

			log.Info().Msgf("🟢 Model '%s' pulled", modelName)

		}(modelName)
	}

	if err != nil {
		return fmt.Errorf("error pulling model: %s", err.Error())
	}

	return nil
}

func (i *OllamaInferenceModelInstance) startOllamaServer(ctx context.Context) error {
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
		"OLLAMA_KEEP_ALIVE=-1",
		"HTTP_PROXY="+os.Getenv("HTTP_PROXY"),
		"HTTPS_PROXY="+os.Getenv("HTTPS_PROXY"),
		"OLLAMA_HOST="+ollamaHost,                 // Bind on localhost with random port
		"OLLAMA_MODELS="+i.runnerOptions.CacheDir, // Where to store the models
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
			if i.currentRequest != nil {
				i.errorSession(i.currentRequest, fmt.Errorf("%s from cmd - %s", err.Error(), errMsg))
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

	return nil
}

func (i *OllamaInferenceModelInstance) Stop() error {
	if i.currentCommand == nil {
		return fmt.Errorf("no Ollama process to stop")
	}
	log.Info().Msgf("🟢 stop Ollama model instance tree")
	if err := killProcessTree(i.currentCommand.Process.Pid); err != nil {
		log.Error().Msgf("error stopping Ollama model process: %s", err.Error())
		return err
	}
	log.Info().Msgf("🟢 stopped Ollama instance")
	close(i.workCh)
	i.cancel()

	return nil
}

func (i *OllamaInferenceModelInstance) ID() string {
	return i.id
}

func (i *OllamaInferenceModelInstance) Filter() types.SessionFilter {
	return types.SessionFilter{
		ModelName: i.modelName,
		Mode:      types.SessionModeInference,
	}
}

func (i *OllamaInferenceModelInstance) Stale() bool {
	return time.Since(i.lastActivity) > i.runnerOptions.Config.Runtimes.Ollama.InstanceTTL
}

func (i *OllamaInferenceModelInstance) Model() model.Model {
	return i.model
}

func (i *OllamaInferenceModelInstance) GetState() (*types.ModelInstanceState, error) {
	if i.initialRequest == nil {
		return nil, fmt.Errorf("no initial session")
	}

	var (
		sessionSummary *types.SessionSummary
	)

	if i.currentRequest != nil {
		var summary string

		// Get last message
		if len(i.currentRequest.Request.Messages) > 0 {
			summary = i.currentRequest.Request.Messages[len(i.currentRequest.Request.Messages)-1].Content
		}

		sessionSummary = &types.SessionSummary{
			SessionID:     i.currentRequest.SessionID,
			Name:          "",
			InteractionID: i.currentRequest.InteractionID,
			Mode:          types.SessionModeInference,
			Type:          types.SessionTypeText,
			ModelName:     i.modelName,
			Owner:         i.currentRequest.OwnerID,
			LoraDir:       "",
			Summary:       summary,
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
		ModelName:        i.modelName,
		Mode:             types.SessionModeInference,
		InitialSessionID: i.initialRequest.SessionID,
		CurrentSession:   sessionSummary,
		JobHistory:       i.jobHistory,
		Timeout:          int(i.runnerOptions.Config.Runtimes.Ollama.InstanceTTL.Seconds()),
		LastActivity:     int(i.lastActivity.Unix()),
		Stale:            stale,
		MemoryUsage:      i.model.GetMemoryRequirements(types.SessionModeInference),
	}, nil
}

func (i *OllamaInferenceModelInstance) processInteraction(inferenceReq *types.RunnerLLMInferenceRequest) error {
	switch {
	case inferenceReq.Request.Stream:
		stream, err := i.client.CreateChatCompletionStream(context.Background(), *inferenceReq.Request)
		if err != nil {
			return fmt.Errorf("failed to get response from inference API: %w", err)
		}

		defer stream.Close()

		var buf string

		toolCalls := make(map[string]openai.ToolCall)

		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				log.Info().Str("session_id", inferenceReq.SessionID).Msg("stream finished")
				// Signal the end of the stream
				i.emitStreamDone(inferenceReq)
				// Send the last message containing full output
				// TODO: set usage

				toolCallsArr := make([]openai.ToolCall, 0, len(toolCalls))
				for _, toolCall := range toolCalls {
					toolCallsArr = append(toolCallsArr, toolCall)
				}

				i.responseProcessor(inferenceReq, types.Usage{}, buf, toolCallsArr, "", true)
				return nil
			}

			if err != nil {
				log.Error().Err(err).Msg("stream error")
				i.errorSession(inferenceReq, err)
				return err
			}

			buf += response.Choices[0].Delta.Content

			if len(response.Choices[0].Delta.ToolCalls) > 0 {
				for _, toolCall := range response.Choices[0].Delta.ToolCalls {
					toolCalls[toolCall.ID] = toolCall
				}
			}

			i.responseProcessor(inferenceReq, types.Usage{}, response.Choices[0].Delta.Content, response.Choices[0].Delta.ToolCalls, "", false)
		}
	default:
		start := time.Now()

		response, err := i.client.CreateChatCompletion(context.Background(), *inferenceReq.Request)
		if err != nil {
			return fmt.Errorf("failed to get response from inference API: %w", err)
		}

		log.Info().
			Str("session_id", inferenceReq.SessionID).
			Msg("response received")

		i.emitStreamDone(inferenceReq)

		usage := types.Usage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
			DurationMs:       time.Since(start).Milliseconds(),
		}

		// Send the last message containing full output
		i.responseProcessor(inferenceReq,
			usage,
			response.Choices[0].Message.Content,
			response.Choices[0].Message.ToolCalls,
			response.Choices[0].Message.ToolCallID,
			true)
		return nil
	}
}

func (i *OllamaInferenceModelInstance) responseProcessor(
	req *types.RunnerLLMInferenceRequest,
	usage types.Usage,
	content string,
	toolCalls []openai.ToolCall,
	toolCallID string,
	done bool) {
	if req == nil {
		log.Error().Msgf("no current request")
		return
	}

	var err error

	resp := &types.RunnerTaskResponse{
		SessionID:     req.SessionID,
		InteractionID: req.InteractionID,
		Owner:         req.OwnerID,
		Done:          done,
		Message:       content,
		Usage:         usage,
		ToolCalls:     toolCalls,
		ToolCallID:    toolCallID,
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

func (i *OllamaInferenceModelInstance) emitStreamDone(req *types.RunnerLLMInferenceRequest) {
	err := i.responseHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeStream,
		SessionID: req.SessionID,
		Owner:     req.OwnerID,
		Message:   "",
		Done:      true,
	})
	if err != nil {
		log.Error().Msgf("error writing event: %s", err.Error())
		return
	}
}

func (i *OllamaInferenceModelInstance) Done() <-chan bool {
	return i.finishCh
}

func (i *OllamaInferenceModelInstance) addJobToHistory(session *types.Session) error {
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

func (i *OllamaInferenceModelInstance) errorSession(req *types.RunnerLLMInferenceRequest, err error) {
	apiUpdateErr := i.responseHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: req.SessionID,
		Owner:     req.OwnerID,
		Error:     err.Error(),
	})

	if apiUpdateErr != nil {
		log.Error().Msgf("Error reporting error to api: %v\n", apiUpdateErr.Error())
	}
}

func (i *OllamaInferenceModelInstance) QueueSession(session *types.Session, isInitialSession bool) {}
