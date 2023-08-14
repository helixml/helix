package controller

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bacalhau-project/lilysaas/api/pkg/bacalhau"
	"github.com/bacalhau-project/lilysaas/api/pkg/contract"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type ControllerOptions struct {
	AppURL         string
	FilestoreToken string
	Bacalhau       bacalhau.Bacalhau
	Contract       contract.Contract
	Store          store.Store
}

type Controller struct {
	AppURL         string
	FilestoreToken string
	Bacalhau       bacalhau.Bacalhau
	Contract       contract.Contract
	Store          store.Store
	imageChan      chan<- *types.ImageCreatedEvent
	artistChan     chan<- *types.ArtistCreatedEvent
}

func NewController(
	options ControllerOptions,
) (*Controller, error) {
	if options.AppURL == "" {
		return nil, fmt.Errorf("app url is required")
	}
	if options.FilestoreToken == "" {
		return nil, fmt.Errorf("filestore token is required")
	}
	controller := &Controller{
		AppURL:         options.AppURL,
		FilestoreToken: options.FilestoreToken,
		Bacalhau:       options.Bacalhau,
		Contract:       options.Contract,
		Store:          options.Store,
		imageChan:      make(chan *types.ImageCreatedEvent),
		artistChan:     make(chan *types.ArtistCreatedEvent),
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
					log.Error().Msgf("Waterlily error in controller loop: %s", err.Error())
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

	wg.Add(6)

	go runFunc(c.checkForNewArtists)
	go runFunc(c.checkForRunningArtists)
	go runFunc(c.checkForFinishedArtists)
	go runFunc(c.checkForNewImages)
	go runFunc(c.checkForRunningImages)
	go runFunc(c.checkForFinishedImages)

	go func() {
		wg.Wait()
		close(errChan)
	}()

	if err := <-errChan; err != nil {
		return err
	}
	return nil
}
