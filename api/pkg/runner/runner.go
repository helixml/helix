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
}

type Runner struct {
	Ctx                context.Context
	Options            RunnerOptions
	SessionUpdatesChan chan *types.Session
	// the backlog of sessions that need a GPU
	sessionQueue    []*types.Session
	sessionQueueMtx sync.Mutex
	// the map of active sessions that are currently running on a GPU
	activeSessions   map[string]*types.Session
	activeSessionMtx sync.Mutex

	// the map of text streams attached to a session
	// not all sessions will have an active text stream
	// it depends what type the session is
	activeTextStreams    map[string]*model.TextStream
	activeTextStreamsMtx sync.Mutex
}

func NewRunner(
	ctx context.Context,
	options RunnerOptions,
) (*Runner, error) {
	if options.ApiURL == "" {
		return nil, fmt.Errorf("api url is required")
	}
	runner := &Runner{
		Ctx:                ctx,
		Options:            options,
		SessionUpdatesChan: make(chan *types.Session),
		activeSessions:     map[string]*types.Session{},
		activeTextStreams:  map[string]*model.TextStream{},
		sessionQueue:       []*types.Session{},
	}
	return runner, nil
}

func (r *Runner) Start() error {
	ticker := time.NewTicker(10 * time.Second)
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
	return nil
}
