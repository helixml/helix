package slack

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

type Slack struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

	// TODO: add method to controller to set trigger status, for example:
	// - bot is running
	// - bot is stopped (trigger disabled)
	// - bot is erroring, such as API key is invalid

	botMu sync.Mutex
	bot   map[string]*SlackBot // Slack bots for each Helix app/agent
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *Slack {
	return &Slack{
		cfg:        cfg,
		store:      store,
		controller: controller,
		bot:        make(map[string]*SlackBot),
	}
}

// Start - reconcile SlackBots, check which apps/agents have Slack triggers enabled
// and start the bot for each of them. Once running, they are added into the map.
// If they get disabled, the bot will be stopped and removed from the map.
func (s *Slack) Start(ctx context.Context) error {
	// Reconcile bots
	for {
		err := s.reconcile(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to reconcile Slack bots")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (s *Slack) reconcile(ctx context.Context) error {
	// Get all apps from the store
	apps, err := s.store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	// Find apps with Slack triggers
	slackApps := make(map[string]*types.SlackTrigger)
	for _, app := range apps {
		for _, trigger := range app.Config.Helix.Triggers {
			if trigger.Slack != nil && trigger.Slack.BotToken != "" {
				slackApps[app.ID] = trigger.Slack
				break
			}
		}
	}

	s.botMu.Lock()
	defer s.botMu.Unlock()

	// Stop bots that are no longer needed
	for appID, bot := range s.bot {
		if _, exists := slackApps[appID]; !exists {
			log.Info().Str("app_id", appID).Msg("stopping Slack bot - no longer configured")
			bot.Stop()
			delete(s.bot, appID)
		}
	}

	// Start or update bots for configured apps
	for appID, slackTrigger := range slackApps {
		app, err := s.store.GetApp(ctx, appID)
		if err != nil {
			log.Error().Err(err).Str("app_id", appID).Msg("failed to get app for Slack bot")
			continue
		}

		existingBot, exists := s.bot[appID]
		if !exists {
			// Create new bot
			log.Info().Str("app_id", appID).Msg("starting new Slack bot")
			bot := NewSlackBot(s.cfg, s.store, s.controller, app, slackTrigger)
			s.bot[appID] = bot

			// Start the bot in a goroutine
			go func(bot *SlackBot, appID string) {
				if err := bot.RunBot(ctx); err != nil {
					log.Error().Err(err).Str("app_id", appID).Msg("Slack bot exited with error")
					// TODO: Update controller status to error
					bot.setStatus(false, fmt.Sprintf("Slack bot exited with error: %v", err))
				}
			}(bot, appID)
		} else {
			// Check if the trigger configuration has changed
			if !s.triggerConfigEqual(existingBot.trigger, slackTrigger) {
				log.Info().Str("app_id", appID).Msg("updating Slack bot configuration")

				// Stop the existing bot
				existingBot.Stop()

				// Create new bot with updated configuration
				bot := NewSlackBot(s.cfg, s.store, s.controller, app, slackTrigger)
				s.bot[appID] = bot

				// Start the new bot in a goroutine
				go func(bot *SlackBot, appID string) {
					if err := bot.RunBot(ctx); err != nil {
						log.Error().Err(err).Str("app_id", appID).Msg("Slack bot exited with error")
						bot.setStatus(false, fmt.Sprintf("Slack bot exited with error: %v", err))
					}
				}(bot, appID)
			}
		}
	}

	return nil
}

// triggerConfigEqual compares two SlackTrigger configurations for equality
func (s *Slack) triggerConfigEqual(a, b *types.SlackTrigger) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.AppToken == b.AppToken &&
		a.BotToken == b.BotToken &&
		slicesEqual(a.Channels, b.Channels)
}

// Stop stops all running Slack bots
func (s *Slack) Stop() {
	s.botMu.Lock()
	defer s.botMu.Unlock()

	log.Info().Msg("stopping all Slack bots")

	for appID, bot := range s.bot {
		log.Info().Str("app_id", appID).Msg("stopping Slack bot")
		bot.Stop()
	}

	// Clear the bot map
	s.bot = make(map[string]*SlackBot)
}

// slicesEqual compares two string slices for equality
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
