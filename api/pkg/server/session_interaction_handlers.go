package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// listInteractions godoc
// @Summary List interactions for a session
// @Description List interactions for a session with pagination
// @Tags    interactions
// @Produce json
// @Param   id path string true "Session ID"
// @Param   page query int false "Page number (0-indexed)"
// @Param   per_page query int false "Page size (default 100)"
// @Param   order query string false "Sort order: 'asc' (oldest first, default) or 'desc' (newest first)"
// @Success 200 {object} types.PaginatedInteractions
// @Router /api/v1/sessions/{id}/interactions [get]
// @Security BearerAuth
func (s *HelixAPIServer) listInteractions(_ http.ResponseWriter, req *http.Request) (*types.PaginatedInteractions, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	page, err := strconv.Atoi(req.URL.Query().Get("page"))
	if err != nil || page < 0 {
		page = 0
	}
	perPage, err := strconv.Atoi(req.URL.Query().Get("per_page"))
	if err != nil || perPage < 1 {
		perPage = 100
	}

	// Support descending order (newest first) for pagination
	order := "id ASC"
	if req.URL.Query().Get("order") == "desc" {
		order = "id DESC"
	}

	session, err := s.Store.GetSession(ctx, id)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get session %s, error: %s", id, err))
	}

	err = s.authorizeUserToSession(ctx, user, session, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403("you are not allowed to access this session")
	}

	interactions, totalCount, err := s.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    id,
		GenerationID: session.GenerationID,
		Page:         page,
		PerPage:      perPage,
		Order:        order,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get interactions for session %s, error: %s", id, err))
	}

	// Cap response_entries to avoid sending multi-MB payloads.
	// - Strip redundant response_message when entries exist
	// - Keep only the last 50 entries
	// - Truncate individual entry content to 100 KB (a single 2.5 MB text
	//   entry will kill the browser's markdown renderer)
	const maxEntries = 50
	const maxEntryContentLen = 100_000
	for _, interaction := range interactions {
		if interaction.ResponseEntries != nil {
			interaction.ResponseMessage = ""

			var entries []map[string]interface{}
			if err := json.Unmarshal(interaction.ResponseEntries, &entries); err == nil {
				if len(entries) > maxEntries {
					entries = entries[len(entries)-maxEntries:]
				}
				for i, entry := range entries {
					if content, ok := entry["content"].(string); ok && len(content) > maxEntryContentLen {
						entries[i]["content"] = content[len(content)-maxEntryContentLen:]
					}
				}
				if truncatedJSON, err := json.Marshal(entries); err == nil {
					interaction.ResponseEntries = truncatedJSON
				}
			}
		}
	}

	totalPages := int(totalCount) / perPage
	if int(totalCount)%perPage > 0 {
		totalPages++
	}

	return &types.PaginatedInteractions{
		Interactions: interactions,
		Page:         page,
		PageSize:     perPage,
		TotalCount:   totalCount,
		TotalPages:   totalPages,
	}, nil
}

// getInteraction godoc
// @Summary Get an interaction by ID
// @Description Get an interaction by ID
// @Tags    interactions
// @Produce json
// @Param   id path string true "Session ID"
// @Param   interaction_id path string true "Interaction ID"
// @Success 200 {object} types.Interaction
// @Router /api/v1/sessions/{id}/interactions/{interaction_id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getInteraction(_ http.ResponseWriter, req *http.Request) (*types.Interaction, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	sessionID := mux.Vars(req)["id"]
	interactionID := mux.Vars(req)["interaction_id"]

	// First load the session
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get session %s, error: %s", sessionID, err))
	}

	err = s.authorizeUserToSession(ctx, user, session, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403("you are not allowed to access this session")
	}

	interaction, err := s.Store.GetInteraction(ctx, interactionID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get interaction %s, error: %s", interactionID, err))
	}

	if interaction.SessionID != sessionID {
		return nil, system.NewHTTPError403("you are not allowed to access this interaction")
	}

	return interaction, nil
}

// feedbackInteraction godoc
// @Summary Provide feedback for an interaction
// @Description Provide feedback for an interaction
// @Tags    interactions
// @Produce json
// @Param   id path string true "Session ID"
// @Param   interaction_id path string true "Interaction ID"
// @Param   feedback body types.FeedbackRequest true "Feedback"
// @Success 200 {object} types.Interaction
// @Router /api/v1/sessions/{id}/interactions/{interaction_id}/feedback [post]
// @Security BearerAuth
func (s *HelixAPIServer) feedbackInteraction(_ http.ResponseWriter, req *http.Request) (*types.Interaction, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	sessionID := mux.Vars(req)["id"]
	interactionID := mux.Vars(req)["interaction_id"]

	// First load the session
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get session %s, error: %s", sessionID, err))
	}

	interaction, err := s.Store.GetInteraction(ctx, interactionID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get interaction %s, error: %s", interactionID, err))
	}

	if interaction.SessionID != sessionID {
		return nil, system.NewHTTPError403("you are not allowed to access this interaction")
	}

	err = s.authorizeUserToSession(ctx, user, session, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403("you are not allowed to access this session")
	}

	var r types.FeedbackRequest
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("failed to decode feedback request, error: %s", err))
	}

	if r.Feedback == "" {
		return nil, system.NewHTTPError400("feedback is required")
	}

	interaction.Feedback = r.Feedback
	interaction.FeedbackMessage = r.FeedbackMessage

	if _, err := s.Store.UpdateInteraction(ctx, interaction); err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to update interaction %s, error: %s", interactionID, err))
	}

	return interaction, nil
}
