package runner

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/inhies/go-bytesize"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/server"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type RunnerOptions struct {
	ApiHost  string
	ApiToken string
	// how long without running a job before we close a model instance
	ModelInstanceTimeoutSeconds int
	// how many bytes of memory does our GPU have?
	// we report this back to the api when we ask
	// for the global next task (well, this minus the
	// currently running models)
	MemoryBytes uint64
	// if this is defined then we convert it usng
	// github.com/inhies/go-bytesize
	MemoryString string
}

type Runner struct {
	Ctx     context.Context
	Options RunnerOptions

	httpClientOptions server.ClientOptions

	modelMutex sync.Mutex
	// the map of model instances that we have loaded
	// and are currently running
	activeModelInstances map[string]*ModelInstance

	// the lowest amount of memory that something can run with
	// if we have less than this amount of memory then there is
	// no point asking for more top level tasks
	// we get this on boot by asking the model package
	lowestMemoryRequirement uint64
}

func NewRunner(
	ctx context.Context,
	options RunnerOptions,
) (*Runner, error) {
	if options.ApiHost == "" {
		return nil, fmt.Errorf("api url is required")
	}
	if options.ApiToken == "" {
		return nil, fmt.Errorf("api token is required")
	}
	if options.MemoryString != "" {
		bytes, err := bytesize.Parse(options.MemoryString)
		if err != nil {
			return nil, err
		}
		options.MemoryBytes = uint64(bytes)
	}
	if options.MemoryBytes == 0 {
		return nil, fmt.Errorf("memory is required")
	}
	lowestMemoryRequirement, err := model.GetLowestMemoryRequirement()
	if err != nil {
		return nil, err
	}
	runner := &Runner{
		Ctx:                     ctx,
		Options:                 options,
		lowestMemoryRequirement: lowestMemoryRequirement,
		httpClientOptions: server.ClientOptions{
			Host:  options.ApiHost,
			Token: options.ApiToken,
		},
	}
	return runner, nil
}

func (r *Runner) Start() error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Ctx.Done():
			return nil
		case <-ticker.C:
			err := r.loop(r.Ctx)
			if err != nil {
				log.Error().Msgf("error in runner loop: %s", err.Error())
				debug.PrintStack()
			}
		}
	}
}

func (r *Runner) loop(ctx context.Context) error {
	r.modelMutex.Lock()
	defer r.modelMutex.Unlock()

	// check for running model instances that have not seen a job in a while
	// and kill them if they are over the timeout
	// TODO: get the timeout to be configurable from the api and so dynamic
	// based on load
	err := r.checkForStaleModelInstances(ctx, time.Second*time.Duration(r.Options.ModelInstanceTimeoutSeconds))
	// ask the api server if it currently has any work based on our free memory
	session, err := r.getNextGlobalSession(ctx)
	if err != nil {
		return err
	}
	if session != nil {
		err = r.addGlobalSession(ctx, session)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) checkForStaleModelInstances(ctx context.Context, timeout time.Duration) error {
	for _, activeModelInstance := range r.activeModelInstances {
		if activeModelInstance.lastJobCompletedTimestamp+int64(timeout.Seconds()) < time.Now().Unix() {
			log.Info().Msgf("Killing stale model instance %s", activeModelInstance.id)
			err := activeModelInstance.stop()
			if err != nil {
				log.Error().Msgf("error stopping model instance %s: %s", activeModelInstance.id, err.Error())
				continue
			}
			delete(r.activeModelInstances, activeModelInstance.id)
		}
	}
	return nil
}

// we have popped the next session from the master API
// let's create a model for it
func (r *Runner) addGlobalSession(ctx context.Context, session *types.Session) error {
	log.Info().Msgf("Add global session %s", session.ID)
	spew.Dump(session)
	model, err := NewModelInstance(ctx, session.ModelName, session.Mode)
	model.nextSession = session
	if err != nil {
		return err
	}
	r.activeModelInstances[model.id] = model
	return nil
}

// ask the master API if they have the next session for us
// we check with the various models and filter based on the currently free memory
// we pass that free memory back to the master API - it will filter out any tasks
// for models that would require more memory than we have available
func (r *Runner) getNextGlobalSession(ctx context.Context) (*types.Session, error) {
	freeMemory := r.getFreeMemory()

	if freeMemory < r.lowestMemoryRequirement {
		// we don't have enough memory to run anything
		// so we just wait for more memory to become available
		return nil, nil
	}

	// TODO: we should send a filter to the api endpoint
	// that lists the currently running model instances
	// and say something like "de-prioritise these models"
	// this is to prevent us randomly allocating instances
	// for all the same type that means other job types
	// never get a chance to run
	return server.GetRequest[*types.Session](
		r.httpClientOptions,
		"/worker/task",
		map[string]string{
			"memory": fmt.Sprintf("%d", freeMemory),
		},
	)
}

func (r *Runner) getUsedMemory() uint64 {
	r.modelMutex.Lock()
	defer r.modelMutex.Unlock()
	memoryUsed := uint64(0)
	for _, modelInstance := range r.activeModelInstances {
		memoryUsed += modelInstance.model.GetMemoryRequirements(modelInstance.filter.Mode)
	}
	return memoryUsed
}

func (r *Runner) getFreeMemory() uint64 {
	return r.Options.MemoryBytes - r.getUsedMemory()
}

// given a running model instance id
// get the next session that it should run
// this is either the nextSession property on the instance
// or it's what is returned by the master API server (if anything)
// this function being called means "I am ready for more work"
// because the child processes are blocking - the child will not be
// asking for more work until it's ready to accept and run it
func (r *Runner) getNextInstanceSession(ctx context.Context, instanceID string) (*types.Session, error) {
	r.modelMutex.Lock()
	defer r.modelMutex.Unlock()
	if instanceID == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	modelInstance, ok := r.activeModelInstances[instanceID]
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	// we've already got a session lined up
	if modelInstance.nextSession != nil {
		modelInstance.currentSession = modelInstance.nextSession
		modelInstance.nextSession = nil
		return modelInstance.currentSession, nil
	}

	// we currently don't have any more work so let's ask the master api if it has some
	session, err := server.GetRequest[*types.Session](
		r.httpClientOptions,
		"/worker/task",
		map[string]string{
			"model_name": string(modelInstance.filter.ModelName),
			"mode":       string(modelInstance.filter.Mode),
		},
	)

	if err != nil {
		return nil, err
	}

	return session, nil
}
