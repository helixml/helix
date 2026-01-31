package teams

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

type Teams struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

	botMu sync.Mutex
	bot   map[string]*TeamsBot // Teams bots for each Helix app/agent
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *Teams {
	return &Teams{
		cfg:        cfg,
		store:      store,
		controller: controller,
		bot:        make(map[string]*TeamsBot),
	}
}

// Start - reconcile TeamsBots, check which apps/agents have Teams triggers enabled
// and start the bot for each of them. Once running, they are added into the map.
// If they get disabled, the bot will be stopped and removed from the map.
func (t *Teams) Start(ctx context.Context) error {
	// Reconcile bots
	for {
		err := t.reconcile(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to reconcile Teams bots")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (t *Teams) reconcile(ctx context.Context) error {
	// Get all apps from the store
	apps, err := t.store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	// Find apps with Teams triggers
	teamsApps := make(map[string]*types.TeamsTrigger)
	for _, app := range apps {
		for _, trigger := range app.Config.Helix.Triggers {
			if trigger.Teams != nil && trigger.Teams.AppID != "" && trigger.Teams.AppPassword != "" {
				teamsApps[app.ID] = trigger.Teams
				break
			}
		}
	}

	t.botMu.Lock()
	defer t.botMu.Unlock()

	// Stop bots that are no longer needed
	for appID, bot := range t.bot {
		if _, exists := teamsApps[appID]; !exists {
			log.Info().Str("app_id", appID).Msg("stopping Teams bot - no longer configured")
			bot.Stop()
			delete(t.bot, appID)
		}
	}

	// Start or update bots for configured apps
	for appID, teamsTrigger := range teamsApps {
		app, err := t.store.GetApp(ctx, appID)
		if err != nil {
			log.Error().Err(err).Str("app_id", appID).Msg("failed to get app for Teams bot")
			continue
		}

		existingBot, exists := t.bot[appID]
		if !exists {
			// Create new bot
			log.Info().Str("app_id", appID).Msg("starting new Teams bot")
			bot := NewTeamsBot(t.cfg, t.store, t.controller, app, teamsTrigger)
			t.bot[appID] = bot

			// Start the bot in a goroutine
			go func(bot *TeamsBot, appID string) {
				if err := bot.Start(ctx); err != nil {
					log.Error().Err(err).Str("app_id", appID).Msg("Teams bot exited with error")
					bot.setStatus(false, fmt.Sprintf("Teams bot exited with error: %v", err))
				}
			}(bot, appID)
		} else {
			// Check if the trigger configuration has changed
			if !t.triggerConfigEqual(existingBot.trigger, teamsTrigger) {
				log.Info().Str("app_id", appID).Msg("updating Teams bot configuration")

				// Stop the existing bot
				existingBot.Stop()

				// Create new bot with updated configuration
				bot := NewTeamsBot(t.cfg, t.store, t.controller, app, teamsTrigger)
				t.bot[appID] = bot

				// Start the new bot in a goroutine
				go func(bot *TeamsBot, appID string) {
					if err := bot.Start(ctx); err != nil {
						log.Error().Err(err).Str("app_id", appID).Msg("Teams bot exited with error")
						bot.setStatus(false, fmt.Sprintf("Teams bot exited with error: %v", err))
					}
				}(bot, appID)
			}
		}
	}

	return nil
}

// triggerConfigEqual compares two TeamsTrigger configurations for equality
func (t *Teams) triggerConfigEqual(a, b *types.TeamsTrigger) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.AppID == b.AppID &&
		a.AppPassword == b.AppPassword &&
		a.TenantID == b.TenantID
}

// Stop stops all running Teams bots
func (t *Teams) Stop() {
	t.botMu.Lock()
	defer t.botMu.Unlock()

	log.Info().Msg("stopping all Teams bots")

	for appID, bot := range t.bot {
		log.Info().Str("app_id", appID).Msg("stopping Teams bot")
		bot.Stop()
	}

	// Clear the bot map
	t.bot = make(map[string]*TeamsBot)
}

// GetBot returns a Teams bot by app ID (used by webhook handler)
func (t *Teams) GetBot(appID string) *TeamsBot {
	t.botMu.Lock()
	defer t.botMu.Unlock()
	return t.bot[appID]
}
