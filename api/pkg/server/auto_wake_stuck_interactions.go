// Auto-wake worker for stuck `state=waiting` interactions.
//
// # Why this exists
//
// ACP (Agent Client Protocol) is request/response with streaming notifications
// scoped to one user-driven turn. The protocol assumes that every agent
// session_update notification is a downstream of the most recent
// session/prompt — there is no first-class verb for "the agent has news that
// didn't arise from a user prompt", no subscription channel, no long-poll
// for unprompted events. See agentclientprotocol/agent-client-protocol#554.
//
// Modern Claude Code has many non-user-initiated triggers for agent activity:
// background bash commands finishing, hooks firing, subagents completing,
// compaction running, MCP servers emitting `tools/list_changed`, etc. The
// `claude-agent-acp` wrapper has events the user needs to see and no
// protocol-legal place to put them. So it buffers them on the outbound
// JSON-RPC channel and waits for the next session/prompt to flush. From our
// observations the wrapper's outbound flush is event-loop-tick driven and
// only effectively kicked by inbound RPC traffic.
//
// User-visible symptom: the user sends a prompt, Zed UI appears done, Helix
// tracks the interaction in `state=waiting` with no `response_message` and
// no `message_completed` arriving from the wrapper. The buffer drains on
// the *next* session/prompt — historically the user discovers this by
// typing "??", "carry on", or "fix it", at which point the previous turn's
// response and the new one both show up together.
//
// # What this worker does
//
// Every ~10 s, scan for interactions matching:
//
//   - state = waiting
//   - response_message = ''
//   - response_entries IS NULL
//   - created < now() - 30s
//
// For each, count any auto-wake interactions that already exist in the same
// session created after it. If the count is < 2, send a "continue" prompt
// via sendChatMessageToExternalAgent and tag the *new* interaction with
// auto_wake_count = N+1. If it's already ≥ 2, mark the stuck interaction
// state=error so it stops being matched and the user sees a clear failure.
//
// # Why "continue" specifically
//
// We compared four wake-up payloads (see design doc). "continue" is the
// least-bad option: it's a verb the model handles as a near-no-op when
// there's nothing to continue, doesn't require resending the user's
// original prompt (which could cause double work), and matches the wake-up
// pattern users already do manually with "?"/"??".
//
// # Why we bypass the prompt queue and never set interrupt=true
//
// The queue dispatch in prompt_history_handlers.go has a busy check that
// defers any new prompt while there's a waiting interaction in the
// session — i.e. it would deadlock the auto-wake against the very
// interaction we're trying to wake. Two ways around it:
//
//  1. Send via queue with interrupt=true → triggers session/cancel before
//     the prompt → triggers claude-agent-acp#551 (cancel-then-prompt
//     swallow) → "continue" itself gets bounced. This is *worse* than not
//     auto-waking at all.
//
//  2. Bypass the queue entirely — call sendChatMessageToExternalAgent
//     directly. The whole premise of this worker is that the wrapper IS
//     idle (it emitted stopReason for the prior turn — that's *why* Zed
//     UI shows "done") and Helix's `state=waiting` is bookkeeping that
//     never received message_completed. The busy check is reasoning about
//     the wrong thing in our scenario, so we sidestep rather than relax it.
//
// We use option (2). Future maintainers will be tempted to "fix" this by
// routing through the queue. Don't.
//
// # Coverage gap
//
// This worker only handles the case where Zed *did* relay the user message
// to Helix but the response never came (interaction lands in `state=waiting`).
// It does NOT cover the case where Zed accepts a keystroke, displays it in
// its own UI, but never sends the corresponding session/update to Helix at
// all. We have no signal to detect that case from inside the API process.
// See the "A worse failure mode: Zed-side silent drop" section of the
// design doc.
//
// # Expected lifetime
//
// Until either:
//   - agentclientprotocol/agent-client-protocol#554 lands a turn-complete
//     barrier, claude-agent-acp ships an honouring release, and Zed picks
//     it up — at which point the wrapper's outbound buffer should drain
//     deterministically and this worker becomes a no-op that we can
//     feature-flag off; or
//   - The protocol grows a real unparented event channel and the wrapper
//     stops stuffing async events into the turn-scoped notification stream
//     in the first place.
//
// Neither is imminent (see design doc "Where the ACP team is on this").
// Plan to ship this with the codebase for months at least.

package server

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const (
	// autoWakeScanInterval is how often the worker checks for stuck
	// interactions. Tighter than this just wastes DB queries; looser than
	// this leaves the user staring at a frozen chat for longer than needed.
	autoWakeScanInterval = 10 * time.Second

	// autoWakeStuckThreshold is the minimum age of a `state=waiting`
	// interaction before we consider it stuck. Set well above the
	// observed normal time-to-first-token for any agent (Claude usually
	// streams the first token in under 5 s; a 30 s gap is a strong signal
	// the wrapper has buffered something).
	autoWakeStuckThreshold = 30 * time.Second

	// autoWakeMaxRetries caps how many "continue" wake-ups we send for a
	// single stuck interaction before giving up. Two is generous — in
	// practice the empirical pattern is that one wake-up flushes the
	// buffer. Beyond two we've almost certainly hit a different failure
	// mode that auto-wake can't fix.
	autoWakeMaxRetries = 2

	// autoWakeContinueMessage is the wake-up payload. See the function
	// comment above for why "continue" beat the alternatives.
	autoWakeContinueMessage = "continue"
)

// startAutoWakeStuckInteractionsWorker launches the periodic scanner.
// Idempotent in spirit but the API server only calls it once at init.
func (apiServer *HelixAPIServer) startAutoWakeStuckInteractionsWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(autoWakeScanInterval)
		defer ticker.Stop()

		log.Info().
			Dur("scan_interval", autoWakeScanInterval).
			Dur("stuck_threshold", autoWakeStuckThreshold).
			Int("max_retries", autoWakeMaxRetries).
			Msg("🚀 [AUTO_WAKE] Started auto-wake worker for stuck waiting interactions")

		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("[AUTO_WAKE] Context cancelled, stopping worker")
				return
			case <-ticker.C:
				apiServer.scanAndAutoWakeStuckInteractions(ctx)
			}
		}
	}()
}

// scanAndAutoWakeStuckInteractions runs one detection pass.
func (apiServer *HelixAPIServer) scanAndAutoWakeStuckInteractions(ctx context.Context) {
	cutoff := time.Now().Add(-autoWakeStuckThreshold)

	// Bound the work per tick. 50 is well above the realistic burst rate
	// of stuck interactions on a single API replica.
	stuck, err := apiServer.Store.ListStuckWaitingInteractions(ctx, cutoff, 50)
	if err != nil {
		log.Warn().Err(err).Msg("[AUTO_WAKE] Failed to query stuck interactions")
		return
	}

	for _, interaction := range stuck {
		apiServer.maybeAutoWake(ctx, interaction)
	}
}

// maybeAutoWake fires a wake-up for a single stuck interaction iff we
// haven't already exhausted retries on it.
func (apiServer *HelixAPIServer) maybeAutoWake(ctx context.Context, stuck *types.Interaction) {
	// Count auto-wake interactions already sent for this session that
	// were created after `stuck`. If the count is ≥ retry cap, we've
	// done what we can and should stop firing — mark the stuck
	// interaction as terminal so subsequent scans don't re-match it.
	existingRetries, err := apiServer.Store.CountAutoWakeAttemptsSince(ctx, stuck.SessionID, stuck.Created)
	if err != nil {
		log.Warn().Err(err).
			Str("interaction_id", stuck.ID).
			Msg("[AUTO_WAKE] Failed to count existing retries; skipping")
		return
	}

	if existingRetries >= autoWakeMaxRetries {
		// Exhausted. Mark stuck as error and stop matching it.
		stuck.State = types.InteractionStateError
		stuck.Error = "Agent unresponsive after auto-wake retries (upstream ACP buffering — see design/2026-04-25-zed-claude-async-event-flush-on-user-input.md)"
		stuck.Updated = time.Now()
		stuck.Completed = time.Now()
		if _, err := apiServer.Store.UpdateInteraction(ctx, stuck); err != nil {
			log.Warn().Err(err).
				Str("interaction_id", stuck.ID).
				Msg("[AUTO_WAKE] Failed to mark exhausted interaction as error")
			return
		}
		log.Warn().
			Str("interaction_id", stuck.ID).
			Str("session_id", stuck.SessionID).
			Int64("retries_attempted", existingRetries).
			Msg("⚠️ [AUTO_WAKE] Exhausted retries — marking interaction as error")
		return
	}

	// Fresh request_id — must NOT collide with any prior mapping or
	// the wrapper's per-session request bookkeeping.
	requestID := "autowake_" + system.GenerateUUID()
	attempt := int(existingRetries) + 1

	log.Info().
		Str("stuck_interaction_id", stuck.ID).
		Str("session_id", stuck.SessionID).
		Int("attempt", attempt).
		Time("stuck_created_at", stuck.Created).
		Str("request_id", requestID).
		Msg("🔔 [AUTO_WAKE] Sending wake-up prompt to unstick session — upstream ACP claude-agent-acp #551 / agent-client-protocol #554")

	// Bypass the prompt queue (busy check would deadlock against the
	// stuck interaction itself) and never set interrupt=true (it
	// triggers claude-agent-acp #551 which would bounce the wake-up
	// itself). See the file header for the full reasoning.
	newInteractionID, err := apiServer.sendChatMessageToExternalAgent(stuck.SessionID, autoWakeContinueMessage, requestID)
	if err != nil {
		log.Warn().Err(err).
			Str("stuck_interaction_id", stuck.ID).
			Str("session_id", stuck.SessionID).
			Msg("[AUTO_WAKE] Failed to send wake-up prompt")
		return
	}
	if newInteractionID == "" {
		// sendChatMessageToExternalAgent only returns an empty ID when
		// the session lookup or interaction creation fails before the
		// command goes out — which it would have logged separately.
		// Nothing for us to tag, so just return.
		return
	}

	// Tag the new auto-wake interaction so:
	//   1. The frontend can render the "↻ Helix auto-sent" badge.
	//   2. The next scan tick can count this attempt towards
	//      autoWakeMaxRetries.
	//
	// We re-fetch rather than mutating the in-memory return value
	// because sendChatMessageToExternalAgent doesn't expose the full
	// created struct — only the ID.
	autoWakeInteraction, fetchErr := apiServer.Store.GetInteraction(ctx, newInteractionID)
	if fetchErr != nil {
		log.Warn().Err(fetchErr).
			Str("auto_wake_interaction_id", newInteractionID).
			Msg("[AUTO_WAKE] Sent wake-up but failed to re-fetch new interaction to tag it")
		return
	}
	autoWakeInteraction.AutoWakeCount = attempt
	autoWakeInteraction.Updated = time.Now()
	if _, err := apiServer.Store.UpdateInteraction(ctx, autoWakeInteraction); err != nil {
		log.Warn().Err(err).
			Str("auto_wake_interaction_id", newInteractionID).
			Int("attempt", attempt).
			Msg("[AUTO_WAKE] Sent wake-up but failed to tag interaction with auto_wake_count")
		return
	}

	log.Info().
		Str("stuck_interaction_id", stuck.ID).
		Str("auto_wake_interaction_id", newInteractionID).
		Int("attempt", attempt).
		Msg("✅ [AUTO_WAKE] Wake-up sent and tagged")
}
