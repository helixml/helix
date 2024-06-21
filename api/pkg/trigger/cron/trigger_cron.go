package cron

import (
	"context"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
)

type Cron struct {
	cfg   *config.ServerConfig
	store store.Store
}

func New(cfg *config.ServerConfig, store store.Store) *Cron {
	return &Cron{
		cfg:   cfg,
		store: store,
	}
}

func (c *Cron) Start(ctx context.Context) error {
	// TODO: implement cron trigger
	<-ctx.Done()
	return nil
}
