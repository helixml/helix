package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gorilla/mux"
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
// @Router /api/v1/providers-endpoints [get]
// @Security BearerAuth
func (s *HelixAPIServer) listProviderEndpoints(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)

	providerEndpoints, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
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

	// Sort endpoints by name, we will attach global ones to the end
	sort.Slice(providerEndpoints, func(i, j int) bool {
		return providerEndpoints[i].Name < providerEndpoints[j].Name
	})

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

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(providerEndpoints)
	if err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// createProviderEndpoint godoc
// @Summary Create a new provider endpoint
// @Description Create a new provider endpoint
// @Tags    providers

// @Success 200 {object} types.ProviderEndpoint
// @Router /api/v1/providers-endpoints [post]
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
// @Router /api/v1/providers-endpoints/{id} [put]
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
// @Router /api/v1/providers-endpoints/{id} [delete]
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
// @Router /api/v1/providers/{id}/daily-usage [get]
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

	existingEndpoint, err := s.Store.GetProviderEndpoint(r.Context(), &store.GetProviderEndpointsQuery{ID: id})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErrResponse(rw, errors.New("provider endpoint not found"), http.StatusNotFound)
			return
		}
		writeErrResponse(rw, fmt.Errorf("error getting provider endpoint: %w", err), http.StatusInternalServerError)
		return
	}

	// Only allow access to owned endpoints or global endpoints or if user is an admin
	if user.Admin {
		// OK, allowed
	} else {
		// Check if user is owner of endpoint or if it's global
		if existingEndpoint.Owner != user.ID && existingEndpoint.EndpointType != types.ProviderEndpointTypeGlobal {
			writeErrResponse(rw, errors.New("unauthorized"), http.StatusUnauthorized)
			return
		}
	}

	metrics, err := s.Store.GetProviderDailyUsageMetrics(r.Context(), id, from, to)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error getting provider daily usage: %w", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, metrics, http.StatusOK)
}
