package server

import (
	"net/http"

	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listModels godoc
// @Summary List models
// @Description List models
// @Tags    models

// @Success 200 {array} types.OpenAIModelsList
// @Param provider query string false "Provider"
// @Router /v1/models [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listModels(rw http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = apiServer.Cfg.Inference.Provider
	}

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
