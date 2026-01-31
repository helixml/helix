package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listDynamicModelInfos godoc
// @Summary List dynamic model infos
// @Description List all dynamic model infos. Requires admin privileges.
// @Tags    model-info
// @Param provider query string false "Filter by provider (e.g., helix, openai)"
// @Param name query string false "Filter by model name"
// @Success 200 {array} types.DynamicModelInfo
// @Failure 403 {string} string "Forbidden - Admin required"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/model-info [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listDynamicModelInfos(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !isAdmin(user) {
		http.Error(rw, "Forbidden: Admin privileges required", http.StatusForbidden)
		return
	}

	q := &types.ListDynamicModelInfosQuery{}

	if r.URL.Query().Get("provider") != "" {
		q.Provider = r.URL.Query().Get("provider")
	}

	if r.URL.Query().Get("name") != "" {
		q.Name = r.URL.Query().Get("name")
	}

	modelInfos, err := apiServer.Store.ListDynamicModelInfos(r.Context(), q)
	if err != nil {
		log.Error().Err(err).Msg("error listing dynamic model infos")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, modelInfos, http.StatusOK)
}

// createDynamicModelInfo godoc
// @Summary Create a new dynamic model info
// @Description Create a new dynamic model info configuration. Requires admin privileges.
// @Tags    model-info
// @Param request body types.DynamicModelInfo true "Dynamic model info configuration"
// @Success 201 {object} types.DynamicModelInfo
// @Failure 400 {string} string "Invalid request body"
// @Failure 403 {string} string "Forbidden - Admin required"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/model-info [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createDynamicModelInfo(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !isAdmin(user) {
		http.Error(rw, "Forbidden: Admin privileges required", http.StatusForbidden)
		return
	}

	var modelInfo types.DynamicModelInfo
	err := json.NewDecoder(r.Body).Decode(&modelInfo)
	if err != nil {
		log.Error().Err(err).Msg("error decoding createDynamicModelInfo request body")
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if modelInfo.ModelInfo.Pricing.Prompt != "" {
		// Try parsing float from prompt price
		_, err := strconv.ParseFloat(modelInfo.ModelInfo.Pricing.Prompt, 64)
		if err != nil {
			http.Error(rw, "Invalid prompt price: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if modelInfo.ModelInfo.Pricing.Completion != "" {
		// Try parsing float from completion price
		_, err := strconv.ParseFloat(modelInfo.ModelInfo.Pricing.Completion, 64)
		if err != nil {
			http.Error(rw, "Invalid completion price: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if modelInfo.ModelInfo.Pricing.Request != "" {
		// Try parsing float from request price
		_, err := strconv.ParseFloat(modelInfo.ModelInfo.Pricing.Request, 64)
		if err != nil {
			http.Error(rw, "Invalid request price: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	createdModelInfo, err := apiServer.Store.CreateDynamicModelInfo(r.Context(), &modelInfo)
	if err != nil {
		log.Error().Err(err).Msg("error creating dynamic model info")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("model_info_id", createdModelInfo.ID).
		Str("admin_user", user.ID).
		Msg("dynamic model info created by admin")

	writeResponse(rw, createdModelInfo, http.StatusCreated)
}

// getDynamicModelInfo godoc
// @Summary Get a dynamic model info by ID
// @Description Get a specific dynamic model info by ID. Requires admin privileges.
// @Tags    model-info
// @Param id path string true "Dynamic model info ID"
// @Success 200 {object} types.DynamicModelInfo
// @Failure 400 {string} string "Missing ID"
// @Failure 403 {string} string "Forbidden - Admin required"
// @Failure 404 {string} string "Dynamic model info not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/model-info/{id} [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getDynamicModelInfo(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !isAdmin(user) {
		http.Error(rw, "Forbidden: Admin privileges required", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	modelInfoID := vars["id"]
	if modelInfoID == "" {
		http.Error(rw, "Dynamic model info ID is required", http.StatusBadRequest)
		return
	}

	modelInfo, err := apiServer.Store.GetDynamicModelInfo(r.Context(), modelInfoID)
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Dynamic model info not found", http.StatusNotFound)
		} else {
			log.Error().Err(err).Str("model_info_id", modelInfoID).Msg("error getting dynamic model info")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	writeResponse(rw, modelInfo, http.StatusOK)
}

// updateDynamicModelInfo godoc
// @Summary Update an existing dynamic model info
// @Description Update an existing dynamic model info configuration. Requires admin privileges.
// @Tags    model-info
// @Param id path string true "Dynamic model info ID"
// @Param request body types.DynamicModelInfo true "Updated dynamic model info configuration"
// @Success 200 {object} types.DynamicModelInfo
// @Failure 400 {string} string "Invalid request body or missing ID"
// @Failure 403 {string} string "Forbidden - Admin required"
// @Failure 404 {string} string "Dynamic model info not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/model-info/{id} [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateDynamicModelInfo(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !isAdmin(user) {
		http.Error(rw, "Forbidden: Admin privileges required", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	modelInfoID := vars["id"]
	if modelInfoID == "" {
		http.Error(rw, "Dynamic model info ID is required", http.StatusBadRequest)
		return
	}

	var modelInfoUpdates types.DynamicModelInfo
	err := json.NewDecoder(r.Body).Decode(&modelInfoUpdates)
	if err != nil {
		log.Error().Err(err).Msg("error decoding updateDynamicModelInfo request body")
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if modelInfoUpdates.ModelInfo.Pricing.Prompt != "" {
		// Try parsing float from prompt price
		_, err := strconv.ParseFloat(modelInfoUpdates.ModelInfo.Pricing.Prompt, 64)
		if err != nil {
			http.Error(rw, "Invalid prompt price: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if modelInfoUpdates.ModelInfo.Pricing.Completion != "" {
		// Try parsing float from completion price
		_, err := strconv.ParseFloat(modelInfoUpdates.ModelInfo.Pricing.Completion, 64)
		if err != nil {
			http.Error(rw, "Invalid completion price: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if modelInfoUpdates.ModelInfo.Pricing.Request != "" {
		// Try parsing float from request price
		_, err := strconv.ParseFloat(modelInfoUpdates.ModelInfo.Pricing.Request, 64)
		if err != nil {
			http.Error(rw, "Invalid request price: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	existingModelInfo, err := apiServer.Store.GetDynamicModelInfo(r.Context(), modelInfoID)
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Dynamic model info not found", http.StatusNotFound)
		} else {
			log.Error().Err(err).Str("model_info_id", modelInfoID).Msg("error getting dynamic model info")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Ensure the ID from the path is used, ignore any ID in the body
	modelInfoUpdates.ID = modelInfoID
	modelInfoUpdates.Created = existingModelInfo.Created
	modelInfoUpdates.Updated = time.Now()

	updatedModelInfo, err := apiServer.Store.UpdateDynamicModelInfo(r.Context(), &modelInfoUpdates)
	if err != nil {
		log.Error().Err(err).Str("model_info_id", modelInfoID).Msg("error updating dynamic model info")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("model_info_id", modelInfoID).
		Str("admin_user", user.ID).
		Msg("dynamic model info updated by admin")

	writeResponse(rw, updatedModelInfo, http.StatusOK)
}

// deleteDynamicModelInfo godoc
// @Summary Delete a dynamic model info
// @Description Delete a dynamic model info configuration. Requires admin privileges.
// @Tags    model-info
// @Param id path string true "Dynamic model info ID"
// @Success 200 {string} string "OK"
// @Failure 400 {string} string "Missing ID"
// @Failure 403 {string} string "Forbidden - Admin required"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/model-info/{id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deleteDynamicModelInfo(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !isAdmin(user) {
		http.Error(rw, "Forbidden: Admin privileges required", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	modelInfoID := vars["id"]
	if modelInfoID == "" {
		http.Error(rw, "Dynamic model info ID is required", http.StatusBadRequest)
		return
	}

	err := apiServer.Store.DeleteDynamicModelInfo(r.Context(), modelInfoID)
	if err != nil {
		log.Error().Err(err).Str("model_info_id", modelInfoID).Msg("error deleting dynamic model info")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("model_info_id", modelInfoID).
		Str("admin_user", user.ID).
		Msg("dynamic model info deleted by admin")

	writeResponse(rw, "OK", http.StatusOK)
}
