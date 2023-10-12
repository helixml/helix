package controller

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bacalhau-project/lilysaas/api/pkg/job"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type ControllerOptions struct {
	Store     store.Store
	JobRunner *job.JobRunner
}

type Controller struct {
	Ctx            context.Context
	Store          store.Store
	JobRunner      *job.JobRunner
	JobUpdatesChan chan *types.Job
}

func NewController(
	ctx context.Context,
	options ControllerOptions,
) (*Controller, error) {
	controller := &Controller{
		Ctx:            ctx,
		Store:          options.Store,
		JobRunner:      options.JobRunner,
		JobUpdatesChan: make(chan *types.Job),
	}
	return controller, nil
}

func (c *Controller) Start() error {
	go func() {
		for {
			select {
			case <-c.Ctx.Done():
				return
			case err := <-c.JobRunner.ErrorChan:
				log.Error().Msgf("Lilypad error in job runner: %s", err.Error())
				return
			default:
				time.Sleep(1 * time.Second)
				err := c.loop(c.Ctx)
				if err != nil {
					log.Error().Msgf("Lilypad error in controller loop: %s", err.Error())
					debug.PrintStack()
				}
			}
		}
	}()
	c.JobRunner.Subscribe(c.Ctx, c.handleJobUpdate)
	return nil
}

func (c *Controller) loop(ctx context.Context) error {
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	// Wrap the function in a closure and handle the WaitGroup and error channel
	runFunc := func(f func(context.Context) error) {
		defer wg.Done()
		if err := f(ctx); err != nil {
			select {
			case errChan <- err:
			default:
			}
		}
	}

	wg.Add(1)

	// an example of a function that is called in the loop
	go runFunc(c.checkForRunningJobs)

	go func() {
		wg.Wait()
		close(errChan)
	}()

	if err := <-errChan; err != nil {
		return err
	}
	return nil
}
