package server

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
)

// liveTurnEvidenceWindow is how long after the last streamed chunk a turn still
// counts as demonstrably alive. It only has to cover the gap between successive
// events of a turn that is actively producing output — not the duration of a
// long tool call, because a turn that has genuinely aborted simply stops
// producing events and the deferred re-check then applies the error.
const liveTurnEvidenceWindow = 10 * time.Second

// streamingEvidence reports when content was last forwarded to the frontend for
// a specific interaction, and whether the session is streaming that interaction
// at all.
func (apiServer *HelixAPIServer) streamingEvidence(sessionID, interactionID string) (time.Time, bool) {
	apiServer.streamingContextsMu.RLock()
	sctx := apiServer.streamingContexts[sessionID]
	apiServer.streamingContextsMu.RUnlock()
	if sctx == nil {
		return time.Time{}, false
	}
	sctx.mu.Lock()
	defer sctx.mu.Unlock()
	if sctx.interactionID != interactionID {
		return time.Time{}, false
	}
	return sctx.lastPublish, true
}

// applyTurnError moves an interaction to state=error — unless the turn it names
// is demonstrably still alive.
//
// Errors from the agent are keyed by request_id, and a request_id identifies a
// turn, not a delivery attempt. So an agent that rejects a *second* delivery of
// a turn it is already running reports that rejection against the turn itself.
// Applying it blindly kills a healthy, streaming turn and strands the session:
// every subsequent chunk becomes unroutable. That is the 2026-08-16 incident
// (see design/2026-08-16-api-restart-live-turn-reconnect.md).
//
// Rather than trying to classify errors by message text, this decides on
// evidence: if content is still flowing into the interaction, the error is
// deferred by one evidence window and re-examined. A turn that really did abort
// stops producing content and the error lands a few seconds later; a turn that
// is alive keeps producing content and the error is discarded.
func (apiServer *HelixAPIServer) applyTurnError(ctx context.Context, interaction *types.Interaction, errorMsg string) {
	if interaction == nil {
		return
	}
	if lastPublish, streaming := apiServer.streamingEvidence(interaction.SessionID, interaction.ID); streaming && time.Since(lastPublish) < liveTurnEvidenceWindow {
		log.Warn().
			Str("session_id", interaction.SessionID).
			Str("interaction_id", interaction.ID).
			Str("request_id", interaction.ExternalAgentRequestID).
			Time("last_publish", lastPublish).
			Str("error", errorMsg).
			Msg("⏸️ [TURN] Error names a turn that is still streaming — deferring; likely a rejected duplicate delivery")
		go apiServer.reapplyTurnErrorAfterSilence(interaction.SessionID, interaction.ID, errorMsg)
		return
	}
	apiServer.commitTurnError(ctx, interaction, errorMsg)
}

// reapplyTurnErrorAfterSilence re-examines a deferred error once the evidence
// window has passed. Fresh content means the turn outlived the error and the
// error is dropped for good.
func (apiServer *HelixAPIServer) reapplyTurnErrorAfterSilence(sessionID, interactionID, errorMsg string) {
	time.Sleep(liveTurnEvidenceWindow)

	ctx := context.Background()
	logger := log.With().
		Str("session_id", sessionID).
		Str("interaction_id", interactionID).
		Logger()

	if lastPublish, streaming := apiServer.streamingEvidence(sessionID, interactionID); streaming && time.Since(lastPublish) < liveTurnEvidenceWindow {
		logger.Info().
			Time("last_publish", lastPublish).
			Msg("✅ [TURN] Deferred error discarded — turn is still producing output")
		return
	}

	interaction, err := apiServer.Store.GetInteraction(ctx, interactionID)
	if err != nil || interaction == nil {
		logger.Warn().Err(err).Msg("[TURN] Could not reload interaction to re-apply deferred error")
		return
	}
	if interaction.State != types.InteractionStateWaiting {
		logger.Debug().
			Str("state", string(interaction.State)).
			Msg("[TURN] Deferred error dropped — turn already reached a terminal state")
		return
	}

	logger.Warn().Str("error", errorMsg).Msg("[TURN] Turn went silent after deferred error — applying it now")
	apiServer.commitTurnError(ctx, interaction, errorMsg)
}

// commitTurnError performs the state transition and the crash/auto-restart
// bookkeeping that follows a terminal turn error.
func (apiServer *HelixAPIServer) commitTurnError(ctx context.Context, interaction *types.Interaction, errorMsg string) {
	// When a subscription-mode Claude Code session aborts with the generic ACP
	// mid-turn message, the real cause is often an invalid subscription token
	// (401) that Zed only logs. Re-probe the owner's subscription and, if it is
	// genuinely bad, replace the useless generic string with a legible auth
	// error.
	errorMsg = apiServer.maybeReclassifySubscriptionAuthError(ctx, interaction.SessionID, errorMsg)

	interaction.State = types.InteractionStateError
	interaction.Error = errorMsg
	interaction.Updated = time.Now()
	if _, err := apiServer.Controller.Options.Store.UpdateInteraction(ctx, interaction); err != nil {
		log.Warn().Err(err).
			Str("interaction_id", interaction.ID).
			Msg("[TURN] Failed to persist turn error")
	}
	apiServer.failRunningTriggerExecution(interaction.SessionID, errorMsg)

	if !isAgentCrashError(errorMsg) {
		return
	}
	// Crash-marking is for QUEUE prompts only (so the queue stops
	// re-dispatching into a dead process); a blocking send has no PromptID and
	// needs none.
	if interaction.PromptID != "" {
		if markErr := apiServer.Controller.Options.Store.MarkPromptAsCrashed(
			ctx, interaction.PromptID, "Agent crashed: "+errorMsg,
		); markErr != nil {
			log.Error().Err(markErr).
				Str("prompt_id", interaction.PromptID).
				Str("interaction_id", interaction.ID).
				Msg("[TURN] Failed to crash-mark prompt")
		}
	}
	log.Warn().
		Str("session_id", interaction.SessionID).
		Str("interaction_id", interaction.ID).
		Msg("💥 [HELIX] Agent crash surfaced on turn error — evaluating auto-restart")
	// The auto-restart decision keys only on "crash + autonomous"; maybeAutoRestart
	// self-gates on the flag and dedupes.
	go apiServer.maybeAutoRestartCrashedAgent(interaction.SessionID)
}
