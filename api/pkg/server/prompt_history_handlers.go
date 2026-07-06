package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// desktopResumeReapStaleThreshold is how long a state=waiting interaction with
// no live WebSocket must have been idle before the queue-side liveness guard
// treats it as an orphan (agent gone) and reaps it so the desktop can resume.
// Sized above the auto-wake stuck threshold (180s) so a genuinely in-flight or
// mid-boot turn is never mistaken for a dead one.
const desktopResumeReapStaleThreshold = 3 * time.Minute

// isOrphanedWaitingInteraction reports whether `latest` (the newest interaction
// of `session`) is a state=waiting turn whose external agent has gone away and
// will therefore never complete — the case that deadlocks the prompt-queue
// busy-check and stops the desktop from resuming. Pure so it can be unit-tested
// exhaustively against every branch of the boot/live/orphan matrix.
//
// All must hold to classify as orphaned (any failing → treat as busy, defer):
//   - latest is state=waiting (nothing else can be actively streaming);
//   - no live WebSocket to the agent (wsLive=false) — a live turn keeps one;
//   - the thread is already established (ZedThreadID set) — an empty ZedThreadID
//     is the very-first-message boot race, which must never be reaped (mirrors
//     the THREAD-ESTABLISHMENT BARRIER in processPendingPromptsForIdleSessions);
//   - the turn has been idle past desktopResumeReapStaleThreshold — a freshly
//     created / mid-boot in-flight turn is protected by the staleness window.
func isOrphanedWaitingInteraction(session *types.Session, latest *types.Interaction, wsLive bool, now time.Time) bool {
	if session == nil || latest == nil {
		return false
	}
	if latest.State != types.InteractionStateWaiting {
		return false
	}
	if wsLive {
		return false
	}
	if session.Metadata.ZedThreadID == "" {
		return false
	}
	return now.Sub(latest.Updated) > desktopResumeReapStaleThreshold
}

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

	// Synchronously mark the canonical session as "starting" if it's idle and
	// has no live WebSocket. This closes the race that caused the
	// "Starting Desktop..." spinner to flicker off: the frontend's first
	// refetch after the optimistic cache write would otherwise overwrite
	// "starting" with the still-stale "stopped" backend row, because the
	// wake goroutine below has not yet had time to call StartDesktop and
	// have hydra write status=starting to the DB. See spec
	// design/tasks/002047_yet-again-sending-a/design.md.
	apiServer.markCanonicalSessionStartingForSync(ctx, syncReq.SpecTaskID)

	// Process pending prompts in the background
	// This runs on EVERY sync to catch prompts that may have been missed
	go apiServer.processPendingPromptsForIdleSessions(context.Background(), syncReq.SpecTaskID)

	return response, nil
}

// markCanonicalSessionStartingForSync flips the canonical planning session's
// external_agent_status to "starting" when its desktop is genuinely idle (no
// live WebSocket connection to the external agent). No-op when the session is
// already "starting" / "running", or when a WS is alive (the existing socket
// will deliver the prompt without any boot). Failures are logged but never
// surfaced to the caller — the synchronous mark is a UX optimisation, not a
// correctness requirement; the async wake goroutine that fires next still
// works as before.
func (apiServer *HelixAPIServer) markCanonicalSessionStartingForSync(ctx context.Context, specTaskID string) {
	if specTaskID == "" {
		return
	}
	specTask, err := apiServer.Store.GetSpecTask(ctx, specTaskID)
	if err != nil || specTask == nil {
		log.Debug().Err(err).Str("spec_task_id", specTaskID).Msg("[PROMPT-SYNC] cannot resolve spec task for sync-time mark; skipping")
		return
	}
	sessionID := specTask.PlanningSessionID
	if sessionID == "" {
		return
	}
	if conn, connected := apiServer.externalAgentWSManager.getConnection(sessionID); connected && conn != nil {
		return
	}
	updated, err := apiServer.Store.MarkSessionStartingIfIdle(ctx, sessionID)
	if err != nil {
		log.Warn().Err(err).
			Str("spec_task_id", specTaskID).
			Str("session_id", sessionID).
			Msg("[PROMPT-SYNC] failed to mark session starting; spinner may flicker")
		return
	}
	if updated {
		log.Info().
			Str("spec_task_id", specTaskID).
			Str("session_id", sessionID).
			Msg("[PROMPT-SYNC] marked idle session as starting (no live WS)")
	} else {
		log.Debug().
			Str("spec_task_id", specTaskID).
			Str("session_id", sessionID).
			Msg("[PROMPT-SYNC] session already starting/running; no mark needed")
	}
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

	// Determine the canonical planning session for this spec task.
	// We only deliver prompts to the session that is the authoritative planning session
	// (task.PlanningSessionID). If a duplicate orphan session was created by a race
	// condition (issue #10), we must not deliver prompts to it — that causes duplicate
	// message sends (issue #2) and confuses the agent (issue #9).
	var canonicalSessionID string
	if specTask, taskErr := apiServer.Store.GetSpecTask(ctx, specTaskID); taskErr == nil && specTask != nil {
		canonicalSessionID = specTask.PlanningSessionID
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
		// Skip sessions that are not the canonical planning session (issue #10b).
		// If we couldn't determine the canonical session, fall through to the old behaviour.
		if canonicalSessionID != "" && entry.SessionID != canonicalSessionID {
			log.Debug().
				Str("spec_task_id", specTaskID).
				Str("canonical_session_id", canonicalSessionID).
				Str("skipped_session_id", entry.SessionID).
				Msg("🔍 [QUEUE] Skipping prompt for non-canonical session (duplicate race session)")
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

		// Load the MOST RECENT interaction so we can check if the session is busy.
		// CRITICAL: ListInteractions defaults to "id ASC" (oldest first). With
		// PerPage=100 and 165 interactions, we'd get interactions 1-100 and
		// interactions[len-1] would be the 100th — almost always Complete from
		// hours ago — so the busy check would always say "idle" and the queue
		// would dispatch on top of an actively-streaming Zed turn.
		// Order DESC + PerPage 1 returns just the newest interaction.
		interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
			SessionID:    sessionID,
			GenerationID: session.GenerationID,
			PerPage:      1,
			Order:        "id DESC",
		})
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to list interactions for queue processing")
			continue
		}

		// Session is idle iff there is no interaction, or the latest one is
		// not still Waiting for Zed. interactions[0] is the newest because of
		// the DESC order above.
		isIdle := true
		if len(interactions) > 0 && interactions[0].State == types.InteractionStateWaiting {
			isIdle = false
		}

		if isIdle {
			if pending.interruptCount > 0 {
				log.Info().
					Str("session_id", sessionID).
					Int("interrupt_count", pending.interruptCount).
					Msg("📤 [QUEUE] Session is idle with interrupt prompts, sending interrupt")
				apiServer.processInterruptPrompt(ctx, sessionID)
			} else {
				// Queue-mode messages when idle: use processPromptQueue for consistent semantics.
				// This ensures queue-mode messages always go through the same code path as
				// post-message_completed dispatch (Bug 2 fix).
				log.Info().
					Str("session_id", sessionID).
					Int("queue_count", pending.queueCount).
					Msg("📤 [QUEUE] Session is idle with queue-mode prompts, dispatching via processPromptQueue")
				apiServer.processPromptQueue(ctx, sessionID)
			}
		} else if pending.interruptCount > 0 {
			// THREAD-ESTABLISHMENT BARRIER: an interrupt cancels the agent's
			// current turn and injects a new message — that only makes sense once
			// the session's thread exists. During the agent boot race the session
			// can be "busy" (the very first message is an in-flight Waiting
			// interaction) while its thread is NOT yet established (ZedThreadID
			// empty, thread_created not received). Firing the interrupt now would
			// (a) cancel the just-delivered first turn before the agent processes
			// it and (b) dispatch with an empty acp_thread_id, forking a NEW thread
			// divorced from the first message — so the agent runs with no context.
			// Defer until the first message lands and thread_created sets
			// ZedThreadID; the prompt stays pending and the next poll retries,
			// then the interrupt fires into the SAME, established thread.
			// See design/2026-06-19-incident-interrupt-during-boot-context-loss.md.
			if session.Metadata.ZedThreadID == "" {
				log.Info().
					Str("session_id", sessionID).
					Int("interrupt_count", pending.interruptCount).
					Msg("⏸️ [QUEUE] Busy but thread not established yet (no ZedThreadID) — deferring interrupt until first message lands and thread_created arrives")
			} else {
				// Session is busy but there are interrupt prompts - these should interrupt the agent
				log.Info().
					Str("session_id", sessionID).
					Int("interrupt_count", pending.interruptCount).
					Msg("📤 [QUEUE] Session is busy but has interrupt prompts, sending interrupt")
				apiServer.processInterruptPrompt(ctx, sessionID)
			}
		} else {
			// Liveness guard for the busy branch. A Waiting latest interaction
			// normally means an agent is actively streaming a turn, so we defer
			// the queue until message_completed. BUT if the agent has gone away
			// (desktop idle-stopped / crashed / restarted) that completion never
			// arrives: the queue waits forever AND the desktop never resumes —
			// the exact deadlock behind the "didn't boot when I sent a message"
			// incident. The idle-checker reaps at stop time for the idle path;
			// this net covers every OTHER way the desktop dies (OOM, crash,
			// stack restart) where nothing reaped the dangling interaction.
			//
			// Reap ONLY when all three hold, so we never kill a live or mid-boot
			// turn:
			//   - no live WebSocket to the external agent (a live turn has one);
			//   - the thread is already established (ZedThreadID set) — an empty
			//     ZedThreadID means the very first message is mid-boot, which we
			//     must not reap (mirrors the THREAD-ESTABLISHMENT BARRIER above);
			//   - the waiting interaction is stale beyond the reap threshold — a
			//     freshly-created in-flight turn is protected.
			latest := interactions[0]
			_, wsLive := apiServer.externalAgentWSManager.getConnection(sessionID)
			if isOrphanedWaitingInteraction(session, latest, wsLive, time.Now()) {
				reaped, reapErr := apiServer.Store.ReapWaitingInteractions(ctx, sessionID, types.InteractionStateInterrupted, "desktop stopped while turn in-flight; reaped so queue can resume")
				if reapErr != nil {
					log.Error().Err(reapErr).Str("session_id", sessionID).Msg("Failed to reap orphaned waiting interaction; queue still blocked")
				} else {
					log.Warn().
						Str("session_id", sessionID).
						Int("reaped_count", len(reaped)).
						Time("latest_waiting_updated", latest.Updated).
						Msg("♻️ [QUEUE] Reaped orphaned waiting interaction (no live agent) — dispatching queued prompt to resume desktop")
					for _, in := range reaped {
						apiServer.publishInteractionUpdateToFrontend(sessionID, session.Owner, in)
					}
					if pending.interruptCount > 0 {
						apiServer.processInterruptPrompt(ctx, sessionID)
					} else {
						apiServer.processPromptQueue(ctx, sessionID)
					}
				}
			} else {
				log.Debug().
					Str("session_id", sessionID).
					Int("queue_count", pending.queueCount).
					Msg("Session is busy (interaction waiting), queue prompts will be processed after message_completed")
			}
		}
	}
}

// processInterruptPrompt processes ONLY interrupt prompts (interrupt=true)
// Used when the session is busy but user sent an interrupt message
func (apiServer *HelixAPIServer) processInterruptPrompt(ctx context.Context, sessionID string) {
	// Serialise drains for this session — see lockPromptDrain. Held across
	// cancel → send so two rapid interrupts can't cancel + dispatch concurrently
	// and reorder; the second interrupt cancels the first's freshly-started turn.
	defer apiServer.lockPromptDrain(sessionID)()

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

	// GetNextInterruptPrompt already atomically claimed this prompt (set status='sending').
	// No additional ClaimPromptForSending call needed — that would fail because
	// the status is already 'sending', causing every interrupt to be silently dropped.

	// Cancel the current turn in Zed before sending the interrupt message.
	// Find the current waiting interaction's request_id and send cancel_current_turn.
	apiServer.cancelCurrentTurnIfActive(ctx, sessionID)

	// Send the prompt to the session (creates interaction and sends to agent)
	if err := apiServer.sendQueuedPromptToSession(ctx, sessionID, nextPrompt); err != nil {
		// Interaction creation failed - revert to 'failed' so it can be retried
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Str("prompt_id", nextPrompt.ID).
			Msg("Failed to create interaction for interrupt prompt - reverting to failed")
		if markErr := apiServer.Store.MarkPromptAsFailed(ctx, nextPrompt.ID, err.Error()); markErr != nil {
			log.Error().Err(markErr).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as failed after interaction creation error")
		}
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Msg("✅ [INTERRUPT] Successfully sent interrupt prompt to session")
}

// cancelCurrentTurnIfActive finds the current waiting interaction for a session
// and sends cancel_current_turn to Zed. It waits up to 3 seconds for acknowledgement.
func (apiServer *HelixAPIServer) cancelCurrentTurnIfActive(ctx context.Context, sessionID string) {
	// Find the current waiting interaction
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("[INTERRUPT] Failed to get session for cancel")
		return
	}

	// Find the request_id for the waiting interaction
	var activeRequestID string
	apiServer.contextMappingsMutex.RLock()
	for reqID, sessID := range apiServer.requestToSessionMapping {
		if sessID == sessionID {
			// Check if this request_id maps to a waiting interaction
			if interactionID, ok := apiServer.requestToInteractionMapping[reqID]; ok {
				interaction, err := apiServer.Store.GetInteraction(ctx, interactionID)
				if err == nil && interaction.State == types.InteractionStateWaiting {
					activeRequestID = reqID
					break
				}
			}
		}
	}
	apiServer.contextMappingsMutex.RUnlock()

	if activeRequestID == "" {
		log.Debug().Str("session_id", sessionID).Msg("[INTERRUPT] No active turn to cancel")
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("request_id", activeRequestID).
		Msg("[INTERRUPT] Cancelling active turn before sending interrupt")

	status, err := apiServer.sendCancelToExternalAgent(sessionID, activeRequestID, 3*time.Second)
	if err != nil {
		log.Warn().Err(err).
			Str("session_id", sessionID).
			Str("request_id", activeRequestID).
			Msg("[INTERRUPT] Cancel timed out or failed — proceeding with interrupt anyway")
	} else {
		log.Info().
			Str("session_id", sessionID).
			Str("request_id", activeRequestID).
			Str("status", status).
			Msg("[INTERRUPT] Turn cancelled successfully")
	}

	_ = session // used above for getting the session
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

// @Summary Delete a prompt history entry
// @Description Soft-deletes a prompt history entry so it is removed from the queue and no longer synced to clients
// @Tags PromptHistory
// @Produce json
// @Param id path string true "Prompt ID"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/prompt-history/{id} [delete]
func (apiServer *HelixAPIServer) deletePromptHistoryEntry(_ http.ResponseWriter, req *http.Request) (map[string]bool, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	promptID := mux.Vars(req)["id"]
	if promptID == "" {
		return nil, system.NewHTTPError400("prompt id is required")
	}

	prompt, err := apiServer.Store.GetPromptHistoryEntry(ctx, promptID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get prompt: %v", err))
	}
	if prompt == nil {
		return nil, system.NewHTTPError404("prompt not found")
	}
	if prompt.UserID != user.ID {
		return nil, system.NewHTTPError403("you don't have permission to delete this prompt")
	}

	if err := apiServer.Store.DeletePromptHistoryEntry(ctx, promptID); err != nil {
		log.Error().Err(err).Str("prompt_id", promptID).Msg("Failed to delete prompt history entry")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to delete prompt: %v", err))
	}

	log.Info().Str("prompt_id", promptID).Str("user_id", user.ID).Msg("Deleted prompt history entry")
	return map[string]bool{"deleted": true}, nil
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
		koditRepoID := extractKoditRepoID(repo.Metadata)
		if koditRepoID == 0 {
			continue
		}

		// Search snippets in this repository
		snippets, err := apiServer.koditService.SearchSnippets(ctx, koditRepoID, query, limit)
		if err != nil {
			log.Debug().Err(err).
				Str("repo_id", repo.ID).
				Int64("kodit_repo_id", koditRepoID).
				Msg("Failed to search snippets in repository")
			continue
		}

		// Convert snippets to unified search results
		for _, snippet := range snippets {
			results = append(results, types.UnifiedSearchResult{
				Type:        "code",
				ID:          strconv.FormatInt(snippet.ID(), 10),
				Title:       truncateString(snippet.Content(), 60),
				Description: truncateString(snippet.Content(), 150),
				URL:         "/repositories/" + repo.ID,
				Icon:        "code",
				Metadata: map[string]string{
					"repoId":   repo.ID,
					"repoName": repo.Name,
					"language": snippet.Language(),
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
