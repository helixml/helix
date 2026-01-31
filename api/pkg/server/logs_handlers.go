package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// LogsSummary represents the summary of all logs across all runners
type LogsSummary struct {
	ActiveInstances     int              `json:"active_instances"`
	RecentErrors        int              `json:"recent_errors"`
	InstancesWithErrors int              `json:"instances_with_errors"`
	MaxLinesPerBuffer   int              `json:"max_lines_per_buffer"`
	ErrorRetentionHours int              `json:"error_retention_hours"`
	Slots               []SlotLogSummary `json:"slots"`
}

// SlotLogSummary represents a summary for a single slot's logs
type SlotLogSummary struct {
	ID       string `json:"id"`
	Model    string `json:"model"`
	RunnerID string `json:"runner_id"`
	HasLogs  bool   `json:"has_logs"`
}

// getLogsSummary godoc
// @Summary Get summary of logs across all runners
// @Description Retrieve a summary of all available logs by aggregating from all runners
// @Tags    logs
// @Produce json
// @Success 200 {object} LogsSummary
// @Failure 500 {object} string
func (s *HelixAPIServer) getLogsSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get all runners
	runners := s.Controller.Options.RunnerController.RunnerIDs()

	var totalActiveInstances int
	var totalRecentErrors int
	var totalInstancesWithErrors int
	var maxLinesPerBuffer int
	var errorRetentionHours float64
	var allSlots []SlotLogSummary

	// For each runner, get its log summary
	for _, runnerID := range runners {
		// Make request to runner's /logs endpoint
		req := &types.Request{
			Method: http.MethodGet,
			URL:    "/api/v1/logs",
		}

		resp, err := s.Controller.Options.RunnerController.Send(ctx, runnerID, nil, req, 10*time.Second)
		if err != nil {
			log.Warn().
				Err(err).
				Str("runner_id", runnerID).
				Msg("Failed to get logs summary from runner")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Warn().
				Int("status_code", resp.StatusCode).
				Str("runner_id", runnerID).
				Msg("Runner returned non-OK status for logs summary")
			continue
		}

		// Parse the runner's response - it returns summary statistics, not slots
		var runnerSummary struct {
			ActiveInstances     int     `json:"active_instances"`
			RecentErrors        int     `json:"recent_errors"`
			InstancesWithErrors int     `json:"instances_with_errors"`
			MaxLinesPerBuffer   int     `json:"max_lines_per_buffer"`
			ErrorRetentionHours float64 `json:"error_retention_hours"`
		}

		if err := json.Unmarshal(resp.Body, &runnerSummary); err != nil {
			log.Warn().
				Err(err).
				Str("runner_id", runnerID).
				Msg("Failed to decode logs summary from runner")
			continue
		}

		// Aggregate the statistics
		totalActiveInstances += runnerSummary.ActiveInstances
		totalRecentErrors += runnerSummary.RecentErrors
		totalInstancesWithErrors += runnerSummary.InstancesWithErrors

		// Use the max buffer size and retention settings from any runner
		if runnerSummary.MaxLinesPerBuffer > maxLinesPerBuffer {
			maxLinesPerBuffer = runnerSummary.MaxLinesPerBuffer
		}
		if runnerSummary.ErrorRetentionHours > errorRetentionHours {
			errorRetentionHours = runnerSummary.ErrorRetentionHours
		}

		// For now, we don't populate the slots array since the log manager
		// doesn't provide individual slot information in its summary
		// This could be enhanced later if needed
	}

	summary := LogsSummary{
		ActiveInstances:     totalActiveInstances,
		RecentErrors:        totalRecentErrors,
		InstancesWithErrors: totalInstancesWithErrors,
		MaxLinesPerBuffer:   maxLinesPerBuffer,
		ErrorRetentionHours: int(errorRetentionHours),
		Slots:               allSlots, // Empty for now
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		log.Error().Err(err).Msg("Failed to encode logs summary response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// getSlotLogs godoc
// @Summary Get logs for a specific slot
// @Description Retrieve logs for a specific slot by proxying the request to the runner
// @Tags    logs
// @Produce json
// @Param   slot_id  path     string  true   "Slot ID"
// @Param   lines    query    int     false  "Maximum number of lines to return (default: 500)"
// @Param   since    query    string  false  "Return logs since this timestamp (RFC3339 format)"
// @Param   level    query    string  false  "Filter by log level (ERROR, WARN, INFO, DEBUG)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} string
// @Failure 404 {object} string
// @Failure 500 {object} string
// @Router /api/v1/logs/{slot_id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSlotLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slotIDStr := vars["slot_id"]

	if slotIDStr == "" {
		http.Error(w, "slot_id is required", http.StatusBadRequest)
		return
	}

	// Parse slot ID
	slotID, err := uuid.Parse(slotIDStr)
	if err != nil {
		http.Error(w, "invalid slot_id format", http.StatusBadRequest)
		return
	}

	// Find which runner has this slot
	runnerID, err := s.findRunnerForSlot(slotID)
	if err != nil {
		log.Error().
			Err(err).
			Str("slot_id", slotIDStr).
			Msg("failed to find runner for slot")
		http.Error(w, "slot not found or runner unavailable", http.StatusNotFound)
		return
	}

	// Build the request URL with query parameters
	requestURL := fmt.Sprintf("/api/v1/logs/%s", slotIDStr)
	if r.URL.RawQuery != "" {
		requestURL += "?" + r.URL.RawQuery
	}

	log.Debug().
		Str("slot_id", slotIDStr).
		Str("runner_id", runnerID).
		Str("request_url", requestURL).
		Msg("proxying logs request to runner")

	// Create request to send to runner
	req := &types.Request{
		Method: "GET",
		URL:    requestURL,
		Body:   nil,
	}

	// Send request to runner with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := s.Controller.Options.RunnerController.Send(ctx, runnerID, nil, req, 30*time.Second)
	if err != nil {
		log.Error().
			Err(err).
			Str("slot_id", slotIDStr).
			Str("runner_id", runnerID).
			Msg("failed to get logs from runner")
		http.Error(w, "failed to retrieve logs from runner", http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)

	// Write response body
	if _, err := w.Write(resp.Body); err != nil {
		log.Error().
			Err(err).
			Str("slot_id", slotIDStr).
			Str("runner_id", runnerID).
			Msg("failed to write logs response")
	}

	log.Debug().
		Str("slot_id", slotIDStr).
		Str("runner_id", runnerID).
		Int("status_code", resp.StatusCode).
		Int("response_size", len(resp.Body)).
		Msg("successfully proxied logs request")
}

// findRunnerForSlot finds which runner has the specified slot
func (s *HelixAPIServer) findRunnerForSlot(slotID uuid.UUID) (string, error) {
	// Access the scheduler's slots to find which runner has this slot
	scheduler := s.Controller.Options.Scheduler
	if scheduler == nil {
		return "", fmt.Errorf("scheduler not available")
	}

	// Search through all slots to find the one with matching ID
	// We need to access the slots field directly since it's not exposed via a public method
	var runnerID string
	var found bool

	// Use reflection or find another way to access scheduler slots
	// For now, let's search through all runners to find the slot
	runners := s.Controller.Options.RunnerController.RunnerIDs()
	for _, rid := range runners {
		slots, err := s.Controller.Options.RunnerController.GetSlots(rid)
		if err != nil {
			continue // Skip this runner if we can't get its slots
		}

		for _, slot := range slots {
			if slot.ID == slotID {
				runnerID = rid
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return "", fmt.Errorf("slot %s not found on any runner", slotID.String())
	}

	return runnerID, nil
}
