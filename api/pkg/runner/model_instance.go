package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// a long running instance of a loaded into memory model
// that can run multiple session tasks sequentially
// we keep state of the active text stream (if the model supports it)
// and are in charge of sending updates out of the model to the api
// to update it's state
type ModelInstance struct {
	id string

	model  model.Model
	filter types.SessionFilter

	finishChan chan bool

	runnerOptions     RunnerOptions
	httpClientOptions system.ClientOptions

	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/task/:instanceid
	nextTaskURL string
	// this is used to read what the next session is
	// i.e. once the session has prepared - we can read the next session
	// and know what the Lora file is
	initialSessionURL string

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

	// the file handler we use to download and upload session files
	fileHandler *FileHandler

	// a history of the session IDs
	jobHistory []*types.SessionSummary
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
	RunnerOptions   RunnerOptions
}

func NewModelInstance(ctx context.Context, cfg *ModelInstanceConfig) (*ModelInstance, error) {
	aiModel, err := model.GetModel(cfg.InitialSession.ModelName)
	if err != nil {
		return nil, err
	}
	id := system.GenerateUUID()

	// if this is empty string then we need to hoist it to be types.LORA_DIR_NONE
	// because then we are always specifically asking for a session that has no finetune file
	// if we left this blank we are saying "we don't care if it has one or not"
	useLoraDir := cfg.InitialSession.LoraDir

	if useLoraDir == "" {
		useLoraDir = types.LORA_DIR_NONE
	}

	httpClientOptions := system.ClientOptions{
		Host:  cfg.RunnerOptions.ApiHost,
		Token: cfg.RunnerOptions.ApiToken,
	}

	modelInstance := &ModelInstance{
		id:                id,
		ctx:               ctx,
		finishChan:        make(chan bool),
		model:             aiModel,
		responseHandler:   cfg.ResponseHandler,
		nextTaskURL:       fmt.Sprintf("%s/%s", cfg.NextTaskURL, id),
		initialSessionURL: fmt.Sprintf("%s/%s", cfg.InitialSessionURL, id),
		initialSession:    cfg.InitialSession,
		filter: types.SessionFilter{
			ModelName: cfg.InitialSession.ModelName,
			Mode:      cfg.InitialSession.Mode,
			LoraDir:   useLoraDir,
			Type:      cfg.InitialSession.Type,
		},
		runnerOptions:     cfg.RunnerOptions,
		httpClientOptions: httpClientOptions,
		jobHistory:        []*types.SessionSummary{},
	}

	fileHandler := NewFileHandler(cfg.RunnerOptions.ID, httpClientOptions, modelInstance.taskResponseHandler)
	modelInstance.fileHandler = fileHandler

	return modelInstance, nil
}

/*



	QUEUE



*/

func (instance *ModelInstance) getSessionFileHander(session *types.Session) *SessionFileHandler {
	return &SessionFileHandler{
		folder:    path.Join(os.TempDir(), "helix", "downloads", session.ID),
		sessionID: session.ID,
		downloadFile: func(sessionID string, remotePath string, localPath string) error {
			return instance.fileHandler.downloadFile(sessionID, remotePath, localPath)
		},
		downloadFolder: func(sessionID string, remotePath string, localPath string) error {
			return instance.fileHandler.downloadFolder(sessionID, remotePath, localPath)
		},
	}
}

// this is the loading of a session onto a running model instance
// it also returns the task that will be fed down into the python code to execute
func (instance *ModelInstance) assignSessionTask(ctx context.Context, session *types.Session) (*types.RunnerTask, error) {
	// mark the instance as active so it doesn't get cleaned up
	instance.lastActivityTimestamp = time.Now().Unix()
	instance.currentSession = session

	task, err := instance.model.GetTask(session, instance.getSessionFileHander(session))
	if err != nil {
		log.Error().Msgf("error getting task: %s", err.Error())
		return nil, err
	}
	task.SessionID = session.ID
	return task, nil
}

// to queue a session means to put it into a buffer and wait for the Python process to boot up and then "pull" it
func (instance *ModelInstance) queueSession(session *types.Session, isInitialSession bool) {
	instance.queuedSession = session
	instance.nextSession = nil

	log.Debug().
		Msgf("🔵 runner prepare session: %s", session.ID)

	preparedSession, err := instance.model.PrepareFiles(session, isInitialSession, instance.getSessionFileHander(session))
	if err != nil {
		log.Error().Msgf("error preparing session: %s", err.Error())
		instance.queuedSession = nil
		instance.nextSession = nil
		instance.errorSession(session, err)
		return
	}

	err = instance.addJobToHistory(session)

	if err != nil {
		log.Error().Msgf("error preparing session: %s", err.Error())
		instance.queuedSession = nil
		instance.nextSession = nil
		instance.errorSession(session, err)
		return
	}

	log.Debug().
		Msgf("🔵 runner assign next session: %s", preparedSession.ID)

	instance.queuedSession = nil
	instance.nextSession = preparedSession
}

/*



	EVENT HANDLERS



*/

func (instance *ModelInstance) errorSession(session *types.Session, err error) {
	apiUpdateErr := instance.responseHandler(&types.RunnerTaskResponse{
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

// we call this function from the text processors
func (instance *ModelInstance) taskResponseHandler(taskResponse *types.RunnerTaskResponse) {
	if instance.currentSession == nil {
		log.Error().Msgf("no current session")
		return
	}
	if instance.currentSession.ID != taskResponse.SessionID {
		log.Error().Msgf("current session ID mis-match: current=%s vs event=%s", instance.currentSession.ID, taskResponse.SessionID)
		return
	}

	var err error

	systemInteraction, err := data.GetSystemInteraction(instance.currentSession)
	if err != nil {
		log.Error().Msgf("error getting system interaction: %s", err.Error())
		return
	}

	taskResponse.InteractionID = systemInteraction.ID
	taskResponse.Owner = instance.currentSession.Owner
	instance.lastActivityTimestamp = time.Now().Unix()

	// if it's the final result then we need to upload the files first
	if taskResponse.Type == types.WorkerTaskResponseTypeResult {
		taskResponse, err = instance.fileHandler.uploadWorkerResponse(taskResponse)
		if err != nil {
			log.Error().Msgf("error uploading task result files: %s", err.Error())
			instance.currentSession = nil
			return
		}

		instance.currentSession = nil
	}

	// this will emit to the controller handler
	// i.e. the function defined in createModelInstance
	err = instance.responseHandler(taskResponse)
	if err != nil {
		log.Error().Msgf("error writing event: %s", err.Error())
		return
	}
}

// run the model process
// we pass the instance context in so we can cancel it using our stopProcess function
func (instance *ModelInstance) startProcess(session *types.Session) error {
	cmd, err := instance.model.GetCommand(instance.ctx, instance.filter, types.RunnerProcessConfig{
		InstanceID:        instance.id,
		NextTaskURL:       instance.nextTaskURL,
		InitialSessionURL: instance.initialSessionURL,
		MockRunner:        instance.runnerOptions.MockRunner,
		MockRunnerError:   instance.runnerOptions.MockRunnerError,
		MockRunnerDelay:   instance.runnerOptions.MockRunnerDelay,
	})
	if err != nil {
		return err
	}
	if cmd == nil {
		return fmt.Errorf("no command to run")
	}
	log.Debug().Msgf("🔵 runner start process: %s %+v %+v", session.ID, cmd.Args, cmd.Env)

	log.Info().
		Msgf("🟢 run model instance: %s, %+v, %s", cmd.Dir, cmd.Args, cmd.Env)

	sessionCopy := *session
	for i, itx := range sessionCopy.Interactions {
		if itx.Error != "" {
			sessionCopy.Interactions[i].Error = "<old error redacted for developer sanity>"
		}
	}

	log.Info().
		Msgf("🟢 initial session: %s, %+v", session.ID, sessionCopy)

	instance.currentCommand = cmd

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

	// create the model textsream
	// this is responsible for chunking stdout into session outputs
	// and keeping track of the current session
	// each model knows how to parse it's own stdout differently
	// we pass a 'textStreamProcessor' function which will get events:
	//  * a new session has started
	//  * some more text has been generated (i.e. streaming output)
	//  * the result has been generated
	// in all cases - each model get's to decide what formatting
	// it's Python needs to use so that these text streams will
	// parse correctly
	stdout, stderr, err := instance.model.GetTextStreams(session.Mode, instance.taskResponseHandler)
	if err != nil {
		return err
	}

	if stdout != nil {
		go stdout.Start()
		stdoutWriters = append(stdoutWriters, stdout)
	}

	if stderr != nil {
		go stderr.Start()
		stderrWriters = append(stderrWriters, stderr)
	}

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
		// Signal the runner to drop the model instance
		defer close(instance.finishChan)

		if err = cmd.Wait(); err != nil {
			log.Error().Msgf("Command ended with an error: %v\n", err.Error())

			// we are currently running a session and we got an error from the Python process
			// this normally means that a job caused an error so let's tell the api
			// that this interaction has it's Error field set

			errstr := string(stderrBuf.Bytes())
			if instance.currentSession != nil {
				instance.errorSession(instance.currentSession, fmt.Errorf("%s from cmd - %s", err.Error(), errstr))
			}

			if strings.Contains(errstr, "(core dumped)") {
				log.Error().Msg("detected coredump, exiting and hoping we get restarted - see https://github.com/helixml/helix/issues/123")
				os.Exit(1)
			}
			if strings.Contains(errstr, "CUDA is not available") {
				log.Error().Msg("detected GPU error, exiting and hoping we get restarted - see https://github.com/helixml/helix/issues/123")
				os.Exit(1)
			}

			return
		}

		log.Info().Msgf("🟢 stop model instance, exit code=%d", cmd.ProcessState.ExitCode())
	}(cmd)
	return nil
}

func (instance *ModelInstance) stopProcess() error {
	if instance.currentCommand == nil {
		return fmt.Errorf("no process to stop")
	}
	log.Info().Msgf("🟢 stop model process")
	if err := syscall.Kill(-instance.currentCommand.Process.Pid, syscall.SIGKILL); err != nil {
		log.Error().Msgf("error stopping model process: %s", err.Error())
		return err
	}
	log.Info().Msgf("🟢 stopped model process")
	return nil
}

func (instance *ModelInstance) isStale() bool {
	stale := false
	if instance.lastActivityTimestamp == 0 {
		stale = false
	} else if instance.lastActivityTimestamp+int64(instance.runnerOptions.ModelInstanceTimeoutSeconds) < time.Now().Unix() {
		stale = true
	}
	return stale
}

func (instance *ModelInstance) addJobToHistory(session *types.Session) error {
	summary, err := data.GetSessionSummary(session)
	if err != nil {
		return err
	}

	// put the job at the start of the array
	instance.jobHistory = append([]*types.SessionSummary{summary}, instance.jobHistory...)
	if len(instance.jobHistory) > instance.runnerOptions.JobHistoryBufferSize {
		instance.jobHistory = instance.jobHistory[:len(instance.jobHistory)-1]
	}

	return nil
}

func (instance *ModelInstance) getState() (*types.ModelInstanceState, error) {
	if instance.initialSession == nil {
		return nil, fmt.Errorf("no initial session")
	}
	currentSession := instance.currentSession
	if currentSession == nil {
		currentSession = instance.queuedSession
	}
	// this can happen when the session has downloaded and is ready
	// but the python is still booting up
	if currentSession == nil {
		currentSession = instance.nextSession
	}

	var sessionSummary *types.SessionSummary
	var err error

	if currentSession != nil {
		sessionSummary, err = data.GetSessionSummary(currentSession)
		if err != nil {
			return nil, err
		}
	}

	return &types.ModelInstanceState{
		ID:               instance.id,
		ModelName:        instance.initialSession.ModelName,
		Mode:             instance.initialSession.Mode,
		LoraDir:          instance.initialSession.LoraDir,
		InitialSessionID: instance.initialSession.ID,
		CurrentSession:   sessionSummary,
		JobHistory:       instance.jobHistory,
		Timeout:          int(instance.runnerOptions.ModelInstanceTimeoutSeconds),
		LastActivity:     int(instance.lastActivityTimestamp),
		Stale:            instance.isStale(),
		MemoryUsage:      instance.model.GetMemoryRequirements(instance.initialSession.Mode),
	}, nil
}
