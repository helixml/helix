package controller

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/lukemarsden/helix/api/pkg/filestore"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type ControllerOptions struct {
	Store     store.Store
	Filestore filestore.FileStore
	// this is an "env" prefix like "dev"
	// the user prefix is handled inside the controller
	// (see getFilestorePath)
	FilePrefixGlobal string
	// this is a golang template that is used to prefix the user
	// path in the filestore - it is passed Owner and OwnerType values
	// write me an example FilePrefixUser as a go template
	// e.g. "users/{{.Owner}}"
	FilePrefixUser string
	// a static path used to denote what sub-folder job results live in
	FilePrefixResults string
}

type Controller struct {
	Ctx                context.Context
	Options            ControllerOptions
	SessionUpdatesChan chan *types.Session
	sessionQueue       []*types.Session
	sessionQueueMtx    sync.Mutex
	activeSessions     map[string]*types.Session
	activeSessionMtx   sync.Mutex
}

func NewController(
	ctx context.Context,
	options ControllerOptions,
) (*Controller, error) {
	if options.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if options.Filestore == nil {
		return nil, fmt.Errorf("filestore is required")
	}
	controller := &Controller{
		Ctx:                ctx,
		Options:            options,
		SessionUpdatesChan: make(chan *types.Session),
		activeSessions:     map[string]*types.Session{},
		sessionQueue:       []*types.Session{},
	}
	return controller, nil
}

func (c *Controller) Start() error {
	err := c.loadSessionQueues(c.Ctx)
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-c.Ctx.Done():
				return
			default:
				time.Sleep(10 * time.Second)
				err := c.loop(c.Ctx)
				if err != nil {
					log.Error().Msgf("Lilypad error in controller loop: %s", err.Error())
					debug.PrintStack()
				}
			}
		}
	}()
	return nil
}

func (c *Controller) loop(ctx context.Context) error {
	// var wg sync.WaitGroup
	// errChan := make(chan error, 1)

	// // Wrap the function in a closure and handle the WaitGroup and error channel
	// runFunc := func(f func(context.Context) error) {
	// 	defer wg.Done()
	// 	if err := f(ctx); err != nil {
	// 		select {
	// 		case errChan <- err:
	// 		default:
	// 		}
	// 	}
	// }

	// wg.Add(1)

	// // an example of a function that is called in the loop
	// go runFunc(c.reloadSessionQueues)

	// go func() {
	// 	wg.Wait()
	// 	close(errChan)
	// }()

	// if err := <-errChan; err != nil {
	// 	return err
	// }
	return nil
}
