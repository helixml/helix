package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/notification"
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

	// Parse optional request body for metadata (desktop_versions, disk_usage, etc.)
	var req types.WolfHeartbeatRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Non-fatal: old clients may send empty body
			log.Debug().Err(err).Str("wolf_id", id).Msg("error decoding heartbeat request body (ignoring)")
		}
	}

	// Get instance before update to check for alert state changes
	instance, err := apiServer.Store.GetWolfInstance(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Wolf instance not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("wolf_id", id).Msg("error getting Wolf instance")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	previousAlertLevel := instance.DiskAlertLevel

	// Update heartbeat with disk metrics
	err = apiServer.Store.UpdateWolfHeartbeat(r.Context(), id, &req)
	if err != nil {
		log.Error().Err(err).Str("wolf_id", id).Msg("error updating Wolf heartbeat")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check for disk alert level changes and send notifications
	if len(req.DiskUsage) > 0 {
		// Store disk usage history for time-series visualization
		apiServer.storeDiskUsageHistory(r.Context(), id, &req)

		// Determine new highest alert level
		newAlertLevel := "ok"
		for _, disk := range req.DiskUsage {
			if disk.AlertLevel == "critical" {
				newAlertLevel = "critical"
				break
			} else if disk.AlertLevel == "warning" && newAlertLevel != "critical" {
				newAlertLevel = "warning"
			}
		}

		// Send alert if level increased (ok->warning, ok->critical, or warning->critical)
		shouldAlert := false
		if newAlertLevel == "critical" && previousAlertLevel != "critical" {
			shouldAlert = true
		} else if newAlertLevel == "warning" && previousAlertLevel == "ok" {
			shouldAlert = true
		}

		if shouldAlert {
			// Send Slack alert
			if apiServer.Janitor != nil {
				alertMsg := apiServer.buildDiskAlertMessage(instance.Name, id, req.DiskUsage, newAlertLevel)
				if err := apiServer.Janitor.SendMessage("", alertMsg); err != nil {
					log.Error().Err(err).Str("wolf_id", id).Msg("failed to send disk alert to Slack")
				} else {
					log.Info().Str("wolf_id", id).Str("alert_level", newAlertLevel).Msg("Disk alert sent to Slack")
				}
			}

			// Send email alert to admin users
			if apiServer.adminAlerter != nil {
				emailData := apiServer.buildDiskAlertEmailData(instance.Name, id, req.DiskUsage, newAlertLevel)
				if err := apiServer.adminAlerter.SendDiskSpaceAlert(r.Context(), emailData); err != nil {
					log.Error().Err(err).Str("wolf_id", id).Msg("failed to send disk alert email to admins")
				} else {
					log.Info().Str("wolf_id", id).Str("alert_level", newAlertLevel).Msg("Disk alert email sent to admins")
				}
			}
		}
	}

	writeResponse(rw, map[string]string{"status": "ok"}, http.StatusOK)
}

// buildDiskAlertMessage formats a disk usage alert for Slack
func (apiServer *HelixAPIServer) buildDiskAlertMessage(wolfName, wolfID string, diskUsage []types.DiskUsageMetric, alertLevel string) string {
	icon := "⚠️"
	if alertLevel == "critical" {
		icon = "🚨"
	}

	msg := icon + " *Disk Space Alert* - " + wolfName + "\n\n"
	msg += "Wolf Instance: `" + wolfID + "`\n"
	msg += "Alert Level: *" + alertLevel + "*\n\n"

	for _, disk := range diskUsage {
		if disk.AlertLevel != "ok" {
			usedGB := float64(disk.UsedBytes) / (1024 * 1024 * 1024)
			totalGB := float64(disk.TotalBytes) / (1024 * 1024 * 1024)
			availGB := float64(disk.AvailBytes) / (1024 * 1024 * 1024)
			msg += "• `" + disk.MountPoint + "`: " +
				"*" + formatFloat(disk.UsedPercent) + "%* used " +
				"(" + formatFloat(usedGB) + "GB / " + formatFloat(totalGB) + "GB, " +
				formatFloat(availGB) + "GB free)\n"
		}
	}

	return msg
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.1f", f)
}

// lastDiskHistoryCleanup tracks when we last cleaned up old disk usage history
var lastDiskHistoryCleanup time.Time

// storeDiskUsageHistory stores disk usage data for time-series visualization
// and periodically cleans up old data (older than 7 days)
func (apiServer *HelixAPIServer) storeDiskUsageHistory(ctx context.Context, wolfInstanceID string, req *types.WolfHeartbeatRequest) {
	now := time.Now()

	// Store history for each mount point
	for _, disk := range req.DiskUsage {
		// Encode container usage if present
		var containerJSON string
		if len(req.ContainerUsage) > 0 {
			if data, err := json.Marshal(req.ContainerUsage); err == nil {
				containerJSON = string(data)
			}
		}

		history := &types.DiskUsageHistory{
			ID:             uuid.New().String(),
			WolfInstanceID: wolfInstanceID,
			Timestamp:      now,
			MountPoint:     disk.MountPoint,
			TotalBytes:     disk.TotalBytes,
			UsedBytes:      disk.UsedBytes,
			AvailBytes:     disk.AvailBytes,
			UsedPercent:    disk.UsedPercent,
			AlertLevel:     disk.AlertLevel,
			ContainerUsage: containerJSON,
		}

		if err := apiServer.Store.CreateDiskUsageHistory(ctx, history); err != nil {
			log.Error().Err(err).
				Str("wolf_id", wolfInstanceID).
				Str("mount_point", disk.MountPoint).
				Msg("failed to store disk usage history")
		}
	}

	// Cleanup old data every hour (time-based, deterministic)
	if now.Sub(lastDiskHistoryCleanup) > time.Hour {
		lastDiskHistoryCleanup = now
		cutoff := now.Add(-7 * 24 * time.Hour) // 7 days retention
		deleted, err := apiServer.Store.DeleteOldDiskUsageHistory(ctx, cutoff)
		if err != nil {
			log.Error().Err(err).Msg("failed to cleanup old disk usage history")
		} else if deleted > 0 {
			log.Info().Int64("deleted", deleted).Msg("cleaned up old disk usage history records")
		}
	}
}

// buildDiskAlertEmailData builds data for disk space alert emails
func (apiServer *HelixAPIServer) buildDiskAlertEmailData(wolfName, wolfID string, diskUsage []types.DiskUsageMetric, alertLevel string) *notification.DiskSpaceAlertData {
	data := &notification.DiskSpaceAlertData{
		WolfName:     wolfName,
		WolfID:       wolfID,
		AlertLevel:   alertLevel,
		DashboardURL: apiServer.Cfg.WebServer.URL + "/external-agents",
		DiskMetrics:  make([]notification.DiskMetricData, 0),
	}

	for _, disk := range diskUsage {
		if disk.AlertLevel != "ok" {
			usedGB := float64(disk.UsedBytes) / (1024 * 1024 * 1024)
			totalGB := float64(disk.TotalBytes) / (1024 * 1024 * 1024)
			availGB := float64(disk.AvailBytes) / (1024 * 1024 * 1024)

			data.DiskMetrics = append(data.DiskMetrics, notification.DiskMetricData{
				MountPoint:  disk.MountPoint,
				UsedPercent: formatFloat(disk.UsedPercent),
				UsedGB:      formatFloat(usedGB),
				TotalGB:     formatFloat(totalGB),
				AvailGB:     formatFloat(availGB),
				AlertLevel:  disk.AlertLevel,
			})
		}
	}

	return data
}

// getDiskUsageHistory godoc
// @Summary Get disk usage history for a Wolf instance
// @Description Get time-series disk usage data for visualization (last 7 days)
// @Tags    wolf
// @Produce json
// @Param id path string true "Wolf instance ID"
// @Param hours query int false "Hours of history to return (default 24, max 168)"
// @Success 200 {object} types.DiskUsageHistoryResponse
// @Failure 404 {string} string "Wolf instance not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/wolf-instances/{id}/disk-history [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getDiskUsageHistory(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Get the wolf instance to verify it exists and get the name
	instance, err := apiServer.Store.GetWolfInstance(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Wolf instance not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("wolf_id", id).Msg("error getting Wolf instance")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse hours parameter (default 24, max 168 = 7 days)
	hours := 24
	if hoursParam := r.URL.Query().Get("hours"); hoursParam != "" {
		if h, err := strconv.Atoi(hoursParam); err == nil && h > 0 && h <= 168 {
			hours = h
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	history, err := apiServer.Store.GetDiskUsageHistory(r.Context(), id, since)
	if err != nil {
		log.Error().Err(err).Str("wolf_id", id).Msg("error getting disk usage history")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to response format with MB instead of bytes
	dataPoints := make([]types.DiskUsageDataPoint, len(history))
	containerMap := make(map[string]uint64) // Latest container usage

	for i, h := range history {
		dataPoints[i] = types.DiskUsageDataPoint{
			Timestamp:   h.Timestamp,
			MountPoint:  h.MountPoint,
			TotalMB:     h.TotalBytes / (1024 * 1024),
			UsedMB:      h.UsedBytes / (1024 * 1024),
			AvailMB:     h.AvailBytes / (1024 * 1024),
			UsedPercent: h.UsedPercent,
			AlertLevel:  h.AlertLevel,
		}

		// Extract container usage from the most recent entry
		if h.ContainerUsage != "" {
			var containers []types.ContainerDiskUsage
			if err := json.Unmarshal([]byte(h.ContainerUsage), &containers); err == nil {
				for _, c := range containers {
					containerMap[c.ContainerName] = c.SizeBytes / (1024 * 1024)
				}
			}
		}
	}

	// Convert container map to summary list
	containerSummaries := make([]types.ContainerDiskUsageSummary, 0, len(containerMap))
	for name, sizeMB := range containerMap {
		containerSummaries = append(containerSummaries, types.ContainerDiskUsageSummary{
			ContainerName: name,
			LatestSizeMB:  sizeMB,
		})
	}

	response := &types.DiskUsageHistoryResponse{
		WolfInstanceID: id,
		WolfName:       instance.Name,
		History:        dataPoints,
		Containers:     containerSummaries,
	}

	writeResponse(rw, response, http.StatusOK)
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
