package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// registerSandbox handles sandbox registration requests
// @Summary Register a sandbox instance
// @Description Register a new sandbox or update an existing one
// @Tags sandbox
// @Accept json
// @Produce json
// @Param sandbox body types.SandboxInstance true "Sandbox instance"
// @Success 200 {object} types.SandboxInstance
// @Router /api/v1/sandboxes/register [post]
func (apiServer *HelixAPIServer) registerSandbox(rw http.ResponseWriter, req *http.Request) {
	var instance types.SandboxInstance
	if err := json.NewDecoder(req.Body).Decode(&instance); err != nil {
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	if instance.ID == "" {
		http.Error(rw, "Sandbox ID is required", http.StatusBadRequest)
		return
	}

	// Set defaults
	if instance.Status == "" {
		instance.Status = "online"
	}
	instance.LastSeen = time.Now()

	if err := apiServer.Store.RegisterSandbox(req.Context(), &instance); err != nil {
		log.Error().Err(err).Str("sandbox_id", instance.ID).Msg("Failed to register sandbox")
		http.Error(rw, "Failed to register sandbox", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("sandbox_id", instance.ID).
		Str("hostname", instance.Hostname).
		Str("gpu_vendor", instance.GPUVendor).
		Msg("Sandbox registered")

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(instance)
}

// sandboxHeartbeat handles sandbox heartbeat requests
// @Summary Update sandbox heartbeat
// @Description Update sandbox status and metrics
// @Tags sandbox
// @Accept json
// @Produce json
// @Param id path string true "Sandbox ID"
// @Param heartbeat body types.SandboxHeartbeatRequest true "Heartbeat data"
// @Success 200 {object} map[string]string
// @Router /api/v1/sandboxes/{id}/heartbeat [post]
func (apiServer *HelixAPIServer) sandboxHeartbeat(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	sandboxID := vars["id"]

	var heartbeat types.SandboxHeartbeatRequest
	if err := json.NewDecoder(req.Body).Decode(&heartbeat); err != nil {
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update heartbeat
	if err := apiServer.Store.UpdateSandboxHeartbeat(req.Context(), sandboxID, &heartbeat); err != nil {
		log.Error().Err(err).Str("sandbox_id", sandboxID).Msg("Failed to update sandbox heartbeat")
		http.Error(rw, "Failed to update heartbeat", http.StatusInternalServerError)
		return
	}

	// Store disk usage history for alerting and trends
	for _, diskMetric := range heartbeat.DiskUsage {
		history := &types.DiskUsageHistory{
			ID:         uuid.New().String(),
			SandboxID:  sandboxID,
			MountPoint: diskMetric.MountPoint,
			TotalBytes: diskMetric.TotalBytes,
			UsedBytes:  diskMetric.UsedBytes,
			AvailBytes: diskMetric.AvailBytes,
			AlertLevel: diskMetric.AlertLevel,
		}
		if err := apiServer.Store.CreateDiskUsageHistory(req.Context(), history); err != nil {
			log.Warn().Err(err).Str("sandbox_id", sandboxID).Msg("Failed to store disk usage history")
		}
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]string{"status": "ok"})
}

// listSandboxes returns all registered sandbox instances
// @Summary List sandbox instances
// @Description Get all registered sandboxes
// @Tags sandbox
// @Produce json
// @Success 200 {array} types.SandboxInstance
// @Router /api/v1/sandboxes [get]
func (apiServer *HelixAPIServer) listSandboxes(rw http.ResponseWriter, req *http.Request) {
	instances, err := apiServer.Store.ListSandboxes(req.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to list sandboxes")
		http.Error(rw, "Failed to list sandboxes", http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(instances)
}

// deregisterSandbox removes a sandbox instance
// @Summary Deregister a sandbox instance
// @Description Remove a sandbox from the registry
// @Tags sandbox
// @Param id path string true "Sandbox ID"
// @Success 200 {object} map[string]string
// @Router /api/v1/sandboxes/{id} [delete]
func (apiServer *HelixAPIServer) deregisterSandbox(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	sandboxID := vars["id"]

	if err := apiServer.Store.DeregisterSandbox(req.Context(), sandboxID); err != nil {
		log.Error().Err(err).Str("sandbox_id", sandboxID).Msg("Failed to deregister sandbox")
		http.Error(rw, "Failed to deregister sandbox", http.StatusInternalServerError)
		return
	}

	log.Info().Str("sandbox_id", sandboxID).Msg("Sandbox deregistered")

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]string{"status": "deleted"})
}

// getDiskUsageHistory returns disk usage history for a sandbox
// @Summary Get disk usage history
// @Description Get disk usage history for trending and alerts
// @Tags sandbox
// @Produce json
// @Param id path string true "Sandbox ID"
// @Param since query string false "Since timestamp (RFC3339)"
// @Success 200 {array} types.DiskUsageHistory
// @Router /api/v1/sandboxes/{id}/disk-history [get]
func (apiServer *HelixAPIServer) getDiskUsageHistory(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	sandboxID := vars["id"]

	// Default to last 24 hours
	since := time.Now().Add(-24 * time.Hour)
	if sinceStr := req.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	history, err := apiServer.Store.GetDiskUsageHistory(req.Context(), sandboxID, since)
	if err != nil {
		log.Error().Err(err).Str("sandbox_id", sandboxID).Msg("Failed to get disk usage history")
		http.Error(rw, "Failed to get disk usage history", http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(history)
}
