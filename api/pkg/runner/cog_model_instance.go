//go:build !windows
// +build !windows

package runner

import (
	"bytes"
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
)

var (
	_ ModelInstance = &CogModelInstance{}
)

type CogInferenceRequest struct {
	Prompt string `json:"prompt"`
}

type CogInferenceResponse struct {
	Status string   `json:"status"`
	Output []string `json:"output"`
}

type CogModelInstanceConfig struct {
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
type CogModelInstance struct {
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
	workCh         chan *types.Session
	getNextSession func() (*types.Session, error)
	cancel         context.CancelFunc
	port           int
}

func (i *CogModelInstance) ID() string {
	return i.id
}

func (i *CogModelInstance) Filter() types.SessionFilter {
	return i.filter
}

func (i *CogModelInstance) Stale() bool {
	return time.Since(i.lastActivity) > i.runnerOptions.Config.Runtimes.Axolotl.InstanceTTL
}

func (i *CogModelInstance) Model() model.Model {
	return i.model
}

func (i *CogModelInstance) Done() <-chan bool {
	return i.finishChan
}

func (i *CogModelInstance) IsActive() bool {
	return i.currentSession != nil
}

func NewCogModelInstance(ctx context.Context, cfg *ModelInstanceConfig) (*CogModelInstance, error) {
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
	modelInstance := &CogModelInstance{
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

func (i *CogModelInstance) getSessionFileHander(session *types.Session) *SessionFileHandler {
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
func (i *CogModelInstance) AssignSessionTask(_ context.Context, session *types.Session) (*types.RunnerTask, error) {
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

func (i *CogModelInstance) QueueSession(*types.Session, bool) {}

/*



	EVENT HANDLERS



*/

func (i *CogModelInstance) errorSession(session *types.Session, err error) {
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

func (i *CogModelInstance) taskResponseHandler(taskResponse *types.RunnerTaskResponse) {
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
		// Upload the final image
		taskResponse, err = i.fileHandler.uploadWorkerResponse(taskResponse)
		if err != nil {
			log.Error().Msgf("error uploading task result files: %s", err.Error())
			i.currentSession = nil
			return
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

func (i *CogModelInstance) Start(_ context.Context) error {
	// Get random free port
	port, err := freeport.GetFreePort()
	if err != nil {
		return fmt.Errorf("error getting free port: %s", err.Error())
	}

	i.port = port

	cmd, err := i.model.GetCommand(i.ctx, i.filter, types.RunnerProcessConfig{
		InstanceID:      i.id,
		MockRunner:      i.runnerOptions.MockRunner,
		MockRunnerError: i.runnerOptions.MockRunnerError,
		MockRunnerDelay: i.runnerOptions.MockRunnerDelay,
		Port:            i.port,
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
			log.Error().Msgf("Cog model instance exited with error: %s", err.Error())

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

		log.Info().Msgf("🟢 Cog model instance stopped, exit code=%d", cmd.ProcessState.ExitCode())
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
			resp, err := http.DefaultClient.Get(fmt.Sprintf("http://localhost:%d/healthz", i.port))
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

	log.Info().Msgf("🟢 Cog model instance started on port %d", i.port)

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

func (i *CogModelInstance) Stop() error {
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

func (i *CogModelInstance) GetState() (*types.ModelInstanceState, error) {
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

func (i *CogModelInstance) processInteraction(session *types.Session) error {
	switch session.Mode {
	case types.SessionModeFinetune:
		return fmt.Errorf("fine-tuning not supported")
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

		lastUserInteraction, err := data.GetLastUserInteraction(session.Interactions)
		if err != nil {
			return fmt.Errorf("getting last user interaction: %w", err)
		}

		// Marshall the request
		requestBody, err := json.Marshal(CogInferenceRequest{
			Prompt: lastUserInteraction.Message,
		})
		if err != nil {
			return fmt.Errorf("marshalling request body: %w", err)
		}

		request, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/predictions/%s", i.port, session.ID), bytes.NewBuffer(requestBody))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		request.Header.Set("Content-Type", "application/json")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return fmt.Errorf("sending request: %w", err)
		}
		defer response.Body.Close()

		if response.StatusCode != http.StatusOK {
			// Copy the whole body to a string so we can log it
			body, err := io.ReadAll(response.Body)
			if err != nil {
				return fmt.Errorf("reading response body: %w", err)
			}
			return fmt.Errorf("non-200 response: %d, body: %s", response.StatusCode, string(body))
		}

		// Parse the response
		var responseBody CogInferenceResponse
		if err := json.NewDecoder(response.Body).Decode(&responseBody); err != nil {
			return fmt.Errorf("decoding response body: %w", err)
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
			Status:        responseBody.Status,
			Files:         responseBody.Output,
		})
	default:
		return fmt.Errorf("unknown session mode: %s", session.Mode)
	}
	return nil
}
