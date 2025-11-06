package crisp

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

type Crisp struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

	botMu sync.Mutex
	bot   map[string]*CrispBot // Crisp bots for each Helix app/agent
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *Crisp {
	return &Crisp{
		cfg:        cfg,
		store:      store,
		controller: controller,
		bot:        make(map[string]*CrispBot),
	}
}

// Start - reconcile CrispBots, check which apps/agents have Crisp triggers enabled
// and start the bot for each of them. Once running, they are added into the map.
// If they get disabled, the bot will be stopped and removed from the map.
func (c *Crisp) Start(ctx context.Context) error {
	log.Info().Msg("starting Crisp bots reconciler and runner")
	// Reconcile bots
	for {
		err := c.reconcile(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to reconcile Crisp bots")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (c *Crisp) reconcile(ctx context.Context) error {
	// Get all apps from the store
	apps, err := c.store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	log.Trace().Msg("reconciling Crisp bots")

	// Find apps with Crisp triggers
	crispApps := make(map[string]*types.CrispTrigger)
	for _, app := range apps {
		for _, trigger := range app.Config.Helix.Triggers {
			if trigger.Crisp != nil && trigger.Crisp.Token != "" {
				crispApps[app.ID] = trigger.Crisp
				break
			}
		}
	}

	log.Trace().Int("crisp_apps", len(crispApps)).Msg("crisp apps")

	c.botMu.Lock()
	defer c.botMu.Unlock()

	// Stop bots that are no longer needed
	for appID, bot := range c.bot {
		if _, exists := crispApps[appID]; !exists {
			log.Info().Str("app_id", appID).Msg("stopping Crisp bot - no longer configured")
			bot.Stop()
			delete(c.bot, appID)
		}
	}

	// Start or update bots for configured apps
	for appID, crispTrigger := range crispApps {
		app, err := c.store.GetApp(ctx, appID)
		if err != nil {
			log.Error().Err(err).Str("app_id", appID).Msg("failed to get app for Crisp bot")
			continue
		}

		existingBot, exists := c.bot[appID]
		if !exists {
			// Create new bot
			log.Info().Str("app_id", appID).Msg("starting new Crisp bot")
			bot := NewCrispBot(c.cfg, c.store, c.controller, app, crispTrigger)
			c.bot[appID] = bot

			// Start the bot in a goroutine
			go func(bot *CrispBot, appID string) {
				if err := bot.RunBot(ctx); err != nil {
					log.Error().Err(err).Str("app_id", appID).Msg("Crisp bot exited with error")
					// TODO: Update controller status to error
					bot.setStatus(false, fmt.Sprintf("Crisp bot exited with error: %v", err))
				}
			}(bot, appID)
		} else {
			// Check if the trigger configuration has changed
			if !c.triggerConfigEqual(existingBot.trigger, crispTrigger) {
				log.Info().Str("app_id", appID).Msg("updating Crisp bot configuration")

				// Stop the existing bot
				existingBot.Stop()

				// Create new bot with updated configuration
				bot := NewCrispBot(c.cfg, c.store, c.controller, app, crispTrigger)
				c.bot[appID] = bot

				// Start the new bot in a goroutine
				go func(bot *CrispBot, appID string) {
					if err := bot.RunBot(ctx); err != nil {
						log.Error().Err(err).Str("app_id", appID).Msg("Crisp bot exited with error")
						bot.setStatus(false, fmt.Sprintf("Crisp bot exited with error: %v", err))
					}
				}(bot, appID)
			}
		}
	}

	return nil
}

// triggerConfigEqual compares two CrispTrigger configurations for equality
func (c *Crisp) triggerConfigEqual(a, b *types.CrispTrigger) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.Identifier == b.Identifier &&
		a.Token == b.Token && b.Nickname == a.Nickname
}

// Stop stops all running Crisp bots
func (c *Crisp) Stop() {
	c.botMu.Lock()
	defer c.botMu.Unlock()

	log.Info().Msg("stopping all Crisp bots")

	for appID, bot := range c.bot {
		log.Info().Str("app_id", appID).Msg("stopping Crisp bot")
		bot.Stop()
	}

	// Clear the bot map
	c.bot = make(map[string]*CrispBot)
}
