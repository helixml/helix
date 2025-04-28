package server

import (
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (apiServer *HelixAPIServer) listHelixModels(rw http.ResponseWriter, r *http.Request) {
	q := &store.ListModelsQuery{}

	if r.URL.Query().Get("type") != "" {
		q.Type = types.ModelType(r.URL.Query().Get("type"))
	}

	if r.URL.Query().Get("name") != "" {
		q.Name = r.URL.Query().Get("name")
	}

	if r.URL.Query().Get("runtime") != "" {
		q.Runtime = types.ModelRuntimeType(r.URL.Query().Get("runtime"))
	}

	models, err := apiServer.Store.ListModels(r.Context(), q)
	if err != nil {
		log.Error().Err(err).Msg("error listing helix models")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, models, http.StatusOK)
}

func (apiServer *HelixAPIServer) createHelixModel(rw http.ResponseWriter, r *http.Request) {

}

func (apiServer *HelixAPIServer) updateHelixModel(rw http.ResponseWriter, r *http.Request) {

}

func (apiServer *HelixAPIServer) deleteHelixModel(rw http.ResponseWriter, r *http.Request) {

}
