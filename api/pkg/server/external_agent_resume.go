package server

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
)

// Resume protocol for external-agent reconnects.
//
// When the API process restarts, the external-agent WebSocket drops but the
// sandbox, Zed and the coding agent keep running. On reconnect Helix has to
// answer one question about an interaction still sitting in `waiting`:
//
//	is the agent already running this turn, or did it never receive it?
//
// Answering "never received it" while the agent is mid-turn re-sends the prompt
// into a live ACP session. That is not merely wasteful: the duplicate carries
// the SAME request_id, so when the agent rejects it, the resulting
// chat_response_error routes to — and kills — the original, still-streaming
// interaction, after which every subsequent chunk is dropped as unroutable.
// See design/2026-08-16-api-restart-live-turn-reconnect.md.
//
// The authoritative answer comes from the agent. `agent_ready` carries an
// `active_turns` array built from Zed's request-lifecycle registry, which
// survives the WebSocket drop because the Zed process does not restart.

// agentTurnReport is the agent's answer to "which turns are you already
// running?", parsed from an agent_ready event.
type agentTurnReport struct {
	// Reported is false when the agent did not send active_turns at all: either
	// an agent build predating the field, or agent_ready never arrived and the
	// readiness timeout fired instead.
	//
	// Absence is not the same as an empty report. An empty report
	// authoritatively means "I am running nothing"; absence means "I cannot
	// tell you", which is what resumeAttachAndVerify exists to handle.
	Reported bool

	// Active maps request_id to the agent's lifecycle state for that turn,
	// "queued" or "running". Both mean the agent owns the turn and must not be
	// sent it again.
	Active map[string]string
}

// owns reports whether the agent claims the given turn, and in which state.
func (r agentTurnReport) owns(requestID string) (string, bool) {
	if !r.Reported || requestID == "" {
		return "", false
	}
	state, ok := r.Active[requestID]
	return state, ok
}

// parseAgentTurnReport reads the active_turns array off an agent_ready event.
// A malformed entry is skipped rather than failing the whole report: a partial
// report still proves the agent speaks the protocol, and every turn it does
// name is still authoritative.
func parseAgentTurnReport(data map[string]interface{}) agentTurnReport {
	raw, present := data["active_turns"]
	if !present {
		return agentTurnReport{}
	}
	list, ok := raw.([]interface{})
	if !ok {
		return agentTurnReport{}
	}
	report := agentTurnReport{Reported: true, Active: make(map[string]string, len(list))}
	for _, item := range list {
		turn, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		requestID, _ := turn["request_id"].(string)
		if requestID == "" {
			continue
		}
		state, _ := turn["state"].(string)
		if state == "" {
			state = "running"
		}
		report.Active[requestID] = state
	}
	return report
}

// resumeAction is what a reconnect should do with a waiting interaction.
type resumeAction int

const (
	// resumeDeliver: the agent does not have this turn. Send it.
	resumeDeliver resumeAction = iota
	// resumeAttach: the agent is already running this turn. Restore routing and
	// let its output stream in. Sending again would duplicate it.
	resumeAttach
	// resumeAttachAndVerify: the agent could not say. Assume it is running (the
	// cheaper mistake) but arm a bounded probe that delivers if the turn proves
	// dead. See verifyResumedTurn.
	resumeAttachAndVerify
)

func (a resumeAction) String() string {
	switch a {
	case resumeDeliver:
		return "deliver"
	case resumeAttach:
		return "attach"
	case resumeAttachAndVerify:
		return "attach_and_verify"
	}
	return "unknown"
}

// decideResume is the single gate on re-delivering a turn to a reconnected
// agent. Every reconnect path goes through here; nothing else may send a
// chat_message for an interaction that already has ExternalAgentDispatchedAt
// set.
func decideResume(report agentTurnReport, interaction *types.Interaction) (resumeAction, string) {
	if interaction == nil {
		return resumeDeliver, "no interaction"
	}
	if state, owned := report.owns(interaction.ExternalAgentRequestID); owned {
		return resumeAttach, "agent reports turn " + state
	}
	if report.Reported {
		return resumeDeliver, "agent reports no such turn"
	}
	// LEGACY (remove with https://github.com/helixml/helix/issues/3047): the connected agent
	// predates active_turns, so the only evidence available is whether Helix
	// ever handed this turn over. Prefer the recoverable mistake: a turn wrongly
	// assumed alive is corrected by verifyResumedTurn a few minutes later, while
	// a turn wrongly re-sent corrupts a live ACP session immediately.
	if interaction.ExternalAgentDispatchedAt != nil {
		return resumeAttachAndVerify, "agent did not report; turn was already dispatched"
	}
	return resumeDeliver, "agent did not report; turn was never dispatched"
}

// pendingResume is a waiting interaction that a reconnect has correlated to the
// live connection but not yet decided what to do with. The decision needs the
// agent's turn report, which does not arrive until agent_ready, so the resolve
// step (routing, claims, correlation) and the delivery step are separate.
type pendingResume struct {
	sessionID     string
	agentID       string
	interactionID string
	requestID     string
	session       *types.Session
	conn          *ExternalAgentWSConnection

	// deliverable is false when this turn must never be sent — the user
	// cancelled it, or another sender already holds the dispatch claim. The
	// request_id is still carried so open_thread correlates with the live turn.
	deliverable bool

	// abandoned is closed when the connection that owns this resume goes away,
	// so a pending verify probe exits instead of firing against a dead peer.
	abandoned chan struct{}
}

// resolveWaitingInteraction finds the oldest interaction still waiting for this
// session and restores everything needed to route the agent's output to it:
// the request_id correlation maps, the durable request binding, and the
// dispatch claim. It deliberately does NOT send the prompt — see decideResume.
//
// Oldest-first is load-bearing. The first message for a session is the one that
// creates the Zed thread and seeds the agent's context; delivering a newer
// waiting interaction ahead of it leaves the agent running with no context at
// all. See design/2026-06-19-incident-interrupt-during-boot-context-loss.md.
func (apiServer *HelixAPIServer) resolveWaitingInteraction(
	ctx context.Context,
	helixSessionID string,
	helixSession *types.Session,
	agentID string,
	conn *ExternalAgentWSConnection,
) *pendingResume {
	interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    helixSessionID,
		GenerationID: helixSession.GenerationID,
		PerPage:      1000,
	})
	if err != nil || len(interactions) == 0 {
		return nil
	}

	for i := 0; i < len(interactions); i++ {
		if interactions[i].State != types.InteractionStateWaiting {
			continue
		}
		interactionID := interactions[i].ID

		apiServer.contextMappingsMutex.Lock()
		currentInteraction, getErr := apiServer.Controller.Options.Store.GetInteraction(ctx, interactionID)
		if getErr != nil || currentInteraction.State != types.InteractionStateWaiting {
			apiServer.contextMappingsMutex.Unlock()
			continue
		}

		// Reuse an in-memory request_id for this session if one survived, else
		// fall back to the durable column, else to the interaction id (the
		// convention sendMessageToSpecTaskAgent uses).
		var requestID string
		for rid, sid := range apiServer.requestToSessionMapping {
			if sid == helixSessionID {
				requestID = rid
				break
			}
		}
		if requestID == "" {
			requestID = currentInteraction.ExternalAgentRequestID
			if requestID == "" {
				requestID = interactionID
			}
			if apiServer.requestToSessionMapping == nil {
				apiServer.requestToSessionMapping = make(map[string]string)
			}
			apiServer.requestToSessionMapping[requestID] = helixSessionID
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("request_id", requestID).
				Msg("🔧 [HELIX] Created request_id mapping from waiting interaction ID")
		}

		// If RunExternalAgent is already delivering this turn, it owns it. Return
		// the winner's request_id so the open_thread this reconnect sends still
		// correlates with the turn that is actually running.
		winnerRequestID, won := apiServer.claimInteractionDispatchLocked(helixSessionID, interactionID, requestID)
		if !won {
			apiServer.contextMappingsMutex.Unlock()
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("interaction_id", interactionID).
				Str("request_id", winnerRequestID).
				Msg("⏭️ [HELIX] Interaction already being delivered by another sender — not re-sending")
			return &pendingResume{
				sessionID:     helixSessionID,
				agentID:       agentID,
				interactionID: interactionID,
				requestID:     winnerRequestID,
				session:       helixSession,
				conn:          conn,
				abandoned:     make(chan struct{}),
			}
		}

		if apiServer.requestToInteractionMapping == nil {
			apiServer.requestToInteractionMapping = make(map[string]string)
		}
		apiServer.requestToInteractionMapping[requestID] = interactionID
		apiServer.contextMappingsMutex.Unlock()

		if currentInteraction.ExternalAgentRequestID != requestID {
			bound, bindErr := apiServer.Store.BindInteractionExternalAgentRequest(ctx, interactionID, currentInteraction.GenerationID, requestID)
			if bindErr != nil || !bound {
				apiServer.releaseInteractionDispatch(interactionID)
				log.Error().Err(bindErr).
					Str("interaction_id", interactionID).
					Str("request_id", requestID).
					Msg("Failed to persist external-agent request mapping before pickup")
				return nil
			}
			currentInteraction.ExternalAgentRequestID = requestID
		}

		resume := &pendingResume{
			sessionID:     helixSessionID,
			agentID:       agentID,
			interactionID: interactionID,
			requestID:     requestID,
			session:       helixSession,
			conn:          conn,
			deliverable:   true,
			abandoned:     make(chan struct{}),
		}

		// A cancel request can outlive the WebSocket connection. Deliver the
		// cancellation instead of the message the user explicitly stopped.
		if currentInteraction.ExternalAgentCancelRequestedAt != nil {
			resume.deliverable = false
			cancelCommand := types.ExternalAgentCommand{
				Type: "cancel_current_turn",
				Data: map[string]interface{}{"request_id": requestID, "session_id": helixSessionID},
			}
			if !apiServer.externalAgentWSManager.queueOrSend(helixSessionID, cancelCommand) {
				log.Warn().
					Str("helix_session_id", helixSessionID).
					Str("request_id", requestID).
					Msg("Failed to queue durable cancellation on reconnect")
			}
		}

		return resume
	}
	return nil
}

// applyResumeDecision runs once per reconnect, after the agent has had its say
// via agent_ready (or the readiness timeout has given up waiting for it).
func (apiServer *HelixAPIServer) applyResumeDecision(resume *pendingResume, report agentTurnReport) {
	if resume == nil || !resume.deliverable {
		return
	}
	ctx := context.Background()

	interaction, err := apiServer.Store.GetInteraction(ctx, resume.interactionID)
	if err != nil || interaction == nil {
		log.Warn().Err(err).
			Str("interaction_id", resume.interactionID).
			Msg("[RESUME] Could not reload interaction for resume decision")
		return
	}
	// The readiness window is wide enough for the turn to have finished, been
	// cancelled, or errored while we waited for agent_ready.
	if interaction.State != types.InteractionStateWaiting || interaction.ExternalAgentCancelRequestedAt != nil {
		log.Info().
			Str("session_id", resume.sessionID).
			Str("interaction_id", resume.interactionID).
			Str("state", string(interaction.State)).
			Msg("[RESUME] Turn is no longer waiting — nothing to resume")
		apiServer.releaseInteractionDispatch(resume.interactionID)
		return
	}

	action, reason := decideResume(report, interaction)
	log.Info().
		Str("session_id", resume.sessionID).
		Str("interaction_id", resume.interactionID).
		Str("request_id", resume.requestID).
		Str("action", action.String()).
		Str("reason", reason).
		Bool("agent_reported", report.Reported).
		Msg("🔀 [RESUME] Reconnect decision for waiting turn")

	switch action {
	case resumeDeliver:
		apiServer.deliverResumedTurn(ctx, resume, interaction)
	case resumeAttach:
		// Nothing to send. Routing was restored by resolveWaitingInteraction, so
		// the turn's output streams straight back into this interaction.
	case resumeAttachAndVerify:
		go apiServer.verifyResumedTurn(resume)
	}
}

// deliverWaitingInteractionNow resolves and immediately decides a session's
// waiting turn against the live connection, with no agent report available.
//
// Used by the agent-switch handoff, where the interaction was created moments
// earlier and has therefore never been dispatched — decideResume returns
// resumeDeliver for it. If the turn HAS already been dispatched, this correctly
// declines to duplicate it, exactly as a reconnect would.
func (apiServer *HelixAPIServer) deliverWaitingInteractionNow(ctx context.Context, session *types.Session) string {
	conn, _ := apiServer.externalAgentWSManager.getConnection(session.ID)
	resume := apiServer.resolveWaitingInteraction(ctx, session.ID, session, "", conn)
	if resume == nil {
		return ""
	}
	// Abandon before deciding: the verify probe belongs to a connection's resume
	// lifecycle, and this is a one-shot call on an already-established
	// connection, not a reconnect. If the turn was already dispatched the
	// correct outcome here is to do nothing and leave it to the agent.
	defer close(resume.abandoned)
	apiServer.applyResumeDecision(resume, agentTurnReport{})
	return resume.requestID
}

// deliverResumedTurn sends the waiting interaction's prompt to the agent. This
// is the only place a reconnect may send a chat_message, and it is reachable
// only through a decideResume verdict.
func (apiServer *HelixAPIServer) deliverResumedTurn(ctx context.Context, resume *pendingResume, interaction *types.Interaction) {
	fullMessage := interaction.PromptMessage
	if interaction.SystemPrompt != "" {
		fullMessage = interaction.SystemPrompt + "\n\n**User Request:**\n" + interaction.PromptMessage
	}

	// Re-read the session rather than trusting the snapshot taken at connect.
	// Delivery can be minutes behind that snapshot (the readiness gate, or the
	// verify probe's silence budget), and ZedThreadID is exactly the field an
	// intervening thread_created would have filled in. Sending a nil
	// acp_thread_id against a session that now HAS a thread makes Zed open a
	// second one for the same turn.
	session := resume.session
	if fresh, err := apiServer.Store.GetSession(ctx, resume.sessionID); err == nil && fresh != nil {
		session = fresh
	} else {
		log.Warn().Err(err).
			Str("session_id", resume.sessionID).
			Msg("[RESUME] Could not refresh session before delivery; using the snapshot from connect")
	}

	var acpThreadID interface{}
	if session.Metadata.ZedThreadID != "" {
		acpThreadID = session.Metadata.ZedThreadID
		log.Info().
			Str("helix_session_id", resume.sessionID).
			Str("zed_thread_id", session.Metadata.ZedThreadID).
			Msg("🔗 [HELIX] Resuming in existing Zed thread after reconnect")
	}

	// Forked sessions: if this is the first message (no Zed thread yet) and a
	// fork_seed interaction exists, prepend the parent transcript. No-op on
	// non-forked sessions and on reconnect-to-existing-thread.
	fullMessage = apiServer.maybePrependTranscript(ctx, session, fullMessage)

	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"message":                   fullMessage,
			"request_id":                resume.requestID,
			"acp_thread_id":             acpThreadID,
			"agent_name":                apiServer.getAgentNameForSession(ctx, session),
			"interaction_id":            resume.interactionID,
			"interaction_generation_id": interaction.GenerationID,
			"track_code_changes":        session.Metadata.SpecTaskID != "",
		},
	}
	apiServer.captureInteractionBeforeCheckpoint(resume.sessionID, command)

	apiServer.contextMappingsMutex.Lock()
	rollbackDispatch, dispatchErr := apiServer.markExternalAgentCommandDispatched(ctx, command)
	if dispatchErr != nil {
		apiServer.contextMappingsMutex.Unlock()
		apiServer.releaseInteractionDispatch(resume.interactionID)
		log.Error().Err(dispatchErr).
			Str("interaction_id", resume.interactionID).
			Str("request_id", resume.requestID).
			Msg("Failed to persist reconnect dispatch")
		return
	}
	queued := apiServer.externalAgentWSManager.queueOrSend(resume.sessionID, command)
	apiServer.contextMappingsMutex.Unlock()

	if queued {
		log.Info().
			Str("agent_session_id", resume.agentID).
			Str("request_id", resume.requestID).
			Str("helix_session_id", resume.sessionID).
			Msg("✅ [HELIX] Delivered waiting chat_message to reconnected agent")
		return
	}
	rollbackDispatch()
	// Nothing was delivered — drop the claim so the next reconnect can pick this
	// interaction up again.
	apiServer.releaseInteractionDispatch(resume.interactionID)
	log.Warn().
		Str("agent_session_id", resume.agentID).
		Msg("⚠️ [HELIX] Failed to queue waiting chat_message")
}

// lastStreamPublish returns when content was last forwarded to the frontend for
// a session, and whether a streaming context exists at all.
func (apiServer *HelixAPIServer) lastStreamPublish(sessionID string) (time.Time, bool) {
	apiServer.streamingContextsMu.RLock()
	sctx := apiServer.streamingContexts[sessionID]
	apiServer.streamingContextsMu.RUnlock()
	if sctx == nil {
		return time.Time{}, false
	}
	sctx.mu.Lock()
	defer sctx.mu.Unlock()
	return sctx.lastPublish, true
}

// verifyResumedTurn is the correction half of resumeAttachAndVerify.
//
// It waits out the same silence budget the auto-wake worker uses (a turn inside
// a long tool call emits nothing while the tool runs, so the budget has to
// cover that), then re-delivers only if every signal still says the turn is
// dead: the interaction never left `waiting`, no content ever reached the
// frontend, the user did not cancel, and this is still the connection that made
// the decision.
//
// Any one of those being false means either the turn was alive after all or a
// newer reconnect has since made its own decision — in both cases delivering
// here would be the duplicate this whole file exists to prevent.
func (apiServer *HelixAPIServer) verifyResumedTurn(resume *pendingResume) {
	budget := autoWakeStuckThreshold()
	timer := time.NewTimer(budget)
	defer timer.Stop()

	select {
	case <-resume.abandoned:
		return
	case <-timer.C:
	}

	ctx := context.Background()
	logger := log.With().
		Str("session_id", resume.sessionID).
		Str("interaction_id", resume.interactionID).
		Str("request_id", resume.requestID).
		Dur("budget", budget).
		Logger()

	// Still the connection that decided? A newer connect ran its own
	// decideResume and owns the turn now.
	if conn, ok := apiServer.externalAgentWSManager.getConnection(resume.sessionID); !ok || conn != resume.conn {
		logger.Debug().Msg("[RESUME] Verify skipped — connection replaced by a newer reconnect")
		return
	}

	// Any content at all disproves "the turn is dead".
	if lastPublish, streaming := apiServer.lastStreamPublish(resume.sessionID); streaming {
		logger.Info().
			Time("last_publish", lastPublish).
			Msg("[RESUME] Verify cleared — attached turn produced content, no re-delivery needed")
		return
	}

	interaction, err := apiServer.Store.GetInteraction(ctx, resume.interactionID)
	if err != nil || interaction == nil {
		logger.Warn().Err(err).Msg("[RESUME] Verify skipped — could not reload interaction")
		return
	}
	if interaction.State != types.InteractionStateWaiting {
		logger.Debug().
			Str("state", string(interaction.State)).
			Msg("[RESUME] Verify cleared — turn left waiting on its own")
		return
	}
	if interaction.ExternalAgentCancelRequestedAt != nil {
		logger.Info().Msg("[RESUME] Verify skipped — cancellation was requested for this turn")
		return
	}

	logger.Warn().
		Msg("💤 [RESUME] Attached turn stayed silent for the full budget — re-delivering (agent could not report active turns)")
	apiServer.deliverResumedTurn(ctx, resume, interaction)
}

// newestWaitingInteraction is the selector of last resort, for events that carry
// no request_id to correlate on (an uncorrelated thread_load_error, where Zed
// could not tie an open_thread failure to a turn). Anything that DOES carry a
// request_id must resolve through interactionForRequest instead — picking the
// newest waiting turn for a correlated event is how an unrelated turn ends up
// wearing another turn's failure.
func (apiServer *HelixAPIServer) newestWaitingInteraction(ctx context.Context, session *types.Session) *types.Interaction {
	interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    session.ID,
		GenerationID: session.GenerationID,
		PerPage:      1000,
	})
	if err != nil {
		return nil
	}
	for i := len(interactions) - 1; i >= 0; i-- {
		if interactions[i].State == types.InteractionStateWaiting {
			return interactions[i]
		}
	}
	return nil
}
