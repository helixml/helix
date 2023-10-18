package controller

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bacalhau-project/lilypad/pkg/data"
	"github.com/bacalhau-project/lilysaas/api/pkg/filestore"
	"github.com/bacalhau-project/lilysaas/api/pkg/job"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type ControllerOptions struct {
	Store     store.Store
	Filestore filestore.FileStore
	JobRunner *job.JobRunner
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
	JobUpdatesChan     chan *types.Job
	SessionUpdatesChan chan *types.Session
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
	if options.JobRunner == nil {
		return nil, fmt.Errorf("job runner is required")
	}
	controller := &Controller{
		Ctx:                ctx,
		Options:            options,
		JobUpdatesChan:     make(chan *types.Job),
		SessionUpdatesChan: make(chan *types.Session),
	}
	return controller, nil
}

func (c *Controller) Start() error {
	go func() {
		for {
			select {
			case <-c.Ctx.Done():
				return
			case err := <-c.Options.JobRunner.ErrorChan:
				log.Error().Msgf("Lilypad error in job runner: %s", err.Error())
				return
			default:
				time.Sleep(1 * time.Second)
				err := c.loopSessions(c.Ctx)
				if err != nil {
					log.Error().Msgf("Lilypad error in controller loop: %s", err.Error())
					debug.PrintStack()
				}
				err = c.loop(c.Ctx)
				if err != nil {
					log.Error().Msgf("Lilypad error in controller loop: %s", err.Error())
					debug.PrintStack()
				}
			}
		}
	}()
	c.Options.JobRunner.Subscribe(c.Ctx, c.handleJobUpdate)
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

func (c *Controller) loopSessions(ctx context.Context) error {
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
	go runFunc(c.triggerSessionTasks)

	go func() {
		wg.Wait()
		close(errChan)
	}()

	if err := <-errChan; err != nil {
		return err
	}
	return nil
}

func (c *Controller) handleJobUpdate(evOffer data.JobOfferContainer) {
	job, err := c.Options.Store.GetJob(context.Background(), evOffer.ID)
	if err != nil {
		fmt.Printf("error loading job: %s\n", err.Error())
		return
	}
	// we have a race condition where we need to write the job to the solver to get
	// it's ID and then we might not have written the job to the database yet
	// TODO: make lilypad have a way to have deterministic ID's so we can know the
	// job ID before submitting it
	if job == nil {
		// this means the job has not been written to the database yet (probably)
		time.Sleep(time.Millisecond * 100)
		job, err = c.Options.Store.GetJob(context.Background(), evOffer.ID)
		if err != nil {
			return
		}
		if job == nil {
			fmt.Printf("job not found: %s\n", evOffer.ID)
			return
		}
	}
	jobData := job.Data
	jobData.Container = evOffer

	c.Options.Store.UpdateJob(
		c.Ctx,
		evOffer.ID,
		data.GetAgreementStateString(evOffer.State),
		"",
		jobData,
	)

	job, err = c.Options.Store.GetJob(context.Background(), evOffer.ID)
	if err != nil {
		fmt.Printf("error loading job: %s\n", err.Error())
		return
	}

	c.JobUpdatesChan <- job
}

// load all jobs that are currently running and check if they are still running
func (c *Controller) checkForRunningJobs(ctx context.Context) error {
	return nil
}
