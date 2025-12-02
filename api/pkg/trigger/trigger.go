package trigger

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/trigger/azure"
	"github.com/helixml/helix/api/pkg/trigger/crisp"
	"github.com/helixml/helix/api/pkg/trigger/cron"
	"github.com/helixml/helix/api/pkg/trigger/discord"
	"github.com/helixml/helix/api/pkg/trigger/slack"
	"github.com/helixml/helix/api/pkg/trigger/teams"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

type Manager struct {
	cfg         *config.ServerConfig
	store       store.Store
	controller  *controller.Controller
	notifier    notification.Notifier
	azureDevOps *azure.AzureDevOps
	teams       *teams.Teams

	wg sync.WaitGroup
}

func NewTriggerManager(cfg *config.ServerConfig, store store.Store, notifier notification.Notifier, controller *controller.Controller) *Manager {
	return &Manager{
		cfg:         cfg,
		store:       store,
		controller:  controller,
		notifier:    notifier,
		azureDevOps: azure.New(cfg, store, controller),
		teams:       teams.New(cfg, store, controller),
	}
}

func (t *Manager) Start(ctx context.Context) {

	log.Info().Msg("starting Helix triggers")

	if t.cfg.Triggers.Discord.Enabled && t.cfg.Triggers.Discord.BotToken != "" {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runDiscord(ctx)
		}()
	}

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runCron(ctx)
	}()

	if t.cfg.Triggers.Slack.Enabled {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runSlack(ctx)
		}()
	}

	if t.cfg.Triggers.Crisp.Enabled {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runCrisp(ctx)
		}()
	}

	if t.cfg.Triggers.Teams.Enabled {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runTeams(ctx)
		}()
	}

	t.wg.Wait()
}

func (t *Manager) ProcessWebhook(ctx context.Context, triggerConfig *types.TriggerConfiguration, payload []byte) error {
	switch {
	case triggerConfig.Trigger.AzureDevOps != nil:
		return t.azureDevOps.ProcessWebhook(ctx, triggerConfig, payload)
	default:
		log.Error().Any("trigger_config", triggerConfig).Msg("unknown trigger type")
		return fmt.Errorf("unknown trigger type")
	}
}

func (t *Manager) runDiscord(ctx context.Context) {
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

func (t *Manager) runCron(ctx context.Context) {
	cronTrigger, err := cron.New(t.cfg, t.store, t.notifier, t.controller)
	if err != nil {
		log.Err(err).Msg("failed to create cron trigger")
		return
	}

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

func (t *Manager) runSlack(ctx context.Context) {
	slackTrigger := slack.New(t.cfg, t.store, t.controller)

	for {
		err := slackTrigger.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start slack trigger, retrying in 10 seconds")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (t *Manager) runCrisp(ctx context.Context) {
	crispTrigger := crisp.New(t.cfg, t.store, t.controller)

	for {
		err := crispTrigger.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start crisp trigger, retrying in 10 seconds")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (t *Manager) runTeams(ctx context.Context) {
	for {
		err := t.teams.Start(ctx)
		if err != nil {
			log.Err(err).Msg("failed to start teams trigger, retrying in 10 seconds")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// GetTeamsBot returns a Teams bot by app ID (used by webhook handler)
func (t *Manager) GetTeamsBot(appID string) *teams.TeamsBot {
	return t.teams.GetBot(appID)
}
