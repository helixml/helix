package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listProviders godoc
// @Summary List currently configured providers
// @Description List currently configured providers
// @Tags    providers

// @Success 200 {array} types.Provider
// @Router /api/v1/providers [get]
// @Security BearerAuth
func (s *HelixAPIServer) listProviders(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)

	providers, err := s.providerManager.ListProviders(r.Context(), user.ID)
	if err != nil {
		log.Err(err).Msg("error listing providers")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(providers)
	if err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
}

var blankAPIKey = "********"

// listProviderEndpoints godoc
// @Summary List currently configured provider endpoints
// @Description List currently configured providers
// @Tags    providers

// @Success 200 {array} types.ProviderEndpoint
// @Param with_models query bool false "Include models"
// @Router /api/v1/provider-endpoints [get]
// @Security BearerAuth
func (s *HelixAPIServer) listProviderEndpoints(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	includeModels := r.URL.Query().Get("with_models") == "true"

	user := getRequestUser(r)

	var (
		providerEndpoints []*types.ProviderEndpoint
		err               error
	)

	if user != nil {
		providerEndpoints, err = s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
			Owner:      user.ID,
			OwnerType:  user.Type,
			WithGlobal: true,
		})
		if err != nil {
			log.Err(err).Msg("error listing provider endpoints")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		for idx := range providerEndpoints {
			if providerEndpoints[idx].APIKey != "" {
				providerEndpoints[idx].APIKey = blankAPIKey
			}
		}

		// Sort endpoints by name before adding global ones
		sort.Slice(providerEndpoints, func(i, j int) bool {
			return providerEndpoints[i].Name < providerEndpoints[j].Name
		})
	}

	// Get global ones from the provider manager
	globalProviderEndpoints, err := s.providerManager.ListProviders(ctx, "")
	if err != nil {
		log.Err(err).Msg("error listing providers")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for _, provider := range globalProviderEndpoints {
		var baseURL string
		switch provider {
		case types.ProviderOpenAI:
			baseURL = s.Cfg.Providers.OpenAI.BaseURL
		case types.ProviderTogetherAI:
			baseURL = s.Cfg.Providers.TogetherAI.BaseURL
		case types.ProviderVLLM:
			baseURL = s.Cfg.Providers.VLLM.BaseURL
		case types.ProviderHelix:
			baseURL = "internal"
		}

		providerEndpoints = append(providerEndpoints, &types.ProviderEndpoint{
			ID:           "-",
			Name:         string(provider),
			Description:  "",
			BaseURL:      baseURL,
			EndpointType: types.ProviderEndpointTypeGlobal,
			Owner:        string(types.OwnerTypeSystem),
			APIKey:       "",
		})
	}

	// Set default
	for idx := range providerEndpoints {
		if providerEndpoints[idx].Name == s.Cfg.Inference.Provider {
			providerEndpoints[idx].Default = true
		}
	}

	// Re-sort the endpoints with default first, then by name
	sort.Slice(providerEndpoints, func(i, j int) bool {
		// If i is default and j is not, i comes first
		if providerEndpoints[i].Default && !providerEndpoints[j].Default {
			return true
		}
		// If j is default and i is not, j comes first
		if providerEndpoints[j].Default && !providerEndpoints[i].Default {
			return false
		}
		// If both are default or both are not default, sort by name
		return providerEndpoints[i].Name < providerEndpoints[j].Name
	})

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Load models if required
	if includeModels {
		for idx := range providerEndpoints {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				models, err := s.getProviderModels(ctx, providerEndpoints[idx])
				if err != nil {
					log.Err(err).
						Str("provider", providerEndpoints[idx].Name).
						Str("endpoint_id", providerEndpoints[idx].ID).
						Str("owner", providerEndpoints[idx].Owner).
						Msg("error listing models")
					mu.Lock()
					providerEndpoints[idx].Status = types.ProviderEndpointStatusError
					providerEndpoints[idx].Error = err.Error()
					mu.Unlock()
					return
				}

				mu.Lock()
				providerEndpoints[idx].Status = types.ProviderEndpointStatusOK
				providerEndpoints[idx].AvailableModels = models
				mu.Unlock()
			}(idx)
		}
	}

	wg.Wait()

	writeResponse(rw, providerEndpoints, http.StatusOK)
}

func (s *HelixAPIServer) getProviderModels(ctx context.Context, providerEndpoint *types.ProviderEndpoint) ([]types.OpenAIModel, error) {
	// Check for cached models
	cacheKey := fmt.Sprintf("%s:%s", providerEndpoint.Name, providerEndpoint.Owner)
	if cached, found := s.cache.Get(cacheKey); found {
		var models []types.OpenAIModel
		err := json.Unmarshal([]byte(cached), &models)
		if err != nil {
			return nil, err
		}
		return models, nil
	}

	provider, err := s.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: providerEndpoint.Name,
		Owner:    providerEndpoint.Owner,
	})
	if err != nil {
		log.Err(err).
			Str("provider", providerEndpoint.Name).
			Str("owner", providerEndpoint.Owner).
			Msg("error getting provider")
		return nil, err
	}

	// Models should respond in 5 seconds or less, otherwise we'll kill the request
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	models, err := provider.ListModels(ctx)
	if err != nil {
		log.Err(err).
			Str("provider", providerEndpoint.Name).
			Str("owner", providerEndpoint.Owner).
			Msg("error listing models")
		return nil, err
	}

	// Cache the models
	modelsJSON, err := json.Marshal(models)
	if err != nil {
		return nil, err
	}
	s.cache.SetWithTTL(cacheKey, string(modelsJSON), 1, s.Cfg.WebServer.ModelsCacheTTL)

	return models, nil
}

// createProviderEndpoint godoc
// @Summary Create a new provider endpoint
// @Description Create a new provider endpoint
// @Tags    providers

// @Success 200 {object} types.ProviderEndpoint
// @Router /api/v1/provider-endpoints [post]
// @Security BearerAuth
func (s *HelixAPIServer) createProviderEndpoint(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)

	isAdmin := s.isAdmin(r)

	if !isAdmin && !s.Cfg.Providers.EnableCustomUserProviders {
		http.Error(rw, "Custom user providers are not enabled", http.StatusForbidden)
		return
	}

	var endpoint types.ProviderEndpoint
	if err := json.NewDecoder(r.Body).Decode(&endpoint); err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Check for duplicate names
	existingProviders, err := s.providerManager.ListProviders(r.Context(), user.ID)
	if err != nil {
		log.Err(err).Msg("error listing providers")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for _, provider := range existingProviders {
		if string(provider) == endpoint.Name {
			http.Error(rw, fmt.Sprintf("Provider with name '%s' already exists", endpoint.Name), http.StatusBadRequest)
			return
		}
	}

	// Set owner information
	endpoint.Owner = user.ID
	endpoint.OwnerType = user.Type

	// Default to user endpoint type if not specified
	if endpoint.EndpointType == "" {
		endpoint.EndpointType = types.ProviderEndpointTypeUser
	}

	// Only admins can add global endpoints
	if endpoint.EndpointType == types.ProviderEndpointTypeGlobal && !isAdmin {
		http.Error(rw, "Only admins can add global endpoints", http.StatusForbidden)
		return
	}

	// Only admins can add endpoints with API key path auth
	if endpoint.APIKeyFromFile != "" && !isAdmin {
		http.Error(rw, "Only admins can add endpoints with API key path auth", http.StatusForbidden)
		return
	}

	createdEndpoint, err := s.Store.CreateProviderEndpoint(ctx, &endpoint)
	if err != nil {
		log.Err(err).Msg("error creating provider endpoint")
		http.Error(rw, "Error creating provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Mask API key in response
	createdEndpoint.APIKey = "*****"

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(createdEndpoint); err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// updateProviderEndpoint godoc
// @Summary Update a provider endpoint
// @Description Update a provider endpoint. Global endpoints can only be updated by admins.
// @Tags    providers

// @Success 200 {object} types.UpdateProviderEndpoint
// @Router /api/v1/provider-endpoints/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateProviderEndpoint(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	endpointID := mux.Vars(r)["id"]

	if !user.Admin && !s.Cfg.Providers.EnableCustomUserProviders {
		http.Error(rw, "Custom user providers are not enabled", http.StatusForbidden)
		return
	}

	// Get existing endpoint
	existingEndpoint, err := s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{ID: endpointID})
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Provider endpoint not found", http.StatusNotFound)
			return
		}
		log.Err(err).Msg("error getting provider endpoint")
		http.Error(rw, "Error getting provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check ownership - only allow updates to owned endpoints or if user is admin
	if existingEndpoint.Owner != user.ID && !user.Admin {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var updatedEndpoint types.UpdateProviderEndpoint
	if err := json.NewDecoder(r.Body).Decode(&updatedEndpoint); err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// For global endpoints, only allow updates by admins
	if updatedEndpoint.EndpointType == types.ProviderEndpointTypeGlobal && !user.Admin {
		http.Error(rw, "Only admins can update global endpoints", http.StatusForbidden)
		return
	}

	// Only admins can add endpoints with API key path auth
	if existingEndpoint.APIKeyFromFile != "" && !user.Admin {
		http.Error(rw, "Only admins can add endpoints with API key path auth", http.StatusForbidden)
		return
	}

	// Preserve ID and ownership information
	existingEndpoint.Description = updatedEndpoint.Description
	existingEndpoint.Models = updatedEndpoint.Models
	existingEndpoint.BaseURL = updatedEndpoint.BaseURL
	if updatedEndpoint.APIKey != nil {
		existingEndpoint.APIKey = *updatedEndpoint.APIKey
	}

	if updatedEndpoint.APIKeyFromFile != nil {
		existingEndpoint.APIKeyFromFile = *updatedEndpoint.APIKeyFromFile
	}

	switch {
	case updatedEndpoint.APIKey != nil:
		// If from key, clear the API key file
		existingEndpoint.APIKey = *updatedEndpoint.APIKey
		existingEndpoint.APIKeyFromFile = ""
	case updatedEndpoint.APIKeyFromFile != nil:
		// If from file, clear the API key
		existingEndpoint.APIKeyFromFile = *updatedEndpoint.APIKeyFromFile
		existingEndpoint.APIKey = ""
	}

	// Update the endpoint
	savedEndpoint, err := s.Store.UpdateProviderEndpoint(ctx, existingEndpoint)
	if err != nil {
		log.Err(err).Msg("error updating provider endpoint")
		http.Error(rw, "Error updating provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Mask API key in response
	savedEndpoint.APIKey = "*****"

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(savedEndpoint); err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// deleteProviderEndpoint godoc
// @Summary Delete a provider endpoint
// @Description Delete a provider endpoint. Global endpoints cannot be deleted.
// @Tags    providers

// @Success 204 "No Content"
// @Router /api/v1/provider-endpoints/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteProviderEndpoint(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	endpointID := mux.Vars(r)["id"]

	// Get existing endpoint
	existingEndpoint, err := s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{ID: endpointID})
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Provider endpoint not found", http.StatusNotFound)
			return
		}
		log.Err(err).Msg("error getting provider endpoint")
		http.Error(rw, "Error getting provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Prevent deletion of global endpoints
	if existingEndpoint.EndpointType == types.ProviderEndpointTypeGlobal && !s.isAdmin(r) {
		http.Error(rw, "Global endpoints cannot be deleted", http.StatusForbidden)
		return
	}

	// Check ownership - only allow deletion of owned endpoints or if user is admin
	if existingEndpoint.Owner != user.ID && !user.Admin {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := s.Store.DeleteProviderEndpoint(ctx, endpointID); err != nil {
		log.Err(err).Msg("error deleting provider endpoint")
		http.Error(rw, "Error deleting provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(http.StatusOK)
}

// getProviderDailyUsage godoc
// @Summary Get provider daily usage
// @Description Get provider daily usage
// @Accept json
// @Produce json
// @Tags    providers
// @Param   id path string true "Provider ID"
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/provider-endpoints/{id}/daily-usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getProviderDailyUsage(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	id := getID(r)

	if !user.Admin && !s.Cfg.Providers.EnableCustomUserProviders {
		writeErrResponse(rw, errors.New("custom user providers are not enabled"), http.StatusForbidden)
		return
	}

	from := time.Now().Add(-time.Hour * 24 * 7) // Last 7 days
	to := time.Now()

	var err error

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("failed to parse from date: %w", err), http.StatusBadRequest)
			return
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("failed to parse to date: %w", err), http.StatusBadRequest)
			return
		}
	}

	visible, err := s.providerVisible(r.Context(), user, id)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error checking provider visibility: %w", err), http.StatusInternalServerError)
		return
	}

	if !visible {
		writeErrResponse(rw, errors.New("not authorized to access this provider"), http.StatusForbidden)
		return
	}

	metrics, err := s.Store.GetProviderDailyUsageMetrics(r.Context(), id, from, to)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error getting provider daily usage: %w", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, metrics, http.StatusOK)
}

// getProviderUsersDailyUsage godoc
// @Summary Get provider daily usage per user
// @Description Get provider daily usage per user
// @Accept json
// @Produce json
// @Tags    providers
// @Param   id path string true "Provider ID"
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Success 200 {array} types.UsersAggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/provider-endpoints/{id}/users-daily-usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getProviderUsersDailyUsage(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	id := getID(r)

	if !user.Admin && !s.Cfg.Providers.EnableCustomUserProviders {
		writeErrResponse(rw, errors.New("custom user providers are not enabled"), http.StatusForbidden)
		return
	}

	from := time.Now().Add(-time.Hour * 24 * 7) // Last 7 days
	to := time.Now()

	var err error

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("failed to parse from date: %w", err), http.StatusBadRequest)
			return
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("failed to parse to date: %w", err), http.StatusBadRequest)
			return
		}
	}

	visible, err := s.providerVisible(r.Context(), user, id)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error checking provider visibility: %w", err), http.StatusInternalServerError)
		return
	}

	if !visible {
		writeErrResponse(rw, errors.New("not authorized to access this provider"), http.StatusForbidden)
		return
	}

	metrics, err := s.Store.GetUsersAggregatedUsageMetrics(r.Context(), id, from, to)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error getting provider daily usage: %w", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, metrics, http.StatusOK)
}

func (s *HelixAPIServer) providerVisible(ctx context.Context, user *types.User, id string) (bool, error) {
	globalProviderEndpoints, err := s.providerManager.ListProviders(ctx, "")
	if err != nil {
		return false, fmt.Errorf("error listing providers: %w", err)
	}

	for _, provider := range globalProviderEndpoints {
		if string(provider) == id {
			return true, nil
		}
	}

	providerEndpoints, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
		Owner:      user.ID,
		OwnerType:  user.Type,
		WithGlobal: true,
	})
	if err != nil {
		log.Err(err).Msg("error listing provider endpoints")
		return false, fmt.Errorf("error listing provider endpoints: %w", err)
	}

	for _, endpoint := range providerEndpoints {
		if endpoint.ID == id || endpoint.Name == id {
			return true, nil
		}
	}

	return false, nil
}
