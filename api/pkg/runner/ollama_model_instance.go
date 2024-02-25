package runner

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

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
	return nil, nil
}

func (i *OllamaModelInstance) SetNextSession(session *types.Session) {
	i.nextSession = session
}

func (i *OllamaModelInstance) QueueSession(session *types.Session, isInitialSession bool) {

}

func (i *OllamaModelInstance) GetQueuedSession() *types.Session {
	return nil
}

func (i *OllamaModelInstance) Start(session *types.Session) error {
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

func (i *OllamaModelInstance) Done() <-chan bool {
	return i.finishCh
}
