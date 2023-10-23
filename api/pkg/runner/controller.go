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
	ApiURL   string
	ApiToken string
	// we run a http server for children processes
	// to pull their next tasks fromm
	// what port does that server run on?
	ServerPort int
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
	// the map of models that we have loaded
	activeModels map[string]*ModelWrapper

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
	if options.ApiURL == "" {
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
			Host:  options.ApiURL,
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

// we have popped the next session from the master API
// let's create a model for it
func (r *Runner) addGlobalSession(ctx context.Context, session *types.Session) error {
	log.Info().Msgf("Add global session %s", session.ID)
	spew.Dump(session)
	model, err := NewModelWrapper(ctx, session.ModelName, session.Mode)
	model.nextSession = session
	if err != nil {
		return err
	}
	r.activeModels[model.id] = model
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

	return server.GetRequest[*types.Session](
		r.httpClientOptions,
		"/worker/task",
		map[string]string{
			"free_memory": fmt.Sprintf("%d", freeMemory),
		},
	)
}

func (r *Runner) getUsedMemory() uint64 {
	r.modelMutex.Lock()
	defer r.modelMutex.Unlock()
	memoryUsed := uint64(0)
	for _, modelWrapper := range r.activeModels {
		memoryUsed += modelWrapper.model.GetMemoryRequirements(modelWrapper.mode)
	}
	return memoryUsed
}

func (r *Runner) getFreeMemory() uint64 {
	return r.Options.MemoryBytes - r.getUsedMemory()
}
