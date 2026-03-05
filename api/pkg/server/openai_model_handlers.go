package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/anthropic"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// hasConnectedRunners checks if at least one runner is connected to the scheduler.
// This is used to determine whether to include internal Helix models in the model list.
func (apiServer *HelixAPIServer) hasConnectedRunners() bool {
	if apiServer.scheduler == nil {
		return false
	}
	runnerStatuses, err := apiServer.scheduler.RunnerStatus()
	if err != nil {
		log.Debug().Err(err).Msg("failed to get runner status")
		return false
	}
	return len(runnerStatuses) > 0
}

// listModels godoc
// @Summary List models
// @Description List models. Supports dual-mode: returns Anthropic format if anthropic-version header is present, otherwise returns OpenAI format with aggregated provider models.
// @Tags    models
// @Success 200 {array} types.OpenAIModelsList
// @Param provider query string false "Provider (for OpenAI format only)"
// @Router /v1/models [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listModels(rw http.ResponseWriter, r *http.Request) {
	// Dual-mode detection: if anthropic-version header is present, return Anthropic format
	if r.Header.Get("anthropic-version") != "" {
		apiServer.listModelsAnthropic(rw, r)
		return
	}

	// OpenAI format - aggregate models from all providers
	apiServer.listModelsOpenAI(rw, r)
}

// listModelsAnthropic forwards the /v1/models request to the upstream Anthropic provider.
// This is used when an Anthropic client (like the inner Helix's Anthropic provider)
// queries /v1/models with the anthropic-version header.
// The request is proxied to the actual Anthropic API (or compatible endpoint like Vertex AI).
func (apiServer *HelixAPIServer) listModelsAnthropic(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "user is required", http.StatusUnauthorized)
		return
	}

	// Get the Anthropic provider endpoint
	endpoint, err := apiServer.getBuiltInProviderEndpoint(string(types.ProviderAnthropic))
	if err != nil {
		log.Err(err).Msg("failed to get Anthropic provider endpoint")
		http.Error(rw, "Anthropic provider not configured: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Set the endpoint in request context for the proxy director
	r = anthropic.SetRequestProviderEndpoint(r, endpoint)

	log.Debug().
		Str("user_id", user.ID).
		Str("base_url", endpoint.BaseURL).
		Msg("proxying /v1/models request to Anthropic provider")

	// Forward the request to the Anthropic proxy
	apiServer.anthropicProxy.ServeHTTP(rw, r)
}

// listModelsOpenAI returns models in OpenAI API format.
// If a specific provider is requested via query param, returns only that provider's models.
// Otherwise, aggregates models from all configured providers with appropriate prefixes.
func (apiServer *HelixAPIServer) listModelsOpenAI(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provider := r.URL.Query().Get("provider")

	// If a specific provider is requested, use the original single-provider behavior
	if provider != "" {
		apiServer.listModelsFromProvider(rw, r, provider)
		return
	}

	// Aggregate models from all providers
	aggregatedModels, err := apiServer.aggregateAllProviderModels(ctx)
	if err != nil {
		log.Err(err).Msg("error aggregating models from providers")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := types.OpenAIModelsList{
		Models: aggregatedModels,
	}

	writeResponse(rw, response, http.StatusOK)
}

// listModelsFromProvider returns models from a specific provider (original single-provider behavior)
func (apiServer *HelixAPIServer) listModelsFromProvider(rw http.ResponseWriter, r *http.Request, provider string) {
	user := getRequestUser(r)

	req := &manager.GetClientRequest{
		Provider: provider,
	}
	if user != nil {
		req.Owner = user.ID
	}

	client, err := apiServer.providerManager.GetClient(r.Context(), req)
	if err != nil {
		log.Err(err).Msg("error getting client")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	models, err := client.ListModels(r.Context())
	if err != nil {
		log.Err(err).Msg("error listing models")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := types.OpenAIModelsList{
		Models: models,
	}

	writeResponse(rw, response, http.StatusOK)
}

// aggregateAllProviderModels collects models from all configured providers.
// - Internal Helix models: returned unprefixed (e.g., "Qwen/Qwen2.5-VL-3B-Instruct")
// - OpenAI models: prefixed with "openai/" (e.g., "openai/gpt-4o")
// - TogetherAI models: prefixed with "togetherai/" (e.g., "togetherai/meta-llama/...")
// - Nebius models: prefixed with "nebius/" (e.g., "nebius/llama-3.3-70b")
// - Anthropic models: NOT included here (served via Anthropic-format endpoint)
// - VLLM models: prefixed with "vllm/" if configured
func (apiServer *HelixAPIServer) aggregateAllProviderModels(ctx context.Context) ([]types.OpenAIModel, error) {
	var allModels []types.OpenAIModel

	// 1. Get internal Helix models (unprefixed) - only if runners are connected
	if apiServer.hasConnectedRunners() {
		helixModels, err := apiServer.getHelixInternalModels(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("failed to get helix internal models")
			// Continue - don't fail if helix models aren't available
		} else {
			allModels = append(allModels, helixModels...)
		}
	} else {
		log.Debug().Msg("no runners connected, skipping internal Helix models")
	}

	// 2. Get models from global providers (from env vars)
	globalProviders, err := apiServer.providerManager.ListProviders(ctx, "")
	if err != nil {
		log.Warn().Err(err).Msg("failed to list global providers")
	} else {
		for _, provider := range globalProviders {
			// Skip helix provider - already handled above
			if provider == types.ProviderHelix {
				continue
			}

			// Skip Anthropic - those are served via the Anthropic-format endpoint
			if provider == types.ProviderAnthropic {
				continue
			}

			models, err := apiServer.getProviderModelsWithPrefix(ctx, string(provider), string(types.OwnerTypeSystem))
			if err != nil {
				log.Debug().
					Err(err).
					Str("provider", string(provider)).
					Msg("failed to get models from global provider (provider may be unavailable)")
				continue
			}
			allModels = append(allModels, models...)
		}
	}

	// 3. Get models from database-stored providers (user and global from DB)
	dbProviders, err := apiServer.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
		Owner:      string(types.OwnerTypeSystem),
		WithGlobal: true,
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to list database providers")
	} else {
		for _, provider := range dbProviders {
			// Skip Anthropic providers - served via Anthropic-format endpoint
			if provider.Name == string(types.ProviderAnthropic) {
				continue
			}

			// Skip helix provider
			if provider.Name == string(types.ProviderHelix) {
				continue
			}

			models, err := apiServer.getProviderModelsWithPrefix(ctx, provider.Name, provider.Owner)
			if err != nil {
				log.Debug().
					Err(err).
					Str("provider", provider.Name).
					Str("owner", provider.Owner).
					Msg("failed to get models from database provider")
				continue
			}
			allModels = append(allModels, models...)
		}
	}

	return allModels, nil
}

// getHelixInternalModels returns models from the internal Helix scheduler (unprefixed)
func (apiServer *HelixAPIServer) getHelixInternalModels(ctx context.Context) ([]types.OpenAIModel, error) {
	client, err := apiServer.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: string(types.ProviderHelix),
		Owner:    string(types.OwnerTypeSystem),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get helix client: %w", err)
	}

	models, err := client.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list helix models: %w", err)
	}

	// Helix internal models are returned unprefixed
	return models, nil
}

// getProviderModelsWithPrefix returns models from a provider with the provider name as prefix
func (apiServer *HelixAPIServer) getProviderModelsWithPrefix(ctx context.Context, providerName, owner string) ([]types.OpenAIModel, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s:%s", providerName, owner)
	if cached, found := apiServer.cache.Get(cacheKey); found {
		var models []types.OpenAIModel
		if err := json.Unmarshal([]byte(cached), &models); err == nil {
			// Add prefix to cached models
			return apiServer.prefixModels(models, providerName), nil
		}
	}

	// Fetch from provider
	client, err := apiServer.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: providerName,
		Owner:    owner,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get client for %s: %w", providerName, err)
	}

	models, err := client.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list models from %s: %w", providerName, err)
	}

	// Add prefix to models
	return apiServer.prefixModels(models, providerName), nil
}

// prefixModels adds the provider name as a prefix to each model ID
func (apiServer *HelixAPIServer) prefixModels(models []types.OpenAIModel, providerName string) []types.OpenAIModel {
	prefixed := make([]types.OpenAIModel, len(models))
	for i, m := range models {
		prefixed[i] = m
		prefixed[i].ID = providerName + "/" + m.ID
		// Update OwnedBy to reflect the provider
		if prefixed[i].OwnedBy == "" {
			prefixed[i].OwnedBy = providerName
		}
	}
	return prefixed
}
