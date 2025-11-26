package server

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// registerWolfInstance godoc
// @Summary Register a new Wolf instance
// @Description Register a new Wolf streaming instance with the control plane
// @Tags    wolf
// @Accept  json
// @Produce json
// @Param request body types.WolfInstanceRequest true "Wolf instance registration"
// @Success 200 {object} types.WolfInstanceResponse
// @Failure 400 {string} string "Invalid request body"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/wolf-instances/register [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) registerWolfInstance(rw http.ResponseWriter, r *http.Request) {
	var req types.WolfInstanceRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Error().Err(err).Msg("error decoding registerWolfInstance request body")
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(rw, "name is required", http.StatusBadRequest)
		return
	}
	if req.Address == "" {
		http.Error(rw, "address is required", http.StatusBadRequest)
		return
	}

	// Create Wolf instance
	instance := &types.WolfInstance{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Address:      req.Address,
		MaxSandboxes: req.MaxSandboxes,
		GPUType:      req.GPUType,
	}

	// Set defaults
	if instance.MaxSandboxes == 0 {
		instance.MaxSandboxes = 10
	}

	err = apiServer.Store.RegisterWolfInstance(r.Context(), instance)
	if err != nil {
		log.Error().Err(err).Msg("error registering Wolf instance")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("wolf_id", instance.ID).
		Str("name", instance.Name).
		Str("address", instance.Address).
		Msg("Wolf instance registered")

	writeResponse(rw, instance.ToResponse(), http.StatusOK)
}

// wolfInstanceHeartbeat godoc
// @Summary Send heartbeat for a Wolf instance
// @Description Update the last heartbeat timestamp and optional metadata for a Wolf instance
// @Tags    wolf
// @Accept  json
// @Produce json
// @Param id path string true "Wolf instance ID"
// @Param request body types.WolfHeartbeatRequest false "Heartbeat metadata"
// @Success 200 {string} string "OK"
// @Failure 404 {string} string "Wolf instance not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/wolf-instances/{id}/heartbeat [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) wolfInstanceHeartbeat(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Parse optional request body for metadata (sway_version, etc.)
	var req types.WolfHeartbeatRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Non-fatal: old clients may send empty body
			log.Debug().Err(err).Str("wolf_id", id).Msg("error decoding heartbeat request body (ignoring)")
		}
	}

	err := apiServer.Store.UpdateWolfHeartbeat(r.Context(), id, req.SwayVersion)
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Wolf instance not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("wolf_id", id).Msg("error updating Wolf heartbeat")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Log version if provided (helps debugging)
	if req.SwayVersion != "" {
		log.Debug().
			Str("wolf_id", id).
			Str("sway_version", req.SwayVersion).
			Msg("Wolf heartbeat received with sway version")
	}

	writeResponse(rw, map[string]string{"status": "ok"}, http.StatusOK)
}

// listWolfInstances godoc
// @Summary List all Wolf instances
// @Description Get all registered Wolf streaming instances
// @Tags    wolf
// @Produce json
// @Success 200 {array} types.WolfInstanceResponse
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/wolf-instances [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listWolfInstances(rw http.ResponseWriter, r *http.Request) {
	instances, err := apiServer.Store.ListWolfInstances(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("error listing Wolf instances")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	responses := make([]*types.WolfInstanceResponse, len(instances))
	for i, instance := range instances {
		responses[i] = instance.ToResponse()
	}

	writeResponse(rw, responses, http.StatusOK)
}

// deregisterWolfInstance godoc
// @Summary Deregister a Wolf instance
// @Description Remove a Wolf instance from the registry
// @Tags    wolf
// @Success 200 {string} string "OK"
// @Failure 404 {string} string "Wolf instance not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/wolf-instances/{id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deregisterWolfInstance(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Check if instance exists
	_, err := apiServer.Store.GetWolfInstance(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Wolf instance not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("wolf_id", id).Msg("error getting Wolf instance")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = apiServer.Store.DeregisterWolfInstance(r.Context(), id)
	if err != nil {
		log.Error().Err(err).Str("wolf_id", id).Msg("error deregistering Wolf instance")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info().Str("wolf_id", id).Msg("Wolf instance deregistered")

	writeResponse(rw, map[string]string{"status": "ok"}, http.StatusOK)
}
