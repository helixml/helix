package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/helixml/helix/api/pkg/anthropic"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	anthropic_sdk "github.com/anthropics/anthropic-sdk-go" // imported as anthropic
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

		http.Error(w, "Failed to get provider endpoint: "+err.Error()+
			". Note: /v1/messages only supports Anthropic-compatible providers. "+
			"For OpenAI-compatible providers (Ollama, OpenRouter, etc.), use /v1/chat/completions instead.",
			http.StatusBadRequest)
		return
	}

	// Guard: reject non-Anthropic providers early with a clear error.
	// The /v1/messages endpoint uses Anthropic API format which is incompatible
	// with OpenAI-compatible providers (Ollama, OpenRouter, etc.).
	// Agent tasks with non-Anthropic providers should route through /v1/chat/completions
	// via the Zed config (see mapHelixToZedProvider in zed_config.go).
	if !isAnthropicCompatible(endpoint) {
		log.Warn().
			Str("provider", endpoint.Name).
			Str("base_url", endpoint.BaseURL).
			Str("user_id", user.ID).
			Msg("non-Anthropic provider hit /v1/messages — rejecting")

		http.Error(w,
			fmt.Sprintf("Provider %q is not Anthropic-compatible. "+
				"The /v1/messages endpoint only supports Anthropic API format. "+
				"Use /v1/chat/completions for OpenAI-compatible providers (Ollama, OpenRouter, etc.).",
				endpoint.Name),
			http.StatusBadRequest)
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

	// Validate the request
	if s.Cfg.Providers.BillingEnabled {
		modelName, ok := parseAnthropicRequestModel(bts)
		if !ok {
			return
		}

		_, err = s.modelInfoProvider.GetModelInfo(r.Context(), &model.ModelInfoRequest{
			BaseURL:  endpoint.BaseURL,
			Provider: endpoint.Name,
			Model:    modelName,
		})
		if err != nil {
			log.Error().Err(err).
				Str("model", modelName).
				Str("provider", endpoint.Name).
				Str("base_url", endpoint.BaseURL).
				Msg("failed to get model info for billing")
			http.Error(w, fmt.Sprintf("Could not find model information for model '%s', error: %s", modelName, err.Error()), http.StatusPreconditionFailed)
			return
		}
		// OK
	}

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

// isAnthropicCompatible checks whether a provider endpoint speaks the Anthropic
// /v1/messages API format. Currently only the built-in "anthropic" provider and
// endpoints whose base URL points to Anthropic's API (or a Vertex AI proxy) qualify.
func isAnthropicCompatible(ep *types.ProviderEndpoint) bool {
	if ep.Name == string(types.ProviderAnthropic) {
		return true
	}
	// Vertex AI endpoints proxy Anthropic format
	if ep.VertexProjectID != "" {
		return true
	}
	// Check base URL for known Anthropic-compatible hosts
	baseURL := strings.ToLower(ep.BaseURL)
	return strings.Contains(baseURL, "anthropic.com") ||
		strings.Contains(baseURL, "anthropic.googleapis.com")
}

func parseAnthropicRequestModel(body []byte) (string, bool) {
	if len(body) == 0 {
		return "", false
	}

	var req anthropic_sdk.Message
	if err := json.Unmarshal(body, &req); err != nil {
		return "", false
	}

	if req.Model == "" {
		return "", false
	}

	return string(req.Model), true
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
	return s.getBuiltInProviderEndpoint(ctx, provider)
}

// getBuiltInProviderEndpoint returns a ProviderEndpoint for built-in (env-baked)
// providers.
//
// Anthropic gets the rich path because /v1/messages actually proxies through here:
// when ANTHROPIC_VERTEX_PROJECT_ID is set, Vertex AI wins unconditionally and
// ANTHROPIC_API_KEY is only used for model listing; otherwise ANTHROPIC_API_KEY
// (or ANTHROPIC_API_KEY_FILE) is required. Set PROVIDERS_BILLING_ENABLED=true to
// track usage on this built-in (separate from STRIPE_BILLING_ENABLED).
//
// Other env-baked globals (openai, togetherai, helix, ...) are returned as
// Name-only synthetics from MultiClientManager.globalClients. We don't need
// their full BaseURL/APIKey here because /v1/messages will reject them via
// isAnthropicCompatible with the actionable "use /v1/chat/completions" message
// — and the synthetic carries enough (Name, EndpointType=Global) to drive that
// branch instead of the cryptic "provider %q not found" error.
func (s *HelixAPIServer) getBuiltInProviderEndpoint(ctx context.Context, provider string) (*types.ProviderEndpoint, error) {
	if provider != string(types.ProviderAnthropic) {
		// Resolve env-baked globals (openai etc.) from the provider manager so
		// callers get a real endpoint they can interrogate, not a "not found".
		if s.providerManager != nil {
			globals, err := s.providerManager.ListProviderEndpoints(ctx, "")
			if err == nil {
				for _, ep := range globals {
					if strings.EqualFold(ep.Name, provider) {
						return ep, nil
					}
				}
			}
		}
		return nil, fmt.Errorf("provider %q not found", provider)
	}

	// Vertex AI mode: Vertex wins unconditionally when configured
	if s.Cfg.Providers.Anthropic.VertexProjectID != "" {
		// API key is optional in Vertex mode — only used for model listing
		apiKey := s.Cfg.Providers.Anthropic.APIKey
		if apiKey == "" && s.Cfg.Providers.Anthropic.APIKeyFromFile != "" {
			data, err := os.ReadFile(s.Cfg.Providers.Anthropic.APIKeyFromFile)
			if err == nil {
				apiKey = strings.TrimSpace(string(data))
			}
		}

		return &types.ProviderEndpoint{
			ID:                    string(types.ProviderAnthropic),
			Name:                  string(types.ProviderAnthropic),
			Description:           "Built-in Anthropic provider (via Google Vertex AI)",
			BaseURL:               s.Cfg.Providers.Anthropic.BaseURL,
			APIKey:                apiKey,
			EndpointType:          types.ProviderEndpointTypeGlobal,
			Owner:                 string(types.OwnerTypeSystem),
			OwnerType:             types.OwnerTypeSystem,
			BillingEnabled:        s.Cfg.Providers.BillingEnabled,
			VertexProjectID:       s.Cfg.Providers.Anthropic.VertexProjectID,
			VertexRegion:          s.Cfg.Providers.Anthropic.VertexRegion,
			VertexCredentialsJSON: s.Cfg.Providers.Anthropic.VertexCredentialsJSON,
			VertexCredentialsFile: s.Cfg.Providers.Anthropic.VertexCredentialsFile,
		}, nil
	}

	// Direct Anthropic API mode: API key is required
	apiKey := s.Cfg.Providers.Anthropic.APIKey
	if apiKey == "" && s.Cfg.Providers.Anthropic.APIKeyFromFile != "" {
		data, err := os.ReadFile(s.Cfg.Providers.Anthropic.APIKeyFromFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read ANTHROPIC_API_KEY_FILE: %w", err)
		}
		apiKey = strings.TrimSpace(string(data))
	}

	if apiKey == "" {
		return nil, fmt.Errorf("anthropic provider not configured: set ANTHROPIC_VERTEX_PROJECT_ID for Vertex AI, or ANTHROPIC_API_KEY / ANTHROPIC_API_KEY_FILE for direct API access")
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
		BillingEnabled: s.Cfg.Providers.BillingEnabled,
	}, nil
}
