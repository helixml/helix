package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/anthropic"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

func (s *HelixAPIServer) anthropicAPIProxyHandler(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "user is required", http.StatusUnauthorized)
		return
	}

	// Claude Code allows setting headers: https://docs.anthropic.com/en/docs/claude-code/settings
	provider := r.Header.Get("X-Provider")
	orgID := r.Header.Get("X-Org-ID")

	endpoint, err := s.getProviderEndpoint(r.Context(), user, orgID, provider)
	if err != nil {
		log.Error().Err(err).Msg("failed to get provider endpoint")

		http.Error(w, "Failed to get provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if endpoint.OwnerType == types.OwnerTypeOrg {
		orgID = endpoint.Owner
	} else {
		orgID = ""
	}

	bts, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	logger := log.With().
		Str("user_id", user.ID).
		Str("project_id", user.ProjectID).
		Str("spec_task_id", user.SpecTaskID).
		Str("organization_id", orgID).
		Logger()

	ctx := oai.SetContextValues(r.Context(), &oai.ContextValues{
		OwnerID:         user.ID,
		SessionID:       "n/a",
		InteractionID:   "n/a",
		OriginalRequest: bts,
		ProjectID:       user.ProjectID,
		SpecTaskID:      user.SpecTaskID,
	})

	// Restore the buffer
	r.Body = io.NopCloser(bytes.NewBuffer(bts))

	ctx = oai.SetContextOrganizationID(ctx, orgID)

	r = r.WithContext(ctx)

	hasEnoughBalance, err := s.Controller.HasEnoughBalance(ctx, getRequestUser(r), orgID, endpoint.BillingEnabled)
	if err != nil {
		logger.Error().Err(err).Msg("failed to check balance")
		http.Error(w, "Failed to check balance: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !hasEnoughBalance {
		logger.Error().Msg("insufficient balance")
		http.Error(w, "Insufficient balance", http.StatusPaymentRequired)
		return
	}

	r = anthropic.SetRequestProviderEndpoint(r, endpoint)

	logger.Info().Msg("has enough balance, proxying request")

	s.anthropicProxy.ServeHTTP(w, r)
}

func (s *HelixAPIServer) getProviderEndpoint(ctx context.Context, user *types.User, orgID, provider string) (*types.ProviderEndpoint, error) {
	// Default to "anthropic" provider if not specified
	// This allows Zed and other Anthropic SDK clients to work without setting X-Provider header
	if provider == "" {
		provider = "anthropic"
	}

	// Priority order for Anthropic credentials:
	// 1. User's own BYOK API key (ProviderEndpoint with provider=anthropic)
	// 2. User's Claude subscription OAuth token (OAuthConnection)
	// 3. Organization's provider endpoint
	// 4. Built-in system provider (from ANTHROPIC_API_KEY env var)

	// 1. Check for user's own Anthropic API key (BYOK)
	if provider == "anthropic" || provider == string(types.ProviderAnthropic) {
		userEndpoint, err := s.getUserAnthropicEndpoint(ctx, user.ID)
		if err == nil && userEndpoint != nil {
			log.Debug().
				Str("user_id", user.ID).
				Str("endpoint_id", userEndpoint.ID).
				Msg("Using user's Anthropic API key for proxy")
			return userEndpoint, nil
		}
	}

	if orgID != "" {
		_, err := s.authorizeOrgMember(ctx, user, orgID)
		if err != nil {
			return nil, fmt.Errorf("error authorizing org member: %w", err)
		}

		q := &store.GetProviderEndpointsQuery{
			Owner: orgID,
		}

		if strings.HasPrefix(provider, system.ProviderEndpointPrefix) {
			q.ID = provider
		} else {
			q.Name = provider
		}

		endpoint, err := s.Store.GetProviderEndpoint(ctx, q)
		if err == nil {
			return endpoint, nil
		}
		// Fall through to check built-in providers from config
	}

	// Try database first for non-org requests
	q := &store.GetProviderEndpointsQuery{
		Owner: orgID,
	}

	if strings.HasPrefix(provider, system.ProviderEndpointPrefix) {
		q.ID = provider
	} else {
		q.Name = provider
	}

	endpoint, err := s.Store.GetProviderEndpoint(ctx, q)
	if err == nil {
		return endpoint, nil
	}

	// Fall back to built-in Anthropic provider from environment variables
	// This allows ANTHROPIC_API_KEY to work without database configuration
	return s.getBuiltInProviderEndpoint(provider)
}

// getUserAnthropicEndpoint returns a ProviderEndpoint for the user's Anthropic credentials.
// It first checks for a user-owned ProviderEndpoint with an Anthropic API key,
// then falls back to checking for an OAuth connection to Anthropic.
func (s *HelixAPIServer) getUserAnthropicEndpoint(ctx context.Context, userID string) (*types.ProviderEndpoint, error) {
	// First, try to get API key from user's ProviderEndpoint
	endpoints, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
		Owner:    userID,
		Provider: types.ProviderAnthropic,
	})
	if err == nil {
		for _, ep := range endpoints {
			if ep.APIKey != "" {
				return ep, nil
			}
		}
	}

	// Fall back to OAuth connection
	providers, err := s.Store.ListOAuthProviders(ctx, &store.ListOAuthProvidersQuery{
		Enabled: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list OAuth providers: %w", err)
	}

	// Find Anthropic provider
	var anthropicProviderID string
	for _, p := range providers {
		if p.Type == types.OAuthProviderTypeAnthropic {
			anthropicProviderID = p.ID
			break
		}
	}

	if anthropicProviderID == "" {
		return nil, fmt.Errorf("no Anthropic OAuth provider configured")
	}

	// Get user's connection to Anthropic
	connection, err := s.Store.GetOAuthConnectionByUserAndProvider(ctx, userID, anthropicProviderID)
	if err != nil {
		return nil, fmt.Errorf("no Anthropic OAuth connection for user: %w", err)
	}

	// Check if token needs refreshing
	if !connection.ExpiresAt.IsZero() && connection.ExpiresAt.Before(time.Now()) {
		// Try to refresh the token
		if connection.RefreshToken != "" {
			clientID := s.Cfg.Providers.Anthropic.OAuthClientID
			clientSecret := s.Cfg.Providers.Anthropic.OAuthClientSecret

			tokenResp, refreshErr := RefreshAnthropicToken(connection.RefreshToken, clientID, clientSecret)
			if refreshErr != nil {
				log.Warn().Err(refreshErr).Str("user_id", userID).Msg("Failed to refresh Anthropic OAuth token")
				return nil, fmt.Errorf("Anthropic OAuth token expired and refresh failed: %w", refreshErr)
			}

			// Update connection with new tokens
			connection.AccessToken = tokenResp.AccessToken
			if tokenResp.RefreshToken != "" {
				connection.RefreshToken = tokenResp.RefreshToken
			}
			if tokenResp.ExpiresIn > 0 {
				connection.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
			}

			_, updateErr := s.Store.UpdateOAuthConnection(ctx, connection)
			if updateErr != nil {
				log.Warn().Err(updateErr).Str("user_id", userID).Msg("Failed to update refreshed Anthropic OAuth token")
			}
		} else {
			return nil, fmt.Errorf("Anthropic OAuth token expired and no refresh token available")
		}
	}

	// Return a synthetic ProviderEndpoint with the OAuth token as the API key
	return &types.ProviderEndpoint{
		ID:             "oauth-" + connection.ID,
		Name:           "Anthropic (Claude Subscription)",
		Description:    "User's Claude subscription via OAuth",
		BaseURL:        s.Cfg.Providers.Anthropic.BaseURL,
		APIKey:         connection.AccessToken, // OAuth token used as bearer token
		EndpointType:   types.ProviderEndpointTypeUser,
		Owner:          userID,
		OwnerType:      types.OwnerTypeUser,
		BillingEnabled: false, // User's own subscription, no Helix billing
	}, nil
}

// getBuiltInProviderEndpoint returns a ProviderEndpoint for the built-in Anthropic provider
// configured via environment variables (ANTHROPIC_API_KEY or ANTHROPIC_API_KEY_FILE).
// This allows the Anthropic proxy to work without requiring database configuration.
//
// To enable usage tracking/billing for this built-in provider, set PROVIDERS_BILLING_ENABLED=true.
// This is separate from STRIPE_BILLING_ENABLED which controls the platform-level Stripe integration.
func (s *HelixAPIServer) getBuiltInProviderEndpoint(provider string) (*types.ProviderEndpoint, error) {
	if provider != string(types.ProviderAnthropic) {
		return nil, fmt.Errorf("provider %q not found", provider)
	}

	// Get API key from env var or file
	apiKey := s.Cfg.Providers.Anthropic.APIKey
	if apiKey == "" && s.Cfg.Providers.Anthropic.APIKeyFromFile != "" {
		data, err := os.ReadFile(s.Cfg.Providers.Anthropic.APIKeyFromFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read ANTHROPIC_API_KEY_FILE: %w", err)
		}
		apiKey = strings.TrimSpace(string(data))
	}

	if apiKey == "" {
		return nil, fmt.Errorf("anthropic provider not configured: ANTHROPIC_API_KEY or ANTHROPIC_API_KEY_FILE not set")
	}

	return &types.ProviderEndpoint{
		ID:             string(types.ProviderAnthropic),
		Name:           string(types.ProviderAnthropic),
		Description:    "Built-in Anthropic provider",
		BaseURL:        s.Cfg.Providers.Anthropic.BaseURL,
		APIKey:         apiKey,
		EndpointType:   types.ProviderEndpointTypeGlobal,
		Owner:          string(types.OwnerTypeSystem),
		OwnerType:      types.OwnerTypeSystem,
		BillingEnabled: s.Cfg.Providers.BillingEnabled, // Controlled by PROVIDERS_BILLING_ENABLED env var
	}, nil
}
