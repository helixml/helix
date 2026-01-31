package tools

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/rs/zerolog/log"
)

// Add OAuth Manager to ChainStrategy
type ChainStrategyOptions struct {
	OAuthManager *oauth.Manager
	SessionStore store.Store
	AppStore     store.Store
}

// SetOAuthManager sets the OAuth manager for the chain strategy
func (c *ChainStrategy) SetOAuthManager(manager *oauth.Manager) {
	c.oauthManager = manager
}

// SetStores sets the stores needed for session and app lookups
func (c *ChainStrategy) SetStores(sessionStore store.Store, appStore store.Store) {
	c.sessionStore = sessionStore
	c.appStore = appStore
}

// InitChainStrategyOAuth initializes the OAuth manager and stores in the NewChainStrategy function
func InitChainStrategyOAuth(strategy Planner, oauthManager *oauth.Manager, sessionStore store.Store, appStore store.Store) {
	if chainStrategy, ok := strategy.(*ChainStrategy); ok && chainStrategy != nil {
		chainStrategy.SetOAuthManager(oauthManager)
		chainStrategy.SetStores(sessionStore, appStore)

		if oauthManager != nil {
			log.Info().Msg("OAuth manager initialized for ChainStrategy")
		}

		if sessionStore != nil && appStore != nil {
			log.Info().Msg("Session and app stores initialized for ChainStrategy")
		}
	}
}

// getUserIDFromSessionID gets the user ID from the session ID by looking up the session and app
func (c *ChainStrategy) getUserIDFromSessionID(ctx context.Context, sessionID string) (string, error) {
	// If we don't have the stores, we can't get the user ID
	if c.sessionStore == nil || c.appStore == nil {
		return "", fmt.Errorf("session store or app store not available")
	}

	// Get the session
	session, err := c.sessionStore.GetSession(ctx, sessionID)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to get session for OAuth lookup")
		return "", err
	}

	// If the session has no parent app, we can't get the user ID
	if session.ParentApp == "" {
		return "", fmt.Errorf("session has no parent app")
	}

	// Get the app
	app, err := c.appStore.GetApp(ctx, session.ParentApp)
	if err != nil {
		log.Warn().Err(err).Str("app_id", session.ParentApp).Msg("Failed to get app for OAuth lookup")
		return "", err
	}

	// Return the app owner as the user ID
	return app.Owner, nil
}

