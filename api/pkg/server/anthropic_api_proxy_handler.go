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

	ctx := oai.SetContextValues(r.Context(), &oai.ContextValues{
		OwnerID:         user.ID,
		SessionID:       "n/a",
		InteractionID:   "n/a",
		OriginalRequest: bts,
	})

	// Restore the buffer
	r.Body = io.NopCloser(bytes.NewBuffer(bts))

	ctx = oai.SetContextOrganizationID(ctx, orgID)

	r = r.WithContext(ctx)

	hasEnoughBalance, err := s.Controller.HasEnoughBalance(ctx, getRequestUser(r), orgID, endpoint.BillingEnabled)
	if err != nil {
		log.Error().Err(err).Msg("failed to check balance")
		http.Error(w, "Failed to check balance: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !hasEnoughBalance {
		http.Error(w, "Insufficient balance", http.StatusPaymentRequired)
		return
	}

	r = anthropic.SetRequestProviderEndpoint(r, endpoint)

	s.anthropicProxy.ServeHTTP(w, r)
}

func (s *HelixAPIServer) getProviderEndpoint(ctx context.Context, user *types.User, orgID, provider string) (*types.ProviderEndpoint, error) {
	// Default to "anthropic" provider if not specified
	// This allows Zed and other Anthropic SDK clients to work without setting X-Provider header
	if provider == "" {
		provider = "anthropic"
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
