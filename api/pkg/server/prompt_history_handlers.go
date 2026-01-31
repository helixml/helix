package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
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

	// Process pending prompts in the background
	// This runs on EVERY sync to catch prompts that may have been missed
	go apiServer.processPendingPromptsForIdleSessions(context.Background(), syncReq.SpecTaskID)

	return response, nil
}

// processPendingPromptsForIdleSessions checks the database for any pending prompts
// (both interrupt and queue) and processes them if the session is idle
// This handles prompts that may have been synced but never processed
func (apiServer *HelixAPIServer) processPendingPromptsForIdleSessions(ctx context.Context, specTaskID string) {
	if specTaskID == "" {
		return
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Msg("🔍 [QUEUE] Processing pending prompts for spec task")

	// Query the database for ALL pending prompts for this spec task
	entries, err := apiServer.Store.ListPromptHistoryBySpecTask(ctx, specTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to list prompt history for queue processing")
		return
	}

	// Collect pending prompts per session
	type sessionPending struct {
		interruptCount int
		queueCount     int
	}
	sessionPrompts := make(map[string]*sessionPending)

	for _, entry := range entries {
		if entry.Status != "pending" && entry.Status != "failed" {
			continue
		}
		if entry.SessionID == "" {
			continue
		}

		if sessionPrompts[entry.SessionID] == nil {
			sessionPrompts[entry.SessionID] = &sessionPending{}
		}

		if entry.Interrupt {
			sessionPrompts[entry.SessionID].interruptCount++
		} else {
			sessionPrompts[entry.SessionID].queueCount++
		}
	}

	if len(sessionPrompts) == 0 {
		return
	}

	log.Debug().
		Str("spec_task_id", specTaskID).
		Int("session_count", len(sessionPrompts)).
		Msg("🔍 [QUEUE] Found sessions with pending prompts")

	// Process pending prompts for idle sessions
	for sessionID, pending := range sessionPrompts {
		session, err := apiServer.Store.GetSession(ctx, sessionID)
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for queue processing")
			continue
		}

		if session == nil {
			continue
		}

		// Load interactions for this session (GetSession doesn't load them)
		interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
			SessionID:    sessionID,
			GenerationID: session.GenerationID,
			PerPage:      100,
		})
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to list interactions for queue processing")
			continue
		}

		// Check if session is idle (no interactions, or last interaction is complete)
		isIdle := true
		if len(interactions) > 0 {
			lastInteraction := interactions[len(interactions)-1]
			if lastInteraction.State == types.InteractionStateWaiting {
				isIdle = false
			}
		}

		if isIdle {
			log.Info().
				Str("session_id", sessionID).
				Int("interrupt_count", pending.interruptCount).
				Int("queue_count", pending.queueCount).
				Msg("📤 [QUEUE] Session is idle, processing pending prompts")
			// When session is idle from sync/list, process ALL pending (interrupt first)
			apiServer.processAnyPendingPrompt(ctx, sessionID)
		} else if pending.interruptCount > 0 {
			// Session is busy but there are interrupt prompts - these should interrupt the agent
			log.Info().
				Str("session_id", sessionID).
				Int("interrupt_count", pending.interruptCount).
				Msg("📤 [QUEUE] Session is busy but has interrupt prompts, sending interrupt")
			apiServer.processInterruptPrompt(ctx, sessionID)
		} else {
			log.Debug().
				Str("session_id", sessionID).
				Int("queue_count", pending.queueCount).
				Msg("Session is busy (interaction waiting), queue prompts will be processed after message_completed")
		}
	}
}

// processInterruptPrompt processes ONLY interrupt prompts (interrupt=true)
// Used when the session is busy but user sent an interrupt message
func (apiServer *HelixAPIServer) processInterruptPrompt(ctx context.Context, sessionID string) {
	// Get the next interrupt prompt for this session
	nextPrompt, err := apiServer.Store.GetNextInterruptPrompt(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get next interrupt prompt")
		return
	}

	if nextPrompt == nil {
		log.Debug().Str("session_id", sessionID).Msg("No pending interrupt prompts")
		return
	}

	isRetry := nextPrompt.Status == "failed"
	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Str("content_preview", truncateString(nextPrompt.Content, 50)).
		Bool("is_retry", isRetry).
		Msg("📤 [INTERRUPT] Processing interrupt prompt")

	// CRITICAL: Mark as 'sent' IMMEDIATELY to prevent race conditions.
	// Once we start processing, mark it done so no other process picks it up.
	if err := apiServer.Store.MarkPromptAsSent(ctx, nextPrompt.ID); err != nil {
		log.Error().Err(err).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as sent")
		// Continue anyway - better to risk duplicate than lose the message
	}

	// Send the prompt to the session (creates interaction and sends to agent)
	if err := apiServer.sendQueuedPromptToSession(ctx, sessionID, nextPrompt); err != nil {
		// Interaction creation failed - revert to 'failed' so it can be retried
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Str("prompt_id", nextPrompt.ID).
			Msg("Failed to create interaction for interrupt prompt - reverting to failed")
		if markErr := apiServer.Store.MarkPromptAsFailed(ctx, nextPrompt.ID); markErr != nil {
			log.Error().Err(markErr).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as failed after interaction creation error")
		}
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Msg("✅ [INTERRUPT] Successfully sent interrupt prompt to session")
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

	// Process pending prompts in the background
	// This runs on EVERY list call (which the frontend polls every 2s when there are pending messages)
	// to catch prompts that may have been synced but never processed
	go apiServer.processPendingPromptsForIdleSessions(context.Background(), specTaskID)

	return response, nil
}

// PromptPinRequest is the request body for pinning/unpinning a prompt
type PromptPinRequest struct {
	Pinned bool `json:"pinned"`
}

// PromptTagsRequest is the request body for updating prompt tags
type PromptTagsRequest struct {
	Tags string `json:"tags"` // JSON array of tags
}

// @Summary Update prompt pin status
// @Description Pin or unpin a prompt for quick access
// @Tags PromptHistory
// @Accept json
// @Produce json
// @Param id path string true "Prompt ID"
// @Param request body PromptPinRequest true "Pin status"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/prompt-history/{id}/pin [put]
func (apiServer *HelixAPIServer) updatePromptPin(_ http.ResponseWriter, req *http.Request) (map[string]bool, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	promptID := mux.Vars(req)["id"]
	if promptID == "" {
		return nil, system.NewHTTPError400("prompt id is required")
	}

	var pinReq PromptPinRequest
	if err := json.NewDecoder(req.Body).Decode(&pinReq); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Verify user owns this prompt
	prompt, err := apiServer.Store.GetPromptHistoryEntry(ctx, promptID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get prompt: %v", err))
	}
	if prompt == nil {
		return nil, system.NewHTTPError404("prompt not found")
	}
	if prompt.UserID != user.ID {
		return nil, system.NewHTTPError403("you don't have permission to modify this prompt")
	}

	if err := apiServer.Store.UpdatePromptPin(ctx, promptID, pinReq.Pinned); err != nil {
		log.Error().Err(err).Str("prompt_id", promptID).Msg("Failed to update prompt pin status")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to update pin status: %v", err))
	}

	return map[string]bool{"pinned": pinReq.Pinned}, nil
}

// @Summary Update prompt tags
// @Description Update tags for a prompt
// @Tags PromptHistory
// @Accept json
// @Produce json
// @Param id path string true "Prompt ID"
// @Param request body PromptTagsRequest true "Tags (JSON array)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/prompt-history/{id}/tags [put]
func (apiServer *HelixAPIServer) updatePromptTags(_ http.ResponseWriter, req *http.Request) (map[string]string, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	promptID := mux.Vars(req)["id"]
	if promptID == "" {
		return nil, system.NewHTTPError400("prompt id is required")
	}

	var tagsReq PromptTagsRequest
	if err := json.NewDecoder(req.Body).Decode(&tagsReq); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Verify user owns this prompt
	prompt, err := apiServer.Store.GetPromptHistoryEntry(ctx, promptID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get prompt: %v", err))
	}
	if prompt == nil {
		return nil, system.NewHTTPError404("prompt not found")
	}
	if prompt.UserID != user.ID {
		return nil, system.NewHTTPError403("you don't have permission to modify this prompt")
	}

	if err := apiServer.Store.UpdatePromptTags(ctx, promptID, tagsReq.Tags); err != nil {
		log.Error().Err(err).Str("prompt_id", promptID).Msg("Failed to update prompt tags")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to update tags: %v", err))
	}

	return map[string]string{"tags": tagsReq.Tags}, nil
}

// @Summary List pinned prompts
// @Description Get all pinned prompts for the current user
// @Tags PromptHistory
// @Accept json
// @Produce json
// @Param spec_task_id query string false "Filter by spec task ID"
// @Success 200 {array} types.PromptHistoryEntry
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/prompt-history/pinned [get]
func (apiServer *HelixAPIServer) listPinnedPrompts(_ http.ResponseWriter, req *http.Request) ([]*types.PromptHistoryEntry, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	specTaskID := req.URL.Query().Get("spec_task_id")

	entries, err := apiServer.Store.ListPinnedPrompts(ctx, user.ID, specTaskID)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to list pinned prompts")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list pinned prompts: %v", err))
	}

	return entries, nil
}

// @Summary Search prompts
// @Description Search prompts by content
// @Tags PromptHistory
// @Accept json
// @Produce json
// @Param q query string true "Search query"
// @Param limit query int false "Max results (default 50)"
// @Success 200 {array} types.PromptHistoryEntry
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/prompt-history/search [get]
func (apiServer *HelixAPIServer) searchPrompts(_ http.ResponseWriter, req *http.Request) ([]*types.PromptHistoryEntry, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	query := req.URL.Query()
	searchQuery := query.Get("q")
	if searchQuery == "" {
		return nil, system.NewHTTPError400("search query 'q' is required")
	}

	limit := 50
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	entries, err := apiServer.Store.SearchPrompts(ctx, user.ID, searchQuery, limit)
	if err != nil {
		log.Error().Err(err).
			Str("user_id", user.ID).
			Str("query", searchQuery).
			Msg("Failed to search prompts")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to search prompts: %v", err))
	}

	return entries, nil
}

// @Summary Increment prompt usage
// @Description Increment usage count when a prompt is reused
// @Tags PromptHistory
// @Accept json
// @Produce json
// @Param id path string true "Prompt ID"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/prompt-history/{id}/use [post]
func (apiServer *HelixAPIServer) incrementPromptUsage(_ http.ResponseWriter, req *http.Request) (map[string]bool, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	promptID := mux.Vars(req)["id"]
	if promptID == "" {
		return nil, system.NewHTTPError400("prompt id is required")
	}

	// Verify user owns this prompt
	prompt, err := apiServer.Store.GetPromptHistoryEntry(ctx, promptID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get prompt: %v", err))
	}
	if prompt == nil {
		return nil, system.NewHTTPError404("prompt not found")
	}
	if prompt.UserID != user.ID {
		return nil, system.NewHTTPError403("you don't have permission to modify this prompt")
	}

	if err := apiServer.Store.IncrementPromptUsage(ctx, promptID); err != nil {
		log.Error().Err(err).Str("prompt_id", promptID).Msg("Failed to increment prompt usage")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to increment usage: %v", err))
	}

	return map[string]bool{"success": true}, nil
}

// @Summary Unified search across Helix entities
// @Description Search across projects, tasks, sessions, prompts, and code
// @Tags Search
// @Accept json
// @Produce json
// @Param q query string true "Search query"
// @Param types query []string false "Entity types to search: projects, tasks, sessions, prompts, code"
// @Param limit query int false "Max results per type (default 10)"
// @Param org_id query string false "Filter by organization ID"
// @Success 200 {object} types.UnifiedSearchResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/search [get]
func (apiServer *HelixAPIServer) unifiedSearch(_ http.ResponseWriter, req *http.Request) (*types.UnifiedSearchResponse, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	query := req.URL.Query()
	searchQuery := query.Get("q")
	if searchQuery == "" {
		return nil, system.NewHTTPError400("search query 'q' is required")
	}

	searchReq := &types.UnifiedSearchRequest{
		Query: searchQuery,
		Types: query["types"],
		OrgID: query.Get("org_id"),
	}

	// Parse limit
	limit := 10
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			searchReq.Limit = l
		}
	}

	// Check if code search is requested
	searchCode := false
	if len(searchReq.Types) == 0 {
		// Default includes code
		searchCode = true
	} else {
		for _, t := range searchReq.Types {
			if t == "code" {
				searchCode = true
				break
			}
		}
	}

	response, err := apiServer.Store.UnifiedSearch(ctx, user.ID, searchReq)
	if err != nil {
		log.Error().Err(err).
			Str("user_id", user.ID).
			Str("query", searchQuery).
			Msg("Failed to perform unified search")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to search: %v", err))
	}

	// Add Kodit code search results if requested and service is available
	if searchCode && apiServer.koditService != nil && apiServer.koditService.IsEnabled() {
		codeResults := apiServer.searchCodeAcrossRepositories(ctx, user.ID, searchQuery, limit)
		response.Results = append(response.Results, codeResults...)
		response.Total = len(response.Results)
	}

	return response, nil
}

// searchCodeAcrossRepositories searches code snippets across all user's Kodit-enabled repositories
func (apiServer *HelixAPIServer) searchCodeAcrossRepositories(ctx context.Context, userID, query string, limit int) []types.UnifiedSearchResult {
	results := make([]types.UnifiedSearchResult, 0)

	// Get all repositories the user owns with Kodit indexing enabled
	repos, err := apiServer.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		OwnerID: userID,
	})
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Failed to list repositories for code search")
		return results
	}

	// Search each Kodit-enabled repository
	for _, repo := range repos {
		if !repo.KoditIndexing {
			continue
		}

		// Get kodit_repo_id from metadata
		var koditRepoID string
		if repo.Metadata != nil {
			if id, ok := repo.Metadata["kodit_repo_id"].(string); ok {
				koditRepoID = id
			}
		}
		if koditRepoID == "" {
			continue
		}

		// Search snippets in this repository
		snippets, err := apiServer.koditService.SearchSnippets(ctx, koditRepoID, query, limit, "")
		if err != nil {
			log.Debug().Err(err).
				Str("repo_id", repo.ID).
				Str("kodit_repo_id", koditRepoID).
				Msg("Failed to search snippets in repository")
			continue
		}

		// Convert snippets to unified search results
		for _, snippet := range snippets {
			title := snippet.FilePath
			if title == "" {
				title = truncateString(snippet.Content, 60)
			}

			results = append(results, types.UnifiedSearchResult{
				Type:        "code",
				ID:          snippet.ID,
				Title:       title,
				Description: truncateString(snippet.Content, 150),
				URL:         "/repositories/" + repo.ID,
				Icon:        "code",
				Metadata: map[string]string{
					"repoId":   repo.ID,
					"repoName": repo.Name,
					"filePath": snippet.FilePath,
					"language": snippet.Language,
				},
			})
		}

		// Limit total code results
		if len(results) >= limit {
			results = results[:limit]
			break
		}
	}

	return results
}
