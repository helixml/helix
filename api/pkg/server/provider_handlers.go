package server

import (
	"encoding/json"
	"fmt"
	"net/http"

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
func (apiServer *HelixAPIServer) listProviders(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)

	providers, err := apiServer.providerManager.ListProviders(r.Context(), user.ID)
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

// listProviderEndpoints godoc
// @Summary List currently configured provider endpoints
// @Description List currently configured providers
// @Tags    providers

// @Success 200 {array} types.ProviderEndpoint
// @Router /api/v1/providers-endpoints [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listProviderEndpoints(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)

	providerEndpoints, err := apiServer.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
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
		providerEndpoints[idx].APIKey = "*****"
	}

	// Get global ones from the provider manager
	globalProviderEndpoints, err := apiServer.providerManager.ListProviders(ctx, "")
	if err != nil {
		log.Err(err).Msg("error listing providers")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for _, provider := range globalProviderEndpoints {
		var baseURL string
		switch provider {
		case types.ProviderOpenAI:
			baseURL = apiServer.Cfg.Providers.OpenAI.BaseURL
		case types.ProviderTogetherAI:
			baseURL = apiServer.Cfg.Providers.TogetherAI.BaseURL
		case types.ProviderVLLM:
			baseURL = apiServer.Cfg.Providers.VLLM.BaseURL
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
			APIKey:       "*****",
		})
	}

	// Set default
	for idx := range providerEndpoints {
		if providerEndpoints[idx].Name == apiServer.Cfg.Inference.Provider {
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
func (apiServer *HelixAPIServer) createProviderEndpoint(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)

	isAdmin := apiServer.isAdmin(r)

	if !isAdmin && !apiServer.Cfg.Providers.EnableCustomUserProviders {
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
	existingProviders, err := apiServer.providerManager.ListProviders(r.Context(), user.ID)
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

	createdEndpoint, err := apiServer.Store.CreateProviderEndpoint(ctx, &endpoint)
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

// @Success 200 {object} types.ProviderEndpoint
// @Router /api/v1/providers-endpoints/{id} [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateProviderEndpoint(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	endpointID := mux.Vars(r)["id"]

	if !user.Admin && !apiServer.Cfg.Providers.EnableCustomUserProviders {
		http.Error(rw, "Custom user providers are not enabled", http.StatusForbidden)
		return
	}

	// Get existing endpoint
	existingEndpoint, err := apiServer.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{ID: endpointID})
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

	var updatedEndpoint types.ProviderEndpoint
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

	if existingEndpoint.Name != updatedEndpoint.Name {
		http.Error(rw, "Cannot change the name of a provider endpoint, create a new one", http.StatusBadRequest)
		return
	}

	// Preserve ID and ownership information
	updatedEndpoint.ID = endpointID
	updatedEndpoint.Owner = existingEndpoint.Owner
	updatedEndpoint.OwnerType = existingEndpoint.OwnerType

	// Update the endpoint
	savedEndpoint, err := apiServer.Store.UpdateProviderEndpoint(ctx, &updatedEndpoint)
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
func (apiServer *HelixAPIServer) deleteProviderEndpoint(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	endpointID := mux.Vars(r)["id"]

	// Get existing endpoint
	existingEndpoint, err := apiServer.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{ID: endpointID})
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
	if existingEndpoint.EndpointType == types.ProviderEndpointTypeGlobal && !apiServer.isAdmin(r) {
		http.Error(rw, "Global endpoints cannot be deleted", http.StatusForbidden)
		return
	}

	// Check ownership - only allow deletion of owned endpoints or if user is admin
	if existingEndpoint.Owner != user.ID && !user.Admin {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := apiServer.Store.DeleteProviderEndpoint(ctx, endpointID); err != nil {
		log.Err(err).Msg("error deleting provider endpoint")
		http.Error(rw, "Error deleting provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(http.StatusOK)
}
