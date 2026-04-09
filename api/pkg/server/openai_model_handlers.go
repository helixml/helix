package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/anthropic"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listModels godoc
// @Summary List models
// @Description List models from a specific provider, or aggregate from all providers if none specified. If the request includes an anthropic-version header, proxies to the upstream Anthropic provider.
// @Tags    models
// @Success 200 {array} types.OpenAIModelsList
// @Param provider query string false "Provider"
// @Router /v1/models [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listModels(rw http.ResponseWriter, r *http.Request) {
	// Anthropic clients identify themselves with this header — proxy straight through
	if r.Header.Get("anthropic-version") != "" {
		apiServer.listModelsAnthropic(rw, r)
		return
	}

	// If a specific provider is requested, return just that provider's models (original behavior)
	if provider := r.URL.Query().Get("provider"); provider != "" {
		apiServer.listModelsForProvider(rw, r, provider)
		return
	}

	// No provider specified — aggregate models from all configured providers
	var allModels []types.OpenAIModel

	// Global providers from env vars (openai, togetherai, anthropic, helix, vllm)
	globalProviders, err := apiServer.providerManager.ListProviders(r.Context(), "")
	if err != nil {
		log.Warn().Err(err).Msg("failed to list global providers for model aggregation")
	} else {
		for _, provider := range globalProviders {
			// Anthropic models are served via the anthropic-version header path
			if provider == types.ProviderAnthropic {
				continue
			}

			cacheKey := fmt.Sprintf("%s:%s", provider, types.OwnerTypeSystem)
			models := apiServer.getCachedModels(cacheKey)
			if models == nil {
				continue
			}

			// Prefix models with provider name so the caller knows where to route them,
			// unless they already have a prefix (e.g. from a downstream Helix that already prefixed)
			if provider != types.ProviderHelix {
				models = prefixModels(models, string(provider))
			}
			allModels = append(allModels, models...)
		}
	}

	// Database-stored provider endpoints (user-created and admin global endpoints)
	dbProviders, err := apiServer.Store.ListProviderEndpoints(r.Context(), &store.ListProviderEndpointsQuery{
		Owner:      string(types.OwnerTypeSystem),
		WithGlobal: true,
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to list database providers for model aggregation")
	} else {
		for _, provider := range dbProviders {
			if provider.Name == string(types.ProviderAnthropic) || provider.Name == string(types.ProviderHelix) {
				continue
			}
			cacheKey := fmt.Sprintf("%s:%s", provider.Name, provider.Owner)
			models := apiServer.getCachedModels(cacheKey)
			if models == nil {
				continue
			}
			allModels = append(allModels, prefixModels(models, provider.Name)...)
		}
	}

	writeResponse(rw, types.OpenAIModelsList{Models: allModels}, http.StatusOK)
}

// listModelsForProvider returns models from a single provider.
func (apiServer *HelixAPIServer) listModelsForProvider(rw http.ResponseWriter, r *http.Request, provider string) {
	user := getRequestUser(r)
	req := &manager.GetClientRequest{Provider: provider}
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

	writeResponse(rw, types.OpenAIModelsList{Models: models}, http.StatusOK)
}

// listModelsAnthropic proxies the request to the upstream Anthropic provider.
func (apiServer *HelixAPIServer) listModelsAnthropic(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "user is required", http.StatusUnauthorized)
		return
	}

	endpoint, err := apiServer.getBuiltInProviderEndpoint(string(types.ProviderAnthropic))
	if err != nil {
		log.Err(err).Msg("failed to get Anthropic provider endpoint")
		http.Error(rw, "Anthropic provider not configured: "+err.Error(), http.StatusInternalServerError)
		return
	}

	r = anthropic.SetRequestProviderEndpoint(r, endpoint)
	apiServer.anthropicProxy.ServeHTTP(rw, r)
}

// getCachedModels returns cached models for a provider, or nil if not cached.
func (apiServer *HelixAPIServer) getCachedModels(cacheKey string) []types.OpenAIModel {
	cached, found := apiServer.cache.Get(cacheKey)
	if !found {
		return nil
	}
	var models []types.OpenAIModel
	if err := json.Unmarshal([]byte(cached), &models); err != nil {
		return nil
	}
	return models
}

// prefixModels prepends providerName/ to model IDs that don't already contain a slash.
func prefixModels(models []types.OpenAIModel, providerName string) []types.OpenAIModel {
	out := make([]types.OpenAIModel, len(models))
	for i, m := range models {
		out[i] = m
		if !strings.Contains(m.ID, "/") {
			out[i].ID = providerName + "/" + m.ID
		}
		if out[i].OwnedBy == "" {
			out[i].OwnedBy = providerName
		}
	}
	return out
}
