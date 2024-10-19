package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"

	"github.com/jmorganca/ollama/api"
	"github.com/rs/zerolog/log"
)

const (
	defaultMaxTokens = 8192
)

type InferenceModelInstanceConfig struct {
	RunnerOptions RunnerOptions

	// Get next chat completion request
	GetNextRequest func() (*types.RunnerLLMInferenceRequest, error)

	// Response writer
	ResponseHandler func(res *types.RunnerLLMInferenceResponse) error
}

var (
	ollamaCommander Commander     = &RealCommander{}
	_               ModelInstance = &OllamaInferenceModelInstance{}
)

func NewOllamaInferenceModelInstance(ctx context.Context, cfg *InferenceModelInstanceConfig, request *types.RunnerLLMInferenceRequest) (*OllamaInferenceModelInstance, error) {
	modelName := model.ModelName(request.Request.Model)

	aiModel, err := model.GetModel(string(modelName))
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
		commander:       ollamaCommander,
		freePortFinder:  freePortFinder,
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
	modelName model.ModelName

	runnerOptions RunnerOptions

	finishCh chan bool

	workCh chan *types.RunnerLLMInferenceRequest

	inUse    atomic.Bool // If we are currently processing a request
	fetching atomic.Bool // If we are fetching the next request

	// client is the model client
	client *api.Client

	ollamaClient *ollamaClient

	// Streaming response handler
	responseHandler func(res *types.RunnerLLMInferenceResponse) error

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

	// Interface to run commands
	commander Commander

	// Interface to find free ports
	freePortFinder FreePortFinder

	// ensure ollama client creation is single threaded
	ollamaClientMutex sync.Mutex

	// port is the port that the ollama server is running on
	port int
}

// Warmup starts Ollama server and pulls the models
func (i *OllamaInferenceModelInstance) Warmup(_ context.Context) error {
	err := i.startOllamaServer(i.ctx)
	if err != nil {
		return err
	}

	return i.warmup(i.ctx)
}

func (i *OllamaInferenceModelInstance) Start(_ context.Context) error {
	err := i.startOllamaServer(i.ctx)
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
					// If context is cancelled, no error
					if i.ctx.Err() != nil {
						log.Error().Msg("context cancelled, exiting")
						return
					}

					log.Error().
						Str("session_id", req.SessionID).
						Err(err).
						Msg("error processing request")
					i.errorResponse(req, err)
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
				req, err := i.fetchNextRequest()
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

func (i *OllamaInferenceModelInstance) fetchNextRequest() (*types.RunnerLLMInferenceRequest, error) {
	i.fetching.Store(true)
	defer i.fetching.Store(false)

	return i.getNextRequest()
}

func (i *OllamaInferenceModelInstance) warmup(_ context.Context) error {
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

func (i *OllamaInferenceModelInstance) startOllamaServer(_ context.Context) error {
	ollamaPath, err := i.commander.LookPath("ollama")
	if err != nil {
		return fmt.Errorf("ollama not found in PATH")
	}

	// Get random free port
	port, err := i.freePortFinder.GetFreePort()
	if err != nil {
		return fmt.Errorf("error getting free port: %s", err.Error())
	}
	i.port = port

	ollamaHost := fmt.Sprintf("0.0.0.0:%d", port)

	func() {
		// ollama client only supports being constructed from the environment, but
		// the environment is global. ensure we speak to the right ollama server by
		// only allowing one client to be created at a time

		// XXX try to fix hanging tests

		// i.ollamaClientMutex.Lock()
		// defer i.ollamaClientMutex.Unlock()

		os.Setenv("OLLAMA_HOST", ollamaHost)
		i.client, err = api.ClientFromEnvironment()
	}()

	if err != nil {
		return fmt.Errorf("error creating Ollama client: %s", err.Error())
	}

	cmd := i.commander.CommandContext(i.ctx, ollamaPath, "serve")
	// Getting base env (HOME, etc)
	cmd.Env = append(cmd.Env,
		os.Environ()...,
	)

	cmd.Env = append(cmd.Env,
		"OLLAMA_KEEP_ALIVE=-1",
		"OLLAMA_MAX_LOADED_MODELS=1",
		"OLLAMA_NUM_PARALLEL=1",
		"OLLAMA_FLASH_ATTENTION=1",
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
				i.errorResponse(i.currentRequest, fmt.Errorf("%s from cmd - %s", err.Error(), errMsg))
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
	// It is very important that we cancel the context only after we've
	// gracefully shut down the child processes in killProcessTree() above.
	// Otherwise we will leave child processes of ollama using GPU memory
	// forever, not discoverable via parent pid thereby permanently making this
	// runner mysteriously terribly slow.
	i.cancel()

	return nil
}

func (i *OllamaInferenceModelInstance) ID() string {
	return i.id
}

func (i *OllamaInferenceModelInstance) Filter() types.SessionFilter {
	return types.SessionFilter{
		ModelName: string(i.modelName),
		Mode:      types.SessionModeInference,
	}
}

func (i *OllamaInferenceModelInstance) Stale() bool {
	// If in use, we don't want to mark it as stale
	if i.inUse.Load() {
		return false
	}

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
			ModelName:     string(i.modelName),
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

	status := i.getOllamaStatus()

	return &types.ModelInstanceState{
		ID:               i.id,
		ModelName:        string(i.modelName),
		Mode:             types.SessionModeInference,
		InitialSessionID: i.initialRequest.SessionID,
		CurrentSession:   sessionSummary,
		JobHistory:       i.jobHistory,
		Timeout:          int(i.runnerOptions.Config.Runtimes.Ollama.InstanceTTL.Seconds()),
		LastActivity:     int(i.lastActivity.Unix()),
		Stale:            stale,
		MemoryUsage:      i.model.GetMemoryRequirements(types.SessionModeInference),
		Status:           status,
	}, nil
}

func (i *OllamaInferenceModelInstance) getOllamaStatus() string {
	// Create a context with a 1-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Prepare the command with the custom environment
	cmd := exec.CommandContext(ctx, "ollama", "ps")
	cmd.Env = append(os.Environ(), fmt.Sprintf("OLLAMA_HOST=127.0.0.1:%d", i.port))

	// Run the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Warn().Msg("Ollama ps command timed out after 1 second")
			return "Timed out"
		}
		log.Error().Err(err).Msgf("Failed to execute ollama ps command with environment OLLAMA_HOST=127.0.0.1:%d", i.port)
		return "Unknown"
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return "Empty (!)"
	}

	// Join all lines except the first (header) line
	status := strings.Join(lines[1:], "\n")
	log.Debug().Msgf("Ollama status on port %d:\n%s", i.port, status)

	return status
}

func (i *OllamaInferenceModelInstance) processInteraction(inferenceReq *types.RunnerLLMInferenceRequest) error {
	i.inUse.Store(true)
	defer i.inUse.Store(false)

	// Get the default Ollama models
	defaultModels, err := model.GetDefaultOllamaModels()
	if err != nil {
		return fmt.Errorf("failed to get default Ollama models: %w", err)
	}

	// Look up the context length for the current model
	max_tokens := defaultMaxTokens
	for _, m := range defaultModels {
		if m.Id == inferenceReq.Request.Model {
			log.Info().Msgf("using context length %d for model %s", m.ContextLength, inferenceReq.Request.Model)
			max_tokens = int(m.ContextLength)
			break
		}
	}

	// If max_tokens is specified in the request, use that instead
	if inferenceReq.Request.MaxTokens > 0 {
		max_tokens = inferenceReq.Request.MaxTokens
	}

	messages := make([]api.Message, 0, len(inferenceReq.Request.Messages))
	for _, m := range inferenceReq.Request.Messages {
		messages = append(messages, api.Message{
			Role:    m.Role,
			Content: m.Content,
			// ignoring Images for now
		})
	}

	req := api.ChatRequest{
		Model:    inferenceReq.Request.Model,
		Messages: messages,
		Stream:   &inferenceReq.Request.Stream,
		Options: map[string]interface{}{
			"temperature": inferenceReq.Request.Temperature,
			"num_ctx":     max_tokens,
			"seed":        inferenceReq.Request.Seed,
			"top_p":       inferenceReq.Request.TopP,
			// ignoring everything else for now
		},
	}

	// Ensure Ollama is ready before sending a request
	log.Info().Msg("waiting for Ollama to be ready")
	startTime := time.Now()
	timeout := 60 * time.Second
	for {
		status := i.getOllamaStatus()
		if strings.Contains(status, "Forever") {
			break
		}
		if time.Since(startTime) > timeout {
			return fmt.Errorf("timeout waiting for Ollama to be ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Info().Msgf("ready in %.4f seconds", time.Since(startTime).Seconds())

	// If the request takes longer than 10 minutes, cancel it
	timeoutCtx, cancel := context.WithTimeout(i.ctx, 600*time.Second)
	defer cancel()

	switch {
	case inferenceReq.Request.Stream:
		start := time.Now()
		err := i.client.Chat(timeoutCtx, &req, func(resp api.ChatResponse) error {
			finishReason := openai.FinishReasonNull
			if resp.Metrics.EvalCount >= inferenceReq.Request.MaxTokens {
				finishReason = openai.FinishReasonLength
			}
			if resp.Done {
				finishReason = openai.FinishReasonStop
			}

			// Metrics
			chatResp := openai.ChatCompletionStreamResponse{
				Model: resp.Model,
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Role:    resp.Message.Role,
							Content: resp.Message.Content,
						},
						FinishReason: finishReason,
					},
				},
				Usage: &openai.Usage{
					PromptTokens:     resp.Metrics.PromptEvalCount,
					CompletionTokens: resp.Metrics.EvalCount,
					TotalTokens:      resp.Metrics.EvalCount + resp.Metrics.PromptEvalCount,
				},
			}
			i.responseStreamProcessor(inferenceReq, &chatResp, resp.Done, time.Since(start).Milliseconds())
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to get response from inference API: %w", err)
		}
		return nil
	default:
		start := time.Now()
		err := i.client.Chat(timeoutCtx, &req, func(resp api.ChatResponse) error {
			finishReason := openai.FinishReasonStop
			if resp.Metrics.EvalCount >= inferenceReq.Request.MaxTokens {
				finishReason = openai.FinishReasonLength
			}

			chatResp := &openai.ChatCompletionResponse{
				Model:   resp.Model,
				Created: resp.CreatedAt.Unix(),
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Role:    resp.Message.Role,
							Content: resp.Message.Content,
						},
						FinishReason: finishReason,
					},
				},
				Usage: openai.Usage{
					PromptTokens:     resp.Metrics.PromptEvalCount,
					CompletionTokens: resp.Metrics.EvalCount,
					TotalTokens:      resp.Metrics.EvalCount + resp.Metrics.PromptEvalCount,
				},
			}
			i.responseProcessor(inferenceReq, chatResp, time.Since(start).Milliseconds())
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to get response from inference API: %w", err)
		}
		return nil
	}
}

func (i *OllamaInferenceModelInstance) responseStreamProcessor(req *types.RunnerLLMInferenceRequest, resp *openai.ChatCompletionStreamResponse, done bool, durationMs int64) {
	if req == nil {
		log.Error().Msgf("no current request")
		return
	}

	if resp == nil {
		// Stub response for the last "done" entry
		resp = &openai.ChatCompletionStreamResponse{}
	}

	var err error

	inferenceResp := &types.RunnerLLMInferenceResponse{
		RequestID:      req.RequestID,
		OwnerID:        req.OwnerID,
		SessionID:      req.SessionID,
		InteractionID:  req.InteractionID,
		StreamResponse: resp,
		DurationMs:     durationMs,
		Done:           done,
	}

	err = i.responseHandler(inferenceResp)
	if err != nil {
		log.Error().Msgf("error writing event: %s", err.Error())
		return
	}
}

func (i *OllamaInferenceModelInstance) responseProcessor(req *types.RunnerLLMInferenceRequest, resp *openai.ChatCompletionResponse, durationMs int64) {
	if req == nil {
		log.Error().Msgf("no current request")
		return
	}

	var err error

	inferenceResp := &types.RunnerLLMInferenceResponse{
		RequestID:     req.RequestID,
		OwnerID:       req.OwnerID,
		SessionID:     req.SessionID,
		InteractionID: req.InteractionID,
		Response:      resp,
		DurationMs:    durationMs,
		Done:          true,
	}

	err = i.responseHandler(inferenceResp)
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

func (i *OllamaInferenceModelInstance) errorResponse(req *types.RunnerLLMInferenceRequest, err error) {
	apiUpdateErr := i.responseHandler(&types.RunnerLLMInferenceResponse{
		RequestID:     req.RequestID,
		OwnerID:       req.OwnerID,
		SessionID:     req.SessionID,
		InteractionID: req.InteractionID,
		Error:         err.Error(),
	})

	if apiUpdateErr != nil {
		log.Error().Msgf("Error reporting error to api: %v\n", apiUpdateErr.Error())
	}
}

func (i *OllamaInferenceModelInstance) QueueSession(session *types.Session, isInitialSession bool) {}
