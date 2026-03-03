package telegram

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/rs/zerolog/log"
)

// Telegram manages bot instances, deduplicating by effective token.
// Only one long-polling connection per unique bot token (Telegram API constraint).
type Telegram struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

	botMu sync.Mutex
	bots  map[string]*TelegramBot // key = effective bot token
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *Telegram {
	return &Telegram{
		cfg:        cfg,
		store:      store,
		controller: controller,
		bots:       make(map[string]*TelegramBot),
	}
}

// Start reconciles TelegramBots, grouping apps by effective token
// and starting one bot per unique token.
func (t *Telegram) Start(ctx context.Context) error {
	log.Info().Msg("starting Telegram bots reconciler and runner")

	for {
		err := t.reconcile(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to reconcile Telegram bots")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (t *Telegram) reconcile(ctx context.Context) error {
	apps, err := t.store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	log.Trace().Msg("reconciling Telegram bots")

	globalToken := t.cfg.Triggers.Telegram.BotToken

	// Group apps by effective token
	// key = effective token, value = map[appID] -> appContext
	tokenGroups := make(map[string]map[string]*appContext)

	for _, app := range apps {
		for _, trigger := range app.Config.Helix.Triggers {
			if trigger.Telegram == nil || !trigger.Telegram.Enabled {
				continue
			}

			effectiveToken := trigger.Telegram.BotToken
			if effectiveToken == "" {
				effectiveToken = globalToken
			}
			if effectiveToken == "" {
				continue // No token available
			}

			if tokenGroups[effectiveToken] == nil {
				tokenGroups[effectiveToken] = make(map[string]*appContext)
			}
			tokenGroups[effectiveToken][app.ID] = &appContext{
				app:     app,
				trigger: trigger.Telegram,
			}
			break // Only one Telegram trigger per app
		}
	}

	log.Trace().Int("token_groups", len(tokenGroups)).Msg("telegram token groups")

	t.botMu.Lock()
	defer t.botMu.Unlock()

	// Stop bots for tokens that no longer have any apps
	for token, bot := range t.bots {
		if _, exists := tokenGroups[token]; !exists {
			log.Info().Msg("stopping Telegram bot - no apps configured for this token")
			bot.Stop()
			delete(t.bots, token)
		}
	}

	// Start or update bots for each token group
	for token, appContexts := range tokenGroups {
		existingBot, exists := t.bots[token]
		if !exists {
			log.Info().Int("app_count", len(appContexts)).Msg("starting new Telegram bot")
			bot := NewTelegramBot(t.cfg, t.store, t.controller, token, appContexts)
			t.bots[token] = bot

			go func(bot *TelegramBot, token string) {
				if err := bot.RunBot(ctx); err != nil {
					log.Error().Err(err).Msg("Telegram bot exited with error")
				}
			}(bot, token)
		} else {
			// Update the app list on the existing bot without restarting
			existingBot.UpdateApps(appContexts)
		}
	}

	return nil
}

// Stop stops all running Telegram bots
func (t *Telegram) Stop() {
	t.botMu.Lock()
	defer t.botMu.Unlock()

	log.Info().Msg("stopping all Telegram bots")

	for _, bot := range t.bots {
		bot.Stop()
	}

	t.bots = make(map[string]*TelegramBot)
}
