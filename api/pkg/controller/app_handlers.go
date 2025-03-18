package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// getAppOAuthTokenEnv retrieves OAuth tokens for an app and returns them as environment variables
func (c *Controller) getAppOAuthTokenEnv(ctx context.Context, app *types.App, userID string) ([]string, error) {
	if c.Options.OAuthManager == nil {
		return nil, nil
	}

	var envVars []string

	// Check each assistant's tools for OAuth provider requirements
	for _, assistant := range app.Config.Helix.Assistants {
		for _, tool := range assistant.Tools {
			if tool.ToolType == types.ToolTypeAPI && tool.Config.API != nil && tool.Config.API.OAuthProvider != "" {
				providerType := tool.Config.API.OAuthProvider
				requiredScopes := tool.Config.API.OAuthScopes

				token, err := c.Options.OAuthManager.GetTokenForTool(ctx, userID, providerType, requiredScopes)
				if err != nil {
					// Check if it's a scope error
					var scopeErr *oauth.ScopeError
					if errors.As(err, &scopeErr) {
						log.Warn().
							Str("app_id", app.ID).
							Str("user_id", userID).
							Str("provider", string(providerType)).
							Strs("missing_scopes", scopeErr.Missing).
							Msg("Missing required OAuth scopes for tool")

						// Include this error in logs but continue with other providers
						continue
					}

					// For other errors, log and continue
					log.Warn().
						Err(err).
						Str("app_id", app.ID).
						Str("user_id", userID).
						Str("provider", string(providerType)).
						Msg("Failed to get OAuth token for tool")

					continue
				}

				// Add the token as an environment variable
				envVar := fmt.Sprintf("OAUTH_TOKEN_%s=%s", strings.ToUpper(string(providerType)), token)
				envVars = append(envVars, envVar)
			}
		}
	}

	return envVars, nil
}
