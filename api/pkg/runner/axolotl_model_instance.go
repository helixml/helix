//go:build !windows
// +build !windows

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

var (
	_ ModelInstance = &AxolotlModelInstance{}
)

type TrainingStatusReport struct {
	Type         string  `json:"type"`
	Loss         float64 `json:"loss"`
	GradNorm     float64 `json:"grad_norm"`
	LearningRate float64 `json:"learning_rate"`
	Epoch        float64 `json:"epoch"`
	Progress     int     `json:"progress"`
}

type AxolotlModelInstanceConfig struct {
	RunnerOptions RunnerOptions

	// Get next chat completion request
	GetNextRequest func() (*types.Session, error)

	// Response writer
	ResponseHandler func(res *types.RunnerLLMInferenceResponse) error
}

// a long running instance of a loaded into memory model
// that can run multiple session tasks sequentially
// we keep state of the active text stream (if the model supports it)
// and are in charge of sending updates out of the model to the api
// to update it's state
type AxolotlModelInstance struct {
	id                string
	model             model.Model
	filter            types.SessionFilter
	finishChan        chan bool
	runnerOptions     RunnerOptions
	httpClientOptions system.ClientOptions

	// we write responses to this function and they will be sent to the api
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

	// the timestamp of when this model instance either completed a job
	// or a new job was pulled and allocated
	// we use this timestamp to cleanup non-active model instances
	lastActivity time.Time

	// the file handler we use to download and upload session files
	fileHandler *FileHandler

	// a history of the session IDs
	jobHistory []*types.SessionSummary

	// New fields TODO tidy up
	client         *openai.Client
	workCh         chan *types.Session
	getNextSession func() (*types.Session, error)
	cancel         context.CancelFunc
}

func (i *AxolotlModelInstance) ID() string {
	return i.id
}

func (i *AxolotlModelInstance) Filter() types.SessionFilter {
	return i.filter
}

func (i *AxolotlModelInstance) Stale() bool {
	return time.Since(i.lastActivity) > i.runnerOptions.Config.Runtimes.Axolotl.InstanceTTL
}

func (i *AxolotlModelInstance) Model() model.Model {
	return i.model
}

func (i *AxolotlModelInstance) Done() <-chan bool {
	return i.finishChan
}

func (i *AxolotlModelInstance) IsActive() bool {
	return i.currentSession != nil
}

type ModelInstanceConfig struct {
	// the session that meant this model instance is instantiated
	InitialSession *types.Session
	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/task/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/task
	NextTaskURL string
	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/initial_session/:instanceid
	InitialSessionURL string

	ResponseHandler func(res *types.RunnerTaskResponse) error

	GetNextSession func() (*types.Session, error)

	RunnerOptions  RunnerOptions
	GetNextRequest func() (*types.Session, error)
}

func NewAxolotlModelInstance(ctx context.Context, cfg *ModelInstanceConfig) (*AxolotlModelInstance, error) {
	aiModel, err := model.GetModel(cfg.InitialSession.ModelName)
	if err != nil {
		return nil, err
	}
	id := system.GenerateUUID()

	httpClientOptions := system.ClientOptions{
		Host:  cfg.RunnerOptions.ApiHost,
		Token: cfg.RunnerOptions.ApiToken,
	}

	ctx, cancel := context.WithCancel(ctx)
	modelInstance := &AxolotlModelInstance{
		id:              id,
		ctx:             ctx,
		cancel:          cancel,
		finishChan:      make(chan bool),
		workCh:          make(chan *types.Session, 1),
		model:           aiModel,
		responseHandler: cfg.ResponseHandler,
		getNextSession:  cfg.GetNextSession,
		initialSession:  cfg.InitialSession,
		filter: types.SessionFilter{
			ModelName: cfg.InitialSession.ModelName,
			Mode:      cfg.InitialSession.Mode,
			LoraDir:   cfg.InitialSession.LoraDir,
			Type:      cfg.InitialSession.Type,
		},
		runnerOptions:     cfg.RunnerOptions,
		jobHistory:        []*types.SessionSummary{},
		lastActivity:      time.Now(),
		httpClientOptions: httpClientOptions,
	}

	fileHandler := NewFileHandler(cfg.RunnerOptions.ID, httpClientOptions, modelInstance.taskResponseHandler)
	modelInstance.fileHandler = fileHandler

	modelInstance.workCh <- cfg.InitialSession

	return modelInstance, nil
}

/*



	QUEUE



*/

func (i *AxolotlModelInstance) getSessionFileHander(session *types.Session) *SessionFileHandler {
	return &SessionFileHandler{
		folder:    path.Join(os.TempDir(), "helix", "downloads", session.ID),
		sessionID: session.ID,
		downloadFile: func(sessionID string, remotePath string, localPath string) error {
			return i.fileHandler.downloadFile(sessionID, remotePath, localPath)
		},
		downloadFolder: func(sessionID string, remotePath string, localPath string) error {
			return i.fileHandler.downloadFolder(sessionID, remotePath, localPath)
		},
	}
}

// this is the loading of a session onto a running model instance
// it also returns the task that will be fed down into the python code to execute
func (i *AxolotlModelInstance) AssignSessionTask(ctx context.Context, session *types.Session) (*types.RunnerTask, error) {
	// mark the instance as active so it doesn't get cleaned up
	i.lastActivity = time.Now()
	i.currentSession = session

	task, err := i.model.GetTask(session, i.getSessionFileHander(session))
	if err != nil {
		log.Error().Msgf("error getting task: %s", err.Error())
		return nil, err
	}
	task.SessionID = session.ID
	return task, nil
}

func (i *AxolotlModelInstance) QueueSession(session *types.Session, isInitialSession bool) {}

/*



	EVENT HANDLERS



*/

func (i *AxolotlModelInstance) errorSession(session *types.Session, err error) {
	apiUpdateErr := i.responseHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeResult,
		SessionID: session.ID,
		Error:     err.Error(),
	})

	if apiUpdateErr != nil {
		log.Error().Msgf("Error reporting error to api: %v\n", apiUpdateErr.Error())
	}
}

/*



PROCESS MANAGEMENT



*/

func (i *AxolotlModelInstance) taskResponseHandler(taskResponse *types.RunnerTaskResponse) {
	if i.currentSession == nil {
		log.Error().Msgf("no current session")
		return
	}
	if i.currentSession.ID != taskResponse.SessionID {
		log.Error().Msgf("current session ID mis-match: current=%s vs event=%s", i.currentSession.ID, taskResponse.SessionID)
		return
	}

	var err error

	assistantInteraction, err := data.GetAssistantInteraction(i.currentSession)
	if err != nil {
		log.Error().Msgf("error getting assistant interaction: %s", err.Error())
		return
	}

	taskResponse.InteractionID = assistantInteraction.ID
	taskResponse.Owner = i.currentSession.Owner
	i.lastActivity = time.Now()

	// if it's the final result then set the current session to nil
	if taskResponse.Type == types.WorkerTaskResponseTypeResult {

		// If it's a fine-tuning session, we need to update the LORA dir
		if i.currentSession.Mode == types.SessionModeFinetune {
			taskResponse, err = i.fileHandler.uploadWorkerResponse(taskResponse)
			if err != nil {
				log.Error().Msgf("error uploading task result files: %s", err.Error())
				i.currentSession = nil
				return
			}
		}

		// TODO(PHIL): This is pretty sketchy, this is the only reason why it would take new work, but it's
		// buried within a handler that is only called when the session is done
		i.currentSession = nil
	}

	// this will emit to the controller handler
	// i.e. the function defined in createModelInstance
	err = i.responseHandler(taskResponse)
	if err != nil {
		log.Error().Msgf("error writing event: %s", err.Error())
		return
	}

}

func (i *AxolotlModelInstance) Start(ctx context.Context) error {
	// Get random free port
	port, err := freeport.GetFreePort()
	if err != nil {
		return fmt.Errorf("error getting free port: %s", err.Error())
	}

	config := openai.DefaultConfig("axolotl")
	config.BaseURL = fmt.Sprintf("http://localhost:%d/v1", port)
	i.client = openai.NewClientWithConfig(config)

	cmd, err := i.model.GetCommand(i.ctx, i.filter, types.RunnerProcessConfig{
		InstanceID:      i.id,
		MockRunner:      i.runnerOptions.MockRunner,
		MockRunnerError: i.runnerOptions.MockRunnerError,
		MockRunnerDelay: i.runnerOptions.MockRunnerDelay,
		Port:            port,
	})
	if err != nil {
		return err
	}
	if cmd == nil {
		return fmt.Errorf("no command to run")
	}
	log.Debug().Msgf("🔵 runner start process: %s %+v %+v", i.initialSession.ID, cmd.Args, cmd.Env)

	log.Info().
		Msgf("🟢 run model instance: %s, %+v, %s", cmd.Dir, cmd.Args, cmd.Env)

	sessionCopy := *i.initialSession
	for i, itx := range sessionCopy.Interactions {
		if itx.Error != "" {
			sessionCopy.Interactions[i].Error = "<old error redacted for developer sanity>"
		}
	}

	log.Debug().
		Msgf("🟢 initial session: %s, %+v", i.initialSession.ID, sessionCopy)

	i.currentCommand = cmd

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	// this buffer is so we can keep the last 10kb of stderr so if
	// there is an error we can send it to the api
	stderrBuf := system.NewLimitedBuffer(1024 * 10)

	stdoutWriters := []io.Writer{os.Stdout}
	stderrWriters := []io.Writer{os.Stderr, stderrBuf}

	go func() {
		_, err := io.Copy(io.MultiWriter(stdoutWriters...), stdoutPipe)
		if err != nil {
			log.Error().Msgf("Error copying stdout: %v", err)
		}
	}()

	// stream stderr to os.Stderr (so we can see it in the logs)
	// and also the error buffer we will use to post the error to the api
	go func() {
		_, err := io.Copy(io.MultiWriter(stderrWriters...), stderrPipe)
		if err != nil {
			log.Error().Msgf("Error copying stderr: %v", err)
		}
	}()

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		log.Error().Msgf("Failed to start command: %v\n", err.Error())
		return err
	}

	go func(cmd *exec.Cmd) {
		defer close(i.finishChan)
		if err := cmd.Wait(); err != nil {
			log.Error().Msgf("Axolotl model instance exited with error: %s", err.Error())

			errstr := string(stderrBuf.Bytes())
			if i.currentSession != nil {
				i.errorSession(i.currentSession, fmt.Errorf("%s from cmd - %s", err.Error(), errstr))
			}

			if strings.Contains(errstr, "(core dumped)") {
				log.Error().Msg("detected coredump, exiting and hoping we get restarted - see https://github.com/helixml/helix/issues/123")
				os.Exit(1)
			}
			if strings.Contains(errstr, "CUDA is not available") {
				log.Error().Msg("detected GPU error, exiting and hoping we get restarted - see https://github.com/helixml/helix/issues/123")
				os.Exit(1)
			}
			if strings.Contains(errstr, "not supported on this GPU") {
				// sometimes this happens when drivers get upgraded and then container needs restarting - we think
				log.Error().Msg("detected GPU error 2, exiting and hoping we get restarted - see https://github.com/helixml/helix/issues/123")
				os.Exit(1)
			}

			return
		}

		log.Info().Msgf("🟢 Axolotl model instance stopped, exit code=%d", cmd.ProcessState.ExitCode())
	}(cmd)

	// Wait for the server to start
	startCtx, cancel := context.WithTimeout(i.ctx, 10*time.Second)
	defer cancel()

WAIT:
	for {
		select {
		case <-startCtx.Done():
			return fmt.Errorf("timeout waiting for Axolotl model instance to start")
		default:
			resp, err := http.DefaultClient.Get(fmt.Sprintf("http://localhost:%d/healthz", port))
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

	log.Info().Msgf("🟢 Axolotl model instance started on port %d", port)

	// TODO: do we need to pull models?

	go func() {
		for {
			select {
			case <-i.ctx.Done():
				log.Info().Msgf("🟢 Axolotl model instance has stopped, closing channel listener")
				return
			case session, ok := <-i.workCh:
				if !ok {
					log.Info().Msg("🟢 workCh closed, exiting")
					return
				}
				log.Info().Str("session_id", session.ID).Msg("🟢 processing interaction")

				i.currentSession = session
				i.lastActivity = time.Now()

				err := i.processInteraction(session)
				if err != nil {
					log.Error().
						Str("session_id", session.ID).
						Err(err).
						Msg("error processing interaction")
					i.errorSession(session, err)
					if strings.Contains(err.Error(), "connection refused") {
						log.Error().Msg("detected connection refused, exiting and hoping we get restarted - see https://github.com/helixml/helix/issues/242")
						os.Exit(1)
					}
				} else {
					log.Info().
						Str("session_id", session.ID).
						Bool("stream", session.Metadata.Stream).
						Msg("🟢 interaction processed")
				}

				i.currentSession = nil
			default:
				// Get next session
				session, err := i.getNextSession()
				if err != nil {
					log.Error().Err(err).Msg("error getting next session")
					time.Sleep(300 * time.Millisecond)
					continue
				}

				if session == nil {
					log.Trace().Msg("no next session")
					time.Sleep(300 * time.Millisecond)
					continue
				}

				log.Debug().Str("session_id", session.ID).Msg("🟢 enqueuing session")

				i.workCh <- session
			}
		}
	}()

	return nil
}

func (i *AxolotlModelInstance) Stop() error {
	if i.currentCommand == nil {
		return fmt.Errorf("no Axolotl process to stop")
	}
	log.Info().Msgf("🟢 stop Axolotl model instance tree")
	if err := killProcessTree(i.currentCommand.Process.Pid); err != nil {
		log.Error().Msgf("error stopping Ollama model process: %s", err.Error())
		return err
	}
	log.Info().Msgf("🟢 stopped Axolotl instance")
	// from Karolis: and on model instance stop close the workCh but the writer
	// needs to not write then as it will panic, better to cancel the ctx, I
	// think that was the idea there
	//
	// Luke: so... try both?
	close(i.workCh)
	i.cancel()

	return nil
}

func (i *AxolotlModelInstance) GetState() (*types.ModelInstanceState, error) {
	if i.initialSession == nil {
		return nil, fmt.Errorf("no initial session")
	}

	var sessionSummary *types.SessionSummary
	var err error

	if i.currentSession != nil {
		sessionSummary, err = data.GetSessionSummary(i.currentSession)
		if err != nil {
			return nil, err
		}
	}

	return &types.ModelInstanceState{
		ID:               i.id,
		ModelName:        i.initialSession.ModelName,
		Mode:             i.initialSession.Mode,
		LoraDir:          i.initialSession.LoraDir,
		InitialSessionID: i.initialSession.ID,
		CurrentSession:   sessionSummary,
		JobHistory:       i.jobHistory,
		Timeout:          int(i.runnerOptions.Config.Runtimes.Axolotl.InstanceTTL.Seconds()),
		LastActivity:     int(i.lastActivity.Unix()),
		Stale:            i.Stale(),
		MemoryUsage:      i.model.GetMemoryRequirements(i.initialSession.Mode),
	}, nil
}

func (i *AxolotlModelInstance) processInteraction(session *types.Session) error {
	switch session.Mode {
	case types.SessionModeFinetune:
		log.Info().Str("session_id", session.ID).Msg("processing fine-tuning interaction")
		// accumulate all JSONL files across all interactions
		// and append them to one large JSONL file
		fileManager := i.getSessionFileHander(session)
		userInteractions := data.FilterUserInteractions(session.Interactions)
		finetuneInteractions := data.FilterFinetuneInteractions(userInteractions)
		jsonLFiles := []string{}
		for _, interaction := range finetuneInteractions {
			for _, file := range interaction.Files {
				if path.Base(file) == types.TEXT_DATA_PREP_QUESTIONS_FILE {
					localFilename := fmt.Sprintf("%s.jsonl", interaction.ID)
					localPath := path.Join(fileManager.GetFolder(), localFilename)
					err := fileManager.DownloadFile(file, localPath)
					if err != nil {
						return err
					}
					jsonLFiles = append(jsonLFiles, localPath)
				}
			}
		}

		combinedFile := path.Join(fileManager.GetFolder(), types.TEXT_DATA_PREP_QUESTIONS_FILE)
		err := system.ConcatenateFiles(combinedFile, jsonLFiles, "\n")
		if err != nil {
			return err
		}

		// Check that combined file size is not zero
		fi, err := os.Stat(combinedFile)
		if err != nil {
			return err
		}
		if fi.Size() <= 1 {
			// Check for 1 byte to account for just a newline character
			return fmt.Errorf("training data file is empty")
		}
		log.Debug().Str("session_id", session.ID).Int64("file_size", fi.Size()).Msgf("combined file size")

		req := openai.FineTuningJobRequest{
			Model:          session.ModelName,
			TrainingFile:   combinedFile,
			ValidationFile: "",
			Hyperparameters: &openai.Hyperparameters{
				Epochs:                 20, // TODO: connect this up to the finetuning API when it is ready
				LearningRateMultiplier: 0.0002,
				BatchSize:              6,
			},
			Suffix: session.ID, // Use the suffix to identify the session and the final directory for the LORA
		}

		job, err := i.client.CreateFineTuningJob(i.ctx, req)
		if err != nil {
			return fmt.Errorf("creating fine-tuning job: %w", err)
		}
		log.Debug().Str("session_id", session.ID).Msgf("fine-tuning job created: %s", job.ID)

		for {
			time.Sleep(1 * time.Second)
			events, err := i.client.ListFineTuningJobEvents(i.ctx, job.ID)
			if err != nil {
				if strings.Contains(err.Error(), "connection refused") {
					continue
				}
				return fmt.Errorf("retrieving fine-tuning events: %w", err)
			}
			log.Debug().Str("session_id", session.ID).Msgf("fine-tuning events: %d", len(events.Data))

			status, err := i.client.RetrieveFineTuningJob(i.ctx, job.ID)
			if err != nil {
				if strings.Contains(err.Error(), "connection refused") {
					continue
				}
				return fmt.Errorf("retrieving fine-tuning status: %w", err)
			}
			log.Debug().Str("session_id", session.ID).Interface("status", status).Msg("fine-tuning status")

			// Get latest training report
			var report TrainingStatusReport
			for _, event := range events.Data {
				// ignore errors, just capture latest whatever we can
				var newReport TrainingStatusReport
				err := json.Unmarshal([]byte(event.Message), &newReport)
				if err == nil {
					if newReport.Type == "training_progress_report" {
						report = newReport
					}
				}
			}

			log.Debug().Str("session_id", session.ID).Interface("report", report).Msg("fine-tuning progress")

			switch status.Status {
			case "running":
				i.responseHandler(&types.RunnerTaskResponse{
					Type:      types.WorkerTaskResponseTypeProgress,
					SessionID: session.ID,
					Owner:     session.Owner,
					Done:      false,
					Progress:  report.Progress,
					Status:    status.Status,
				})
			case "succeeded":
				if len(status.ResultFiles) < 1 {
					return fmt.Errorf("fine-tuning succeeded but no result files")
				}
				i.taskResponseHandler(&types.RunnerTaskResponse{
					Type:      types.WorkerTaskResponseTypeResult,
					SessionID: session.ID,
					Owner:     session.Owner,
					Done:      true,
					Progress:  100,
					LoraDir:   status.ResultFiles[0],
					Status:    status.Status,
				})
				return nil
			case string(openai.RunStatusFailed):
				if len(events.Data) > 0 {
					return fmt.Errorf("fine-tuning failed: %s", events.Data[len(events.Data)-1].Message)
				} else {
					return fmt.Errorf("fine-tuning failed with no events")
				}
			default:
				return fmt.Errorf("unknown fine-tuning status: %s", status.Status)
			}
		}
	case types.SessionModeInference:
		log.Debug().Str("session_id", session.ID).Msg("processing inference interaction")

		downloadedLoraDir := ""
		if session.LoraDir != "" {
			downloadedLoraDir = "/tmp/helix/results/" + session.ID
			err := i.fileHandler.downloadFolder(session.ID, session.LoraDir, downloadedLoraDir)
			if err != nil {
				return fmt.Errorf("downloading LORA dir: %w", err)
			}
		}

		// Convert session interactions to chat completion messages
		var messages []openai.ChatCompletionMessage
		// Adding the system prompt first
		if session.Metadata.SystemPrompt != "" {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: session.Metadata.SystemPrompt,
			})
		}

		for _, interaction := range session.Interactions {
			switch interaction.Creator {
			case types.CreatorTypeUser:
				messages = append(messages, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleUser,
					Content: interaction.Message,
				})
			case types.CreatorTypeSystem:
				// Ignore because axoloatl doesn't support system messages after the initial one
			case types.CreatorTypeAssistant:
				messages = append(messages, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: interaction.Message,
				})
			case types.CreatorTypeTool:
				messages = append(messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleUser,
					Content:    interaction.Message,
					ToolCalls:  interaction.ToolCalls,
					ToolCallID: interaction.ToolCallID,
				})
			}
		}

		resp, err := i.client.CreateChatCompletion(i.ctx, openai.ChatCompletionRequest{
			Model:    downloadedLoraDir,
			Messages: messages,
		})
		if err != nil {
			return fmt.Errorf("creating chat completion: %w", err)
		}

		// Signal the end of the stream
		assistantInteraction, err := data.GetAssistantInteraction(session)
		if err != nil {
			return fmt.Errorf("getting assistant interaction: %w", err)
		}

		i.taskResponseHandler(&types.RunnerTaskResponse{
			Type:          types.WorkerTaskResponseTypeResult,
			SessionID:     session.ID,
			InteractionID: assistantInteraction.ID,
			Owner:         session.Owner,
			Done:          true,
			Message:       resp.Choices[0].Message.Content,
			Usage: types.Usage{
				TotalTokens:      resp.Usage.TotalTokens,
				PromptTokens:     resp.Usage.PromptTokens,
				CompletionTokens: resp.Usage.CompletionTokens,
				DurationMs:       time.Since(time.UnixMilli(resp.Created)).Milliseconds(),
			},
		})
	default:
		return fmt.Errorf("unknown session mode: %s", session.Mode)
	}
	return nil
}
