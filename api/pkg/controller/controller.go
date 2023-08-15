package controller

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bacalhau-project/lilysaas/api/pkg/contract"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/rs/zerolog/log"
)

type ControllerOptions struct {
	Contract contract.Contract
	Store    store.Store
}

type Controller struct {
	Contract contract.Contract
	Store    store.Store
}

func NewController(
	options ControllerOptions,
) (*Controller, error) {
	controller := &Controller{
		Contract: options.Contract,
		Store:    options.Store,
	}
	return controller, nil
}

func (c *Controller) Start(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(1 * time.Second)
				err := c.loop(ctx)
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

	go runFunc(c.checkForNewJobs)

	go func() {
		wg.Wait()
		close(errChan)
	}()

	if err := <-errChan; err != nil {
		return err
	}
	return nil
}
