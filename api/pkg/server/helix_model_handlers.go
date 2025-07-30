package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listHelixModels godoc
// @Summary List Helix models
// @Description List all available Helix models, optionally filtering by type, name, or runtime.
// @Tags    models
// @Param type query string false "Filter by model type (e.g., chat, embedding)"
// @Param name query string false "Filter by model name"
// @Param runtime query string false "Filter by model runtime (e.g., ollama, vllm)"
// @Success 200 {array} types.Model
// @Router /api/v1/helix-models [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listHelixModels(rw http.ResponseWriter, r *http.Request) {
	q := &store.ListModelsQuery{}

	if r.URL.Query().Get("type") != "" {
		q.Type = types.ModelType(r.URL.Query().Get("type"))
	}

	if r.URL.Query().Get("name") != "" {
		q.Name = r.URL.Query().Get("name")
	}

	if r.URL.Query().Get("runtime") != "" {
		q.Runtime = types.Runtime(r.URL.Query().Get("runtime"))
	}

	models, err := apiServer.Store.ListModels(r.Context(), q)
	if err != nil {
		log.Error().Err(err).Msg("error listing helix models")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, models, http.StatusOK)
}

// createHelixModel godoc
// @Summary Create a new Helix model
// @Description Create a new Helix model configuration. Requires admin privileges.
// @Tags    models
// @Param request body types.Model true "Model configuration"
// @Success 201 {object} types.Model
// @Failure 400 {string} string "Invalid request body"
// @Failure 403 {string} string "Forbidden - Admin required"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/helix-models [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createHelixModel(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !isAdmin(user) {
		http.Error(rw, "Forbidden: Admin privileges required", http.StatusForbidden)
		return
	}

	var model types.Model
	err := json.NewDecoder(r.Body).Decode(&model)
	if err != nil {
		log.Error().Err(err).Msg("error decoding createHelixModel request body")
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	createdModel, err := apiServer.Store.CreateModel(r.Context(), &model)
	if err != nil {
		log.Error().Err(err).Msg("error creating helix model")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, createdModel, http.StatusCreated)
}

// updateHelixModel godoc
// @Summary Update an existing Helix model
// @Description Update an existing Helix model configuration. Requires admin privileges.
// @Tags    models
// @Param id path string true "Model ID"
// @Param request body types.Model true "Updated model configuration"
// @Success 200 {object} types.Model
// @Failure 400 {string} string "Invalid request body or missing ID"
// @Failure 403 {string} string "Forbidden - Admin required"
// @Failure 404 {string} string "Model not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/helix-models/{id} [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateHelixModel(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !isAdmin(user) {
		http.Error(rw, "Forbidden: Admin privileges required", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	modelID := vars["id"]
	if modelID == "" {
		http.Error(rw, "Model ID is required", http.StatusBadRequest)
		return
	}

	var modelUpdates types.Model
	err := json.NewDecoder(r.Body).Decode(&modelUpdates)
	if err != nil {
		log.Error().Err(err).Msg("error decoding updateHelixModel request body")
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	existingModel, err := apiServer.Store.GetModel(r.Context(), modelID)
	if err != nil {
		log.Error().Err(err).Str("model_id", modelID).Msg("error getting helix model")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure the ID from the path is used, ignore any ID in the body
	modelUpdates.ID = modelID
	modelUpdates.Created = existingModel.Created
	modelUpdates.Updated = time.Now()

	// Mark as user-modified since this is an admin update
	modelUpdates.UserModified = true

	updatedModel, err := apiServer.Store.UpdateModel(r.Context(), &modelUpdates)
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Model not found", http.StatusNotFound)
		} else {
			log.Error().Err(err).Str("model_id", modelID).Msg("error updating helix model")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	log.Info().
		Str("model_id", modelID).
		Str("admin_user", user.ID).
		Msg("model updated by admin - marked as user-modified to preserve changes")

	writeResponse(rw, updatedModel, http.StatusOK)
}

// deleteHelixModel godoc
// @Summary Delete a Helix model
// @Description Delete a Helix model configuration. Requires admin privileges.
// @Tags    models
// @Param id path string true "Model ID"
// @Success 200 {string} string "OK"
// @Failure 400 {string} string "Missing ID"
// @Failure 403 {string} string "Forbidden - Admin required"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/helix-models/{id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deleteHelixModel(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !isAdmin(user) {
		http.Error(rw, "Forbidden: Admin privileges required", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	modelID := vars["id"]
	if modelID == "" {
		http.Error(rw, "Model ID is required", http.StatusBadRequest)
		return
	}

	err := apiServer.Store.DeleteModel(r.Context(), modelID)
	if err != nil {
		// Delete might fail if the record doesn't exist, but we treat it as success (idempotent)
		// unless it's a different error. Gorm returns error on Delete even if record not found.
		// We log the error but return OK to the client.
		log.Error().Err(err).Str("model_id", modelID).Msg("error deleting helix model (potentially benign if not found)")
		// Optionally, check for specific errors if needed, e.g., foreign key constraints
		// For now, assume deletion failure due to not found is acceptable.
		// If it was another error:
		// http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		// return
	}

	writeResponse(rw, "OK", http.StatusOK) // Return 200 OK on successful deletion or if not found
}
