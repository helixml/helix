package server

import (
	"encoding/json"
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
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
	providers, err := apiServer.providerManager.ListProviders(r.Context())
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

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(providerEndpoints)
	if err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
}
