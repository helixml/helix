package trigger

import (
	"context"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/trigger/cron"
	"github.com/helixml/helix/api/pkg/trigger/discord"
	"github.com/helixml/helix/api/pkg/trigger/slack"

	"github.com/rs/zerolog/log"
)

type TriggerManager struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller
	wg         sync.WaitGroup
}

func NewTriggerManager(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *TriggerManager {
	return &TriggerManager{
		cfg:        cfg,
		store:      store,
		controller: controller,
	}
}

func (t *TriggerManager) Start(ctx context.Context) {

	log.Info().Msg("starting Helix triggers")

	if t.cfg.Triggers.Discord.Enabled && t.cfg.Triggers.Discord.BotToken != "" {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runDiscord(ctx)
		}()
	}

	if t.cfg.Triggers.Slack.Enabled && t.cfg.Triggers.Slack.BotToken != "" {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runSlack(ctx)
		}()
	}

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runCron(ctx)
	}()

	t.wg.Wait()
}

func (t *TriggerManager) runSlack(ctx context.Context) {
	slackTrigger := slack.New(t.cfg, t.store, t.controller)

	for {
		err := slackTrigger.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start slack trigger, retrying in 10 seconds")
		}
	}
}

func (t *TriggerManager) runDiscord(ctx context.Context) {
	discordTrigger := discord.New(t.cfg, t.store, t.controller)

	for {
		err := discordTrigger.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start Discord trigger, retrying in 10 seconds")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (t *TriggerManager) runCron(ctx context.Context) {
	cronTrigger := cron.New(t.cfg, t.store)

	for {
		err := cronTrigger.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start cron trigger, retrying in 10 seconds")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}
