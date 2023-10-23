package runner

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type RunnerOptions struct {
	ApiURL string
	// how many bytes of memory does our GPU have?
	// we report this back to the api when we ask
	// for the global next task (well, this minus the
	// currently running models)
	Memory uint64
}

type Runner struct {
	Ctx     context.Context
	Options RunnerOptions

	modelMutex sync.Mutex
	// the map of models that we have loaded
	activeModels map[string]model.Model

	// SessionUpdatesChan chan *types.Session
	// // the backlog of sessions that need a GPU
	// sessionQueue    []*types.Session
	// sessionQueueMtx sync.Mutex
	// // the map of active sessions that are currently running on a GPU
	// activeSessions   map[string]*types.Session
	// activeSessionMtx sync.Mutex

	// // the map of text streams attached to a session
	// // not all sessions will have an active text stream
	// // it depends what type the session is
	// activeTextStreams    map[string]*model.TextStream
	// activeTextStreamsMtx sync.Mutex
}

func NewRunner(
	ctx context.Context,
	options RunnerOptions,
) (*Runner, error) {
	if options.ApiURL == "" {
		return nil, fmt.Errorf("api url is required")
	}
	if options.Memory == 0 {
		return nil, fmt.Errorf("memory is required")
	}
	runner := &Runner{
		Ctx:     ctx,
		Options: options,
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
				log.Error().Msgf("Lilypad error in controller loop: %s", err.Error())
				debug.PrintStack()
			}
		}
	}
}

func (r *Runner) loop(ctx context.Context) error {
	fmt.Printf("runner loop --------------------------------------\n")
	return nil
}

// ask the master API if they have the next session for us
// we check with the various models and filter based on the currently free memory
// we pass that free memory back to the master API - it will filter out any tasks
// for models that would require more memory than we have available
func (r *Runner) getNextGlobalSession(ctx context.Context) (*types.Session, error) {
	return nil, nil
}

// func (r *Runner) getUsedMemory() uint64 {
// 	r.modelMutex.Lock()
// 	defer r.modelMutex.Unlock()
// 	memoryUsed := uint64(0)
// 	for _, model := range r.activeModels {
// 		memoryUsed += model.GetMemoryRequirements()
// 	}
// 	return memoryUsed
// }

// func (r *Runner) getFreeMemory() uint64 {
// 	return r.Options.Memory - r.getUsedMemory()
// }
