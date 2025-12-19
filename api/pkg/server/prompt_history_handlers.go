package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// @Summary Sync prompt history
// @Description Sync prompt history entries from the frontend (union merge - no deletes)
// @Tags PromptHistory
// @Accept json
// @Produce json
// @Param request body types.PromptHistorySyncRequest true "Prompt history entries to sync"
// @Success 200 {object} types.PromptHistorySyncResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/prompt-history/sync [post]
func (apiServer *HelixAPIServer) syncPromptHistory(_ http.ResponseWriter, req *http.Request) (*types.PromptHistorySyncResponse, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	var syncReq types.PromptHistorySyncRequest
	if err := json.NewDecoder(req.Body).Decode(&syncReq); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Validate required fields
	if syncReq.SpecTaskID == "" {
		return nil, system.NewHTTPError400("spec_task_id is required")
	}

	response, err := apiServer.Store.SyncPromptHistory(ctx, user.ID, &syncReq)
	if err != nil {
		log.Error().Err(err).
			Str("user_id", user.ID).
			Str("spec_task_id", syncReq.SpecTaskID).
			Msg("Failed to sync prompt history")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to sync prompt history: %v", err))
	}

	log.Info().
		Str("user_id", user.ID).
		Str("spec_task_id", syncReq.SpecTaskID).
		Int("synced", response.Synced).
		Int("existing", response.Existing).
		Int("total_entries", len(response.Entries)).
		Msg("Synced prompt history")

	return response, nil
}

// @Summary List prompt history
// @Description Get prompt history entries for the current user
// @Tags PromptHistory
// @Accept json
// @Produce json
// @Param spec_task_id query string true "Spec Task ID (required)"
// @Param project_id query string false "Project ID (optional filter)"
// @Param session_id query string false "Session ID (optional filter)"
// @Param since query int false "Only entries after this timestamp (Unix milliseconds)"
// @Param limit query int false "Max entries to return (default 100)"
// @Success 200 {object} types.PromptHistoryListResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/prompt-history [get]
func (apiServer *HelixAPIServer) listPromptHistory(_ http.ResponseWriter, req *http.Request) (*types.PromptHistoryListResponse, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	// Parse query parameters
	query := req.URL.Query()
	specTaskID := query.Get("spec_task_id")
	if specTaskID == "" {
		return nil, system.NewHTTPError400("spec_task_id is required")
	}

	listReq := &types.PromptHistoryListRequest{
		SpecTaskID: specTaskID,
		ProjectID:  query.Get("project_id"),
		SessionID:  query.Get("session_id"),
	}

	// Parse since (Unix ms)
	if sinceStr := query.Get("since"); sinceStr != "" {
		since, err := strconv.ParseInt(sinceStr, 10, 64)
		if err == nil {
			listReq.Since = since
		}
	}

	// Parse limit
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err == nil {
			listReq.Limit = limit
		}
	}

	response, err := apiServer.Store.ListPromptHistory(ctx, user.ID, listReq)
	if err != nil {
		log.Error().Err(err).
			Str("user_id", user.ID).
			Str("spec_task_id", specTaskID).
			Msg("Failed to list prompt history")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list prompt history: %v", err))
	}

	return response, nil
}
