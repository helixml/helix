// Agent questions (ACP elicitations).
//
// An ACP agent can ask the user a question mid-turn — Claude Code does this via its
// AskUserQuestion tool, which claude-agent-acp turns into an ACP elicitation. The agent's
// turn blocks until the client answers. This file is the Helix half of that loop: it
// records the question so a human can see it, and reconciles its status with the agent.
//
// Two things here are easy to get wrong and are load-bearing:
//
//   - A WebSocket reconnect is NOT evidence that the agent is gone. agent_ready fires on
//     every reconnect and the commonest cause is the Helix API restarting, while the
//     desktop container, Zed and its respond_tx all survive. So nothing in this file
//     resolves a question because of a reconnect. Instead the agent re-affirms what it
//     still holds on a heartbeat, and questions are reaped only when those statements
//     stop (see reapStaleElicitations).
//
//   - A question keeps its interaction in state=waiting, legitimately. Machinery that
//     "recovers" stuck waiting interactions has to know the difference between a hung
//     agent and a turn waiting on a human.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/server/wsprotocol"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const (
	// defaultElicitationResyncGrace is how long a pending question survives without the
	// agent re-affirming that it still holds it. The agent's heartbeat is every 15s, so
	// this is four missed beats — long enough to ride out a slow thread reload, short
	// enough that a dead question stops being answerable promptly.
	//
	// Override with HELIX_ELICITATION_RESYNC_GRACE_SECONDS.
	defaultElicitationResyncGrace = 60 * time.Second

	// elicitationReapInterval is how often we look for questions whose agent has gone
	// silent.
	elicitationReapInterval = 20 * time.Second
)

func elicitationResyncGrace() time.Duration {
	if raw := os.Getenv("HELIX_ELICITATION_RESYNC_GRACE_SECONDS"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultElicitationResyncGrace
}

// sameElicitation reports whether two question payloads are equivalent for the purposes
// of frontend patching. Status and resolution are what change after the question is first
// asked; the schema and message never do.
func sameElicitation(a, b *wsprotocol.ElicitationEntry) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID &&
		a.Status == b.Status &&
		a.ResolutionReason == b.ResolutionReason &&
		string(a.Content) == string(b.Content)
}

// handleElicitationRequested records a question the agent asked, and notifies the user.
//
// Safe to call repeatedly for the same elicitation: the agent re-announces outstanding
// questions when the WebSocket reconnects, so a restarted API can rebuild its view. Only
// the first announcement notifies.
func (apiServer *HelixAPIServer) handleElicitationRequested(sessionID string, syncMsg *types.SyncMessage) error {
	acpThreadID, _ := syncMsg.Data["acp_thread_id"].(string)
	elicitationID, _ := syncMsg.Data["elicitation_id"].(string)
	if elicitationID == "" {
		return fmt.Errorf("missing elicitation_id")
	}

	requestID, _ := syncMsg.Data["request_id"].(string)
	entryIndex, _ := syncMsg.Data["entry_index"].(string)
	toolCallID, _ := syncMsg.Data["tool_call_id"].(string)
	mode, _ := syncMsg.Data["mode"].(string)
	message, _ := syncMsg.Data["message"].(string)
	status, _ := syncMsg.Data["status"].(string)
	if status == "" {
		status = types.ElicitationStatusPending
	}

	// The schema is passed through verbatim — the frontend renders every control from
	// it, so anything lost here is an option the user never gets offered.
	var schemaJSON []byte
	if raw, ok := syncMsg.Data["requested_schema"]; ok && raw != nil {
		encoded, err := json.Marshal(raw)
		if err != nil {
			log.Warn().Err(err).Str("elicitation_id", elicitationID).
				Msg("[ELICITATION] Failed to marshal requested schema")
		} else {
			schemaJSON = encoded
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	helixSessionID, targetInteraction, err := apiServer.resolveElicitationTarget(ctx, sessionID, acpThreadID, requestID)
	if err != nil {
		// Dropping a question is the bug this feature exists to fix, so this is loud.
		log.Error().Err(err).
			Str("acp_thread_id", acpThreadID).
			Str("elicitation_id", elicitationID).
			Str("request_id", requestID).
			Msg("[ELICITATION] Could not route question to an interaction — the user will not see it")
		return nil
	}

	elicitation := &types.AgentElicitation{
		ID:            elicitationID,
		SessionID:     helixSessionID,
		InteractionID: targetInteraction.ID,
		RequestID:     requestID,
		AcpThreadID:   acpThreadID,
		ToolCallID:    toolCallID,
		EntryIndex:    entryIndex,
		Message:       message,
		Mode:          mode,
		Schema:        schemaJSON,
		Status:        status,
	}

	isNew, err := apiServer.Store.UpsertAgentElicitation(ctx, elicitation)
	if err != nil {
		return fmt.Errorf("failed to persist elicitation: %w", err)
	}

	// Load back so the entry mirrors the authoritative row rather than the wire event —
	// a re-announcement of an already-answered question must not resurrect it.
	stored, err := apiServer.Store.GetAgentElicitation(ctx, elicitationID)
	if err != nil {
		return fmt.Errorf("failed to reload elicitation: %w", err)
	}

	apiServer.writeElicitationEntry(ctx, helixSessionID, targetInteraction, stored)

	if isNew {
		log.Info().
			Str("session_id", helixSessionID).
			Str("interaction_id", targetInteraction.ID).
			Str("elicitation_id", elicitationID).
			Msg("❓ [ELICITATION] Agent asked the user a question")
		apiServer.notifyAgentQuestion(helixSessionID, elicitation)
	}
	return nil
}

// resolveElicitationTarget finds the session and interaction a question belongs to.
//
// Uses the same resolution as handleMessageAdded rather than a bespoke rule: the
// streaming context if the request_id maps to one, else the session's newest waiting
// interaction. An elicitation always happens mid-turn, so a missing request_id means
// something upstream lost the correlation, not that there is no turn.
func (apiServer *HelixAPIServer) resolveElicitationTarget(
	ctx context.Context,
	agentSessionID, acpThreadID, requestID string,
) (string, *types.Interaction, error) {
	apiServer.contextMappingsMutex.RLock()
	helixSessionID, exists := apiServer.contextMappings[acpThreadID]
	apiServer.contextMappingsMutex.RUnlock()

	if !exists {
		foundSession, err := apiServer.findSessionByZedThreadID(ctx, acpThreadID)
		if err != nil || foundSession == nil {
			return "", nil, fmt.Errorf("no Helix session for thread %s", acpThreadID)
		}
		helixSessionID = foundSession.ID
		apiServer.contextMappingsMutex.Lock()
		apiServer.contextMappings[acpThreadID] = helixSessionID
		apiServer.contextMappingsMutex.Unlock()
	}

	if sctx := apiServer.getOrCreateStreamingContext(ctx, helixSessionID, requestID); sctx != nil {
		sctx.mu.Lock()
		interaction := sctx.interaction
		sctx.mu.Unlock()
		if interaction != nil {
			return helixSessionID, interaction, nil
		}
	}

	// Fallback: the newest waiting interaction on the session. This is the same
	// last-resort path handleMessageAdded uses when the context mapping misses.
	interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: helixSessionID,
		PerPage:   1,
		Order:     "id DESC",
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to list interactions: %w", err)
	}
	if len(interactions) == 0 {
		return "", nil, fmt.Errorf("session %s has no interactions", helixSessionID)
	}
	return helixSessionID, interactions[0], nil
}

// writeElicitationEntry mirrors the question into the interaction's transcript so it
// renders in conversation order and stays there once answered, then pushes the change to
// any connected client.
//
// The row remains authoritative for status; this entry is written by the same handlers
// that write the row, so the two cannot drift.
func (apiServer *HelixAPIServer) writeElicitationEntry(
	ctx context.Context,
	helixSessionID string,
	interaction *types.Interaction,
	elicitation *types.AgentElicitation,
) {
	sctx := apiServer.getOrCreateStreamingContext(ctx, helixSessionID, elicitation.RequestID)
	if sctx == nil {
		log.Warn().Str("session_id", helixSessionID).
			Msg("[ELICITATION] No streaming context; question not mirrored into transcript")
		return
	}

	sctx.mu.Lock()
	defer sctx.mu.Unlock()

	if sctx.accumulator == nil {
		sctx.accumulator = &wsprotocol.MessageAccumulator{}
	}

	entry := &wsprotocol.ElicitationEntry{
		ID:               elicitation.ID,
		ToolCallID:       elicitation.ToolCallID,
		Message:          elicitation.Message,
		Mode:             elicitation.Mode,
		Schema:           json.RawMessage(elicitation.Schema),
		Status:           elicitation.Status,
		Content:          json.RawMessage(elicitation.Content),
		ResolutionReason: elicitation.ResolutionReason,
	}

	messageID := elicitation.EntryIndex
	if messageID == "" {
		// Without Zed's entry index there is no ordering anchor; fall back to the
		// elicitation id so the question still appears rather than being dropped.
		messageID = "elicitation-" + elicitation.ID
	}
	sctx.accumulator.UpsertElicitation(messageID, entry)

	currentEntries := sctx.accumulator.Entries()
	owner := ""
	if sctx.session != nil {
		owner = sctx.session.Owner
	}
	if err := apiServer.publishEntryPatchesToFrontend(
		helixSessionID, owner, interaction.ID,
		sctx.previousEntries, currentEntries, sctx.commenterID,
	); err != nil {
		log.Warn().Err(err).Msg("[ELICITATION] Failed to publish question to frontend")
	}
	sctx.previousEntries = currentEntries

	// Persist immediately rather than waiting for the throttled write: a question that
	// only exists in memory disappears on an API restart, which is exactly when the user
	// is most likely to reload the page looking for it.
	entriesJSON, err := json.Marshal(currentEntries)
	if err != nil {
		log.Warn().Err(err).Msg("[ELICITATION] Failed to marshal entries")
		return
	}
	interaction.ResponseEntries = entriesJSON
	if _, err := apiServer.Store.UpdateInteraction(ctx, interaction); err != nil {
		log.Warn().Err(err).Str("interaction_id", interaction.ID).
			Msg("[ELICITATION] Failed to persist question to interaction")
	}
	sctx.interaction = interaction
}

// handleElicitationResolved records that a question stopped being answerable, whatever
// resolved it — an answer, a skip, turn teardown, or a follow-up prompt.
func (apiServer *HelixAPIServer) handleElicitationResolved(sessionID string, syncMsg *types.SyncMessage) error {
	elicitationID, _ := syncMsg.Data["elicitation_id"].(string)
	if elicitationID == "" {
		return fmt.Errorf("missing elicitation_id")
	}
	status, _ := syncMsg.Data["status"].(string)
	if status == "" || status == types.ElicitationStatusPending {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var contentJSON []byte
	if raw, ok := syncMsg.Data["content"]; ok && raw != nil {
		if encoded, err := json.Marshal(raw); err == nil {
			contentJSON = encoded
		}
	}

	return apiServer.finalizeElicitation(ctx, elicitationID, status, reasonForStatus(status), contentJSON)
}

// reasonForStatus maps a terminal status to the reason shown in the UI. A cancel that
// arrives without a more specific reason is the turn ending or being interrupted.
func reasonForStatus(status string) string {
	switch status {
	case types.ElicitationStatusAccepted:
		return types.ElicitationReasonAnswered
	case types.ElicitationStatusDeclined:
		return types.ElicitationReasonSkipped
	case types.ElicitationStatusCancelled:
		return types.ElicitationReasonInterrupted
	default:
		return ""
	}
}

// finalizeElicitation applies a terminal status, mirrors it into the transcript and
// clears the notification. The status transition is conditional, so a late or duplicate
// event is a no-op rather than a resurrection.
func (apiServer *HelixAPIServer) finalizeElicitation(
	ctx context.Context,
	elicitationID, status, reason string,
	content []byte,
) error {
	transitioned, err := apiServer.Store.TransitionAgentElicitation(
		ctx, elicitationID,
		[]string{types.ElicitationStatusPending, types.ElicitationStatusSubmitting},
		status, reason, content,
	)
	if err != nil {
		return fmt.Errorf("failed to transition elicitation: %w", err)
	}
	if !transitioned {
		log.Debug().
			Str("elicitation_id", elicitationID).
			Str("status", status).
			Msg("[ELICITATION] Already resolved; ignoring")
		return nil
	}

	stored, err := apiServer.Store.GetAgentElicitation(ctx, elicitationID)
	if err != nil {
		return fmt.Errorf("failed to reload elicitation: %w", err)
	}

	log.Info().
		Str("elicitation_id", elicitationID).
		Str("status", stored.Status).
		Str("reason", stored.ResolutionReason).
		Msg("[ELICITATION] Question resolved")

	interaction, err := apiServer.Store.GetInteraction(ctx, stored.InteractionID)
	if err != nil {
		log.Warn().Err(err).Str("interaction_id", stored.InteractionID).
			Msg("[ELICITATION] Could not load interaction to mirror resolution")
	} else {
		apiServer.writeElicitationEntry(ctx, stored.SessionID, interaction, stored)
	}

	apiServer.clearAgentQuestionNotification(ctx, stored)
	return nil
}

// handleElicitationResync applies the agent's statement of which questions it still
// holds: listed ones are alive, and their liveness stamp is refreshed.
//
// The reaping of unlisted questions is deliberately NOT done here. This event is
// per-thread, and a thread that failed to load reports nothing — treating "absent from
// this message" as "dead" would kill live questions during a slow reload. Instead the
// stamps are refreshed here and the reaper acts on silence over time.
func (apiServer *HelixAPIServer) handleElicitationResync(sessionID string, syncMsg *types.SyncMessage) error {
	rawIDs, _ := syncMsg.Data["elicitation_ids"].([]interface{})
	ids := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		if id, ok := raw.(string); ok && id != "" {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := apiServer.Store.TouchAgentElicitations(ctx, ids); err != nil {
		return fmt.Errorf("failed to refresh elicitation liveness: %w", err)
	}
	log.Debug().
		Str("session_id", sessionID).
		Int("count", len(ids)).
		Msg("[ELICITATION] Agent re-affirmed outstanding questions")
	return nil
}

// handleElicitationResponseAck records what the agent did with an answer we sent.
//
// noop means it was already resolved (someone else answered, or the turn moved on);
// not_found means the elicitation or its thread is gone. Both mean the question can no
// longer be answered, so the card must stop offering to.
func (apiServer *HelixAPIServer) handleElicitationResponseAck(sessionID string, syncMsg *types.SyncMessage) error {
	elicitationID, _ := syncMsg.Data["elicitation_id"].(string)
	if elicitationID == "" {
		return fmt.Errorf("missing elicitation_id")
	}
	status, _ := syncMsg.Data["status"].(string)
	errMsg, _ := syncMsg.Data["error"].(string)

	switch status {
	case "accepted":
		// The authoritative terminal status arrives separately via elicitation_resolved.
		return nil
	case "noop", "not_found":
		log.Info().
			Str("session_id", sessionID).
			Str("elicitation_id", elicitationID).
			Str("ack_status", status).
			Str("error", errMsg).
			Msg("[ELICITATION] Answer did not apply; reconciling")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return apiServer.finalizeElicitation(ctx, elicitationID,
			types.ElicitationStatusCancelled, types.ElicitationReasonAgentNoLongerHolds, nil)
	default:
		log.Warn().Str("ack_status", status).Msg("[ELICITATION] Unknown ack status")
		return nil
	}
}

// startElicitationReaper cancels questions no agent has claimed for longer than the grace
// window.
//
// This is the only place Helix declares a question dead of its own accord, and it does so
// on evidence: an agent holding a question re-affirms it every 15s, so silence past the
// grace window means the process that owned the respond_tx is gone. Note what this is
// NOT keyed on — a WebSocket reconnect, which proves nothing because an API restart
// causes one while the agent lives on.
func (apiServer *HelixAPIServer) startElicitationReaper(ctx context.Context) {
	ticker := time.NewTicker(elicitationReapInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				apiServer.reapStaleElicitations(ctx)
			}
		}
	}()
}

func (apiServer *HelixAPIServer) reapStaleElicitations(ctx context.Context) {
	cutoff := time.Now().Add(-elicitationResyncGrace())
	stale, err := apiServer.Store.ReapStaleAgentElicitations(ctx, cutoff)
	if err != nil {
		log.Warn().Err(err).Msg("[ELICITATION] Failed to reap stale questions")
		return
	}
	for _, elicitation := range stale {
		log.Info().
			Str("elicitation_id", elicitation.ID).
			Str("session_id", elicitation.SessionID).
			Msg("[ELICITATION] Reaped question: no agent has claimed it within the grace window")

		interaction, err := apiServer.Store.GetInteraction(ctx, elicitation.InteractionID)
		if err == nil {
			apiServer.writeElicitationEntry(ctx, elicitation.SessionID, interaction, elicitation)
		}
		apiServer.clearAgentQuestionNotification(ctx, elicitation)
	}
}

// notifyAgentQuestion tells the user an agent is waiting on them. One emit reaches the
// in-app bell, the project's Slack thread, and the org event sink.
//
// Deliberately NOT suppressed when the user is active in the session, unlike
// agent_interaction_completed: that heuristic is right for "your turn finished" and wrong
// for a question, which needs an answer either way.
func (apiServer *HelixAPIServer) notifyAgentQuestion(helixSessionID string, elicitation *types.AgentElicitation) {
	if apiServer.attentionService == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		session, err := apiServer.Store.GetSession(ctx, helixSessionID)
		if err != nil || session.Metadata.SpecTaskID == "" {
			return
		}
		task, err := apiServer.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
		if err != nil {
			log.Warn().Err(err).Str("spec_task_id", session.Metadata.SpecTaskID).
				Msg("[ELICITATION] Failed to load spectask for question notification")
			return
		}
		// The elicitation id is the idempotency qualifier, so the agent re-announcing an
		// outstanding question on reconnect cannot re-notify.
		if _, err := apiServer.attentionService.EmitEvent(
			ctx,
			types.AttentionEventAgentQuestion,
			task,
			elicitation.ID,
			map[string]interface{}{
				"interaction_id": elicitation.InteractionID,
				"session_id":     helixSessionID,
				"elicitation_id": elicitation.ID,
				"question":       elicitation.Message,
			},
		); err != nil {
			log.Warn().Err(err).Str("elicitation_id", elicitation.ID).
				Msg("[ELICITATION] Failed to emit question notification")
		}
	}()
}

// clearAgentQuestionNotification dismisses the notification for a resolved question.
// Scoped to this one question — dismissing every event on the task would clear unrelated
// notifications the user has not dealt with.
func (apiServer *HelixAPIServer) clearAgentQuestionNotification(ctx context.Context, elicitation *types.AgentElicitation) {
	session, err := apiServer.Store.GetSession(ctx, elicitation.SessionID)
	if err != nil || session.Metadata.SpecTaskID == "" {
		return
	}
	key := types.BuildAttentionEventIdempotencyKey(
		session.Metadata.SpecTaskID, types.AttentionEventAgentQuestion, elicitation.ID)
	if err := apiServer.Store.DismissAttentionEventByIdempotencyKey(ctx, key); err != nil {
		log.Warn().Err(err).Str("elicitation_id", elicitation.ID).
			Msg("[ELICITATION] Failed to dismiss question notification")
	}
}

// publishSessionElicitationUpdate nudges clients to refetch after a status change that
// did not come through the streaming path.
func (apiServer *HelixAPIServer) publishSessionElicitationUpdate(session *types.Session, interaction *types.Interaction) {
	if session == nil || interaction == nil {
		return
	}
	event := &types.WebsocketEvent{
		Type:          types.WebsocketEventInteractionUpdate,
		SessionID:     session.ID,
		InteractionID: interaction.ID,
		Owner:         session.Owner,
		Interaction:   interaction,
	}
	messageBytes, err := json.Marshal(event)
	if err != nil {
		return
	}
	if err := apiServer.pubsub.Publish(context.Background(),
		pubsub.GetSessionQueue(session.Owner, session.ID), messageBytes); err != nil {
		log.Warn().Err(err).Msg("[ELICITATION] Failed to publish interaction update")
	}
}
