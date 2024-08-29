package knowledge

import (
	"context"
	"fmt"

	gocron "github.com/go-co-op/gocron/v2"
)

func (r *Reconciler) startCron(ctx context.Context) error {
	s, err := gocron.NewScheduler()
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	// start the scheduler
	s.Start()

	// Block until the context is done
	<-ctx.Done()

	// when you're done, shut it down
	err = s.Shutdown()
	if err != nil {
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}

	return nil
}

func (r *Reconciler) reconcileCronJobs(ctx context.Context) error {
	return nil
}
