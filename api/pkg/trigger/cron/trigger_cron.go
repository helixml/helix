package cron

import (
	"context"
	"fmt"

	"github.com/go-co-op/gocron/v2"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type Cron struct {
	cfg   *config.ServerConfig
	store store.Store
	cron  gocron.Scheduler
}

func New(cfg *config.ServerConfig, store store.Store) (*Cron, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	return &Cron{
		cfg:   cfg,
		store: store,
		cron:  s,
	}, nil
}

func (c *Cron) Start(ctx context.Context) error {
	// start the scheduler
	c.cron.Start()

	// Block until the context is done
	<-ctx.Done()

	// when you're done, shut it down
	err := c.cron.Shutdown()
	if err != nil {
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}

	return nil
}

func (c *Cron) reconcileCronApps(ctx context.Context) error {
	apps, err := c.store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

}

func (c *Cron) listApps(ctx context.Context) ([]*types.App, error) {
	apps, err := c.store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}

	var filteredApps []*types.App

	for _, app := range apps {
		for _, trigger := range app.Config.Helix.Triggers {
			if trigger.Cron != nil && trigger.Cron.Schedule != "" {
				filteredApps = append(filteredApps, app)
			}
		}
	}

	return filteredApps, nil
}
