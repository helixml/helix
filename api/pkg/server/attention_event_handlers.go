package server

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listAttentionEvents godoc
// @Summary List active attention events
// @Description Returns attention events that need human action for the current user. Only returns events that have not been dismissed and are not currently snoozed.
// @Tags    attention-events
// @Produce json
// @Param   active query bool false "Filter to active (non-dismissed, non-snoozed) events only (default: true)"
// @Success 200 {array} types.AttentionEvent
// @Failure 401 {string} string "unauthorized"
// @Failure 500 {string} string "internal server error"
// @Router  /api/v1/attention-events [get]
func (s *HelixAPIServer) listAttentionEvents(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	organizationID := user.OrganizationID

	events, err := s.Store.ListAttentionEvents(r.Context(), user.ID, organizationID)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to list attention events")
		http.Error(w, "failed to list attention events", http.StatusInternalServerError)
		return
	}

	if events == nil {
		events = []*types.AttentionEvent{}
	}

	jsonBytes, err := json.Marshal(events)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	h := fnv.New64a()
	h.Write(jsonBytes)
	etag := fmt.Sprintf(`"%x"`, h.Sum64())

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache, must-revalidate")

	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes) //nolint:errcheck
}

// updateAttentionEvent godoc
// @Summary Update an attention event
// @Description Acknowledge, dismiss, or snooze an attention event.
// @Tags    attention-events
// @Accept  json
// @Produce json
// @Param   id path string true "Attention event ID"
// @Param   request body types.AttentionEventUpdateRequest true "Update request"
// @Success 200 {object} map[string]string
// @Failure 400 {string} string "bad request"
// @Failure 401 {string} string "unauthorized"
// @Failure 404 {string} string "not found"
// @Failure 500 {string} string "internal server error"
// @Router  /api/v1/attention-events/{id} [patch]
func (s *HelixAPIServer) updateAttentionEvent(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	eventID := vars["id"]
	if eventID == "" {
		http.Error(w, "event ID is required", http.StatusBadRequest)
		return
	}

	// Verify the event belongs to this user.
	event, err := s.Store.GetAttentionEvent(r.Context(), eventID)
	if err != nil {
		http.Error(w, "attention event not found", http.StatusNotFound)
		return
	}
	if event.UserID != user.ID {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req types.AttentionEventUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.Store.UpdateAttentionEvent(r.Context(), eventID, &req); err != nil {
		log.Error().Err(err).Str("event_id", eventID).Msg("Failed to update attention event")
		http.Error(w, "failed to update attention event", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// dismissAllAttentionEvents godoc
// @Summary Dismiss all active attention events
// @Description Bulk-dismiss all active (non-dismissed) attention events for the current user.
// @Tags    attention-events
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {string} string "unauthorized"
// @Failure 500 {string} string "internal server error"
// @Router  /api/v1/attention-events/dismiss-all [post]
func (s *HelixAPIServer) dismissAllAttentionEvents(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	organizationID := user.OrganizationID

	dismissed, err := s.Store.BulkDismissAttentionEvents(r.Context(), user.ID, organizationID)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to dismiss all attention events")
		http.Error(w, "failed to dismiss attention events", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"dismissed": dismissed,
	})
}
