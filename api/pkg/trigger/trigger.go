package trigger

import (
	"context"
	"fmt"
	"net/http"
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
	"github.com/helixml/helix/api/pkg/trigger/notion"
	"github.com/helixml/helix/api/pkg/trigger/project"
	"github.com/helixml/helix/api/pkg/trigger/slack"
	"github.com/helixml/helix/api/pkg/trigger/teams"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

type Manager struct {
	cfg                  *config.ServerConfig
	store                store.Store
	controller           *controller.Controller
	notifier             notification.Notifier
	specTaskCreator      cron.SpecTaskCreator
	externalAgentStarter cron.ExternalAgentStarter
	azureDevOps          *azure.AzureDevOps
	teams                *teams.Teams
	helixCodeReview      *project.HelixCodeReviewTrigger
	notion               *notion.Notion

	wg sync.WaitGroup
}

func NewTriggerManager(cfg *config.ServerConfig, store store.Store, notifier notification.Notifier, controller *controller.Controller) *Manager {
	m := &Manager{
		cfg:             cfg,
		store:           store,
		controller:      controller,
		notifier:        notifier,
		azureDevOps:     azure.New(cfg, store, controller),
		teams:           teams.New(cfg, store, controller),
		helixCodeReview: project.New(cfg, store, controller),
	}
	// Notion gets a partially-constructed handler — the spec-task creator is
	// wired in via SetSpecTaskCreator after the SpecDrivenTaskService exists.
	// Until then, create events return a clear error rather than panicking.
	m.notion = notion.New(cfg, store, nil, nil, nil, &defaultEmbedURLBuilder{cfg: cfg})
	return m
}

// SetSpecTaskCreator sets the spec task creator for cron triggers that use the "spec_task" action.
// This is set after construction because the SpecDrivenTaskService is created later in the init sequence.
func (t *Manager) SetSpecTaskCreator(specTaskCreator cron.SpecTaskCreator) {
	t.specTaskCreator = specTaskCreator
	// Re-wire the Notion handler now that the spec-task creator is available.
	// Cancel + lookup hooks are wired separately by the spec-task service
	// once it implements them; for the MVP they remain nil and the trigger
	// degrades gracefully.
	t.notion = notion.New(t.cfg, t.store, specTaskCreator, nil, nil, &defaultEmbedURLBuilder{cfg: t.cfg})
}

// defaultEmbedURLBuilder produces the embed URL pasted into Notion pages by
// the OnExternalCreate hook. URL pattern matches the EmbedTaskPage route in
// the frontend (see frontend/src/router.tsx).
type defaultEmbedURLBuilder struct {
	cfg *config.ServerConfig
}

func (b *defaultEmbedURLBuilder) BuildEmbedURL(spectask *types.SpecTask, accessToken string) string {
	base := b.cfg.WebServer.URL
	if base == "" {
		return ""
	}
	url := fmt.Sprintf("%s/embed/task/%s", base, spectask.ID)
	if accessToken != "" {
		url += "?access_token=" + accessToken
	}
	return url
}

// Notion returns the Notion trigger handler for direct invocation by the
// spec-task service's completion / cancellation hooks (not currently wired
// — see TODO in spec_driven_task_service for where this would land).
func (t *Manager) Notion() *notion.Notion {
	return t.notion
}

func (t *Manager) SetExternalAgentStarter(starter cron.ExternalAgentStarter) {
	t.externalAgentStarter = starter
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

func (t *Manager) ProcessWebhook(ctx context.Context, triggerConfig *types.TriggerConfiguration, headers http.Header, payload []byte) error {
	switch {
	case triggerConfig.Trigger.AzureDevOps != nil:
		return t.azureDevOps.ProcessWebhook(ctx, triggerConfig, payload)
	case triggerConfig.Trigger.Notion != nil:
		return t.notion.ProcessWebhook(ctx, triggerConfig, headers, payload)
	default:
		log.Error().Any("trigger_config", triggerConfig).Msg("unknown trigger type")
		return fmt.Errorf("unknown trigger type")
	}
}

func (t *Manager) ProcessGitPushEvent(ctx context.Context, specTask *types.SpecTask, repo *types.GitRepository, commitHash string) error {
	return t.helixCodeReview.ProcessGitPushEvent(ctx, specTask, repo, commitHash)
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
	cronTrigger, err := cron.New(t.cfg, t.store, t.notifier, t.controller, t.specTaskCreator, t.externalAgentStarter)
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
