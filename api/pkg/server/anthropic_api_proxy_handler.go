package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

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

	endpoint, err := s.getProviderEndpoint(r.Context(), user)
	if err != nil {
		log.Error().Err(err).Msg("failed to get provider endpoint")

		http.Error(w, "Failed to get provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
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
		Str("organization_id", user.OrganizationID).
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

	ctx = oai.SetContextOrganizationID(ctx, user.OrganizationID)

	r = r.WithContext(ctx)

	hasEnoughBalance, err := s.Controller.HasEnoughBalance(ctx, getRequestUser(r), user.OrganizationID, endpoint.BillingEnabled)
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

// anthropicTokenCountHandler proxies the token counting request to Anthropic.
// Token counting is free according to Anthropic, so we don't check balance.
// See: https://docs.anthropic.com/en/api/messages-count-tokens
// func (s *HelixAPIServer) anthropicTokenCountHandler(w http.ResponseWriter, r *http.Request) {
// 	log.Info().Msg("🔢 [TOKEN_COUNT] Received token counting request")

// 	user := getRequestUser(r)
// 	if user == nil {
// 		log.Warn().Msg("🔢 [TOKEN_COUNT] No user in request, returning 401")
// 		http.Error(w, "user is required", http.StatusUnauthorized)
// 		return
// 	}

// 	provider := r.Header.Get("X-Provider")
// 	orgID := r.Header.Get("X-Org-ID")

// 	log.Info().
// 		Str("user_id", user.ID).
// 		Str("provider", provider).
// 		Str("org_id", orgID).
// 		Msg("🔢 [TOKEN_COUNT] Processing request")

// 	endpoint, err := s.getProviderEndpoint(r.Context(), user, orgID, provider)
// 	if err != nil {
// 		log.Error().Err(err).Msg("🔢 [TOKEN_COUNT] Failed to get provider endpoint")
// 		http.Error(w, "Failed to get provider endpoint: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	// Set the endpoint in request context for the proxy director
// 	r = anthropic.SetRequestProviderEndpoint(r, endpoint)

// 	log.Info().
// 		Str("user_id", user.ID).
// 		Str("provider", endpoint.Name).
// 		Str("base_url", endpoint.BaseURL).
// 		Msg("🔢 [TOKEN_COUNT] Proxying to Anthropic")

// 	s.anthropicProxy.ServeHTTP(w, r)

// 	log.Info().
// 		Str("user_id", user.ID).
// 		Msg("🔢 [TOKEN_COUNT] Request completed")
// }

func (s *HelixAPIServer) getProviderEndpoint(ctx context.Context, user *types.User) (*types.ProviderEndpoint, error) {
	// Default to "anthropic" provider if not specified
	// This allows Zed and other Anthropic SDK clients to work without setting X-Provider header

	provider := "anthropic"

	// Helix agent
	if user.ProjectID != "" {
		project, err := s.Store.GetProject(ctx, user.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("error getting project: %w", err)
		}

		projectApp, err := s.Store.GetApp(ctx, project.DefaultHelixAppID)
		if err != nil {
			return nil, fmt.Errorf("error getting project app: %w", err)
		}

		if len(projectApp.Config.Helix.Assistants) == 0 {
			return nil, fmt.Errorf("no assistants found for project app")
		}

		provider = projectApp.Config.Helix.Assistants[0].Provider

		if provider == "" {
			return nil, fmt.Errorf("no provider found for project app")
		}
	}

	if user.OrganizationID != "" {
		_, err := s.authorizeOrgMember(ctx, user, user.OrganizationID)
		if err != nil {
			return nil, fmt.Errorf("error authorizing org member: %w", err)
		}

		q := &store.GetProviderEndpointsQuery{
			Owner: user.OrganizationID,
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
		Owner: user.OrganizationID,
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
