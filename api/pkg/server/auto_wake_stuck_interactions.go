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
// JSON-RPC channel and only flushes them when the next session/prompt
// arrives. The result, observed empirically: the user sends a prompt X,
// nothing happens; the user types again; the *previous* turn's response
// arrives attached to the new prompt's interaction (off-by-one delivery
// via Helix's stale-request_id fallback in handleMessageCompleted). The
// stuck interaction X never gets its own response.
//
// # What this worker does (in-place retry)
//
// Every ~10 s, scan for interactions matching:
//
//   - state = waiting
//   - response_message = ''
//   - response_entries IS NULL
//   - created < now() - 30s
//
// For each candidate, IF the session has an active WebSocket connection
// to the external agent, re-send X's own `prompt_message` over the wire
// with a fresh request_id mapped back to X.ID, and bump X.auto_wake_count
// via a targeted column UPDATE. We do *not* create a new interaction
// for the wake-up. Two consequences:
//
//  1. The buffered head-of-queue at the wrapper drains under the kick
//     of the new inbound RPC. Whatever arrives carries an old stale
//     request_id; Helix's matcher fallback ("most recent waiting
//     interaction") binds it to X — because X is still the only waiting
//     interaction in the session. The off-by-one shift that would
//     otherwise misroute the response is eliminated by not having
//     a "next" interaction to be off by.
//
//  2. The wake-up does not pollute the user's chat history with extra
//     "continue"/"retry" prompts. The frontend renders X with a
//     "↻ Retried Nx" badge counted off `auto_wake_count`.
//
// After two unsuccessful retries (auto_wake_count >= 2 and X still in
// state=waiting), X is marked state=error so subsequent scans don't
// re-match it.
//
// # Why we re-send the original prompt content
//
// We tried "continue" first. User observation: "in the end only one
// message ends up being sent to the agent" — the wrapper bounces or
// drops most of our wake-ups. Whichever wake-up actually gets through
// to Claude should carry the user's real intent, not "continue" (which
// elicits "continue what?"). Re-sending X.prompt_message is idempotent
// in intent: at worst the agent processes the same prompt twice.
//
// # Why we gate on WebSocket connection
//
// New sessions take "multiple minutes" to boot the desktop and bring the
// claude-agent-acp wrapper up. During boot the spec planning prompt sits
// in state=waiting because the agent hasn't connected yet. Without this
// gate the worker fires every 10 s during the boot window — the
// downstream sendCommandToExternalAgent would either trigger a redundant
// auto-start of the dev container or fail with "no WebSocket connection
// found", and either way wastes the retry budget on a non-issue. Skip
// until the session has an active externalAgentWSManager connection.
//
// # Why we bypass sendChatMessageToExternalAgent
//
// Earlier versions of this worker called sendChatMessageToExternalAgent
// (which creates a fresh interaction, then post-tagged it with
// auto_wake_count via a separate UPDATE). The streaming path's
// concurrent UpdateInteraction calls — which use GORM Save and so write
// every column from their stale in-memory copy — raced against our
// post-tag and overwrote auto_wake_count back to 0. The retry-cap
// counter never engaged and the worker fired forever. Witnessed live on
// spt_01kq2308n428ss3wrm67ta6mjd: 6 wake-ups in 70 seconds. Now we
// inline the WS send and don't create a new interaction at all, so
// there's no row for the streaming path to clobber.
//
// # Why we never set interrupt=true
//
// Routing through the queue's interrupt path triggers session/cancel,
// which triggers claude-agent-acp#551 (cancel-then-prompt swallow): the
// next 1-2 prompts return stopReason=end_turn immediately with 0 tokens.
// Our wake-up itself would be one of the swallowed prompts. We send via
// sendCommandToExternalAgent directly with a plain chat_message.
//
// # Coverage gap
//
// This worker only handles the case where Zed *did* relay the user
// message to Helix but the response never came (interaction lands in
// `state=waiting`). It does NOT cover the case where Zed accepts a
// keystroke, displays it in its own UI, but never sends the
// corresponding session/update to Helix at all. We have no signal to
// detect that case from inside the API process. See the "A worse
// failure mode: Zed-side silent drop" section of the design doc.
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
	// interactions. Tighter than this just wastes DB queries; looser
	// leaves the user staring at a frozen chat for longer than needed.
	autoWakeScanInterval = 10 * time.Second

	// autoWakeStuckThreshold is the minimum age of a `state=waiting`
	// interaction before we consider it stuck. Set well above the
	// observed normal time-to-first-token for any agent (Claude usually
	// streams the first token in under 5 s; a 30 s gap is a strong
	// signal the wrapper has buffered something).
	autoWakeStuckThreshold = 30 * time.Second

	// autoWakeMaxRetries caps how many wake-ups we attempt for a single
	// stuck interaction before giving up and marking it state=error.
	autoWakeMaxRetries = 2
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

	stuck, err := apiServer.Store.ListStuckWaitingInteractions(ctx, cutoff, 50)
	if err != nil {
		log.Warn().Err(err).Msg("[AUTO_WAKE] Failed to query stuck interactions")
		return
	}

	for _, interaction := range stuck {
		apiServer.maybeAutoWake(ctx, interaction)
	}
}

// maybeAutoWake fires an in-place retry for a single stuck interaction
// if all gates pass. See the file header for the design rationale.
func (apiServer *HelixAPIServer) maybeAutoWake(ctx context.Context, stuck *types.Interaction) {
	// Gate 1 — WebSocket connection AND grace period since connect.
	//
	// If the agent isn't connected yet (desktop still booting after a
	// fresh session start, or after a reconnect) or has dropped,
	// sending a wake-up is pointless. The downstream
	// sendCommandToExternalAgent would either trigger a redundant
	// auto-start of the dev container or fail outright.
	//
	// Also: even when the connection has just come up, the agent may
	// be processing a prompt that was queued during boot — give it the
	// same grace period (autoWakeStuckThreshold) before deciding it's
	// stuck. This prevents the worker from firing on a 5-minute-old
	// interaction that the agent only started processing 5 seconds ago
	// because the WS reconnect just completed.
	conn, connected := apiServer.externalAgentWSManager.getConnection(stuck.SessionID)
	if !connected || conn == nil {
		return
	}
	if time.Since(conn.ConnectedAt) < autoWakeStuckThreshold {
		return
	}

	// Gate 2 — retry cap. Read off the stuck row itself, atomically
	// updated by IncrementInteractionAutoWakeCount on prior attempts.
	if stuck.AutoWakeCount >= autoWakeMaxRetries {
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
			Int("retries_attempted", stuck.AutoWakeCount).
			Msg("⚠️ [AUTO_WAKE] Exhausted retries — marked stuck interaction as error")
		return
	}

	// Need session metadata for the WS command.
	session, err := apiServer.Store.GetSession(ctx, stuck.SessionID)
	if err != nil {
		log.Warn().Err(err).
			Str("interaction_id", stuck.ID).
			Msg("[AUTO_WAKE] Failed to load session; skipping")
		return
	}

	// Skip if the stuck interaction has no prompt content to re-send.
	// Pathological case (synthetic interactions, bad data) — don't try
	// to invent a wake-up payload.
	if stuck.PromptMessage == "" {
		return
	}

	// Bump the counter *first* via a targeted column UPDATE. If the
	// send below fails after this, the next scan tick will see the
	// higher count and either retry once more or exhaust — strictly
	// monotonic, no double-bump risk.
	newCount, err := apiServer.Store.IncrementInteractionAutoWakeCount(ctx, stuck.ID)
	if err != nil {
		log.Warn().Err(err).
			Str("interaction_id", stuck.ID).
			Msg("[AUTO_WAKE] Failed to increment auto_wake_count; skipping send")
		return
	}

	requestID := "autowake_" + system.GenerateUUID()

	// Route any response that arrives for this request_id back to the
	// stuck interaction (rather than letting Helix's fallback matcher
	// invent some other binding). Note: even without this, the
	// fallback would still find X as "most recent waiting" since we
	// don't create a new interaction. This is belt-and-braces.
	apiServer.contextMappingsMutex.Lock()
	if apiServer.requestToInteractionMapping == nil {
		apiServer.requestToInteractionMapping = make(map[string]string)
	}
	apiServer.requestToInteractionMapping[requestID] = stuck.ID
	apiServer.contextMappingsMutex.Unlock()

	var acpThreadID interface{} = nil
	if session.Metadata.ZedThreadID != "" {
		acpThreadID = session.Metadata.ZedThreadID
	}
	agentName := apiServer.getAgentNameForSession(ctx, session)

	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"message":       stuck.PromptMessage,
			"request_id":    requestID,
			"acp_thread_id": acpThreadID,
			"agent_name":    agentName,
		},
	}

	log.Info().
		Str("stuck_interaction_id", stuck.ID).
		Str("session_id", stuck.SessionID).
		Int("attempt", newCount).
		Time("stuck_created_at", stuck.Created).
		Str("request_id", requestID).
		Msg("🔔 [AUTO_WAKE] Re-sending stuck interaction's prompt to unstick session — upstream ACP claude-agent-acp #551 / agent-client-protocol #554")

	if err := apiServer.sendCommandToExternalAgent(stuck.SessionID, command); err != nil {
		log.Warn().Err(err).
			Str("stuck_interaction_id", stuck.ID).
			Str("session_id", stuck.SessionID).
			Msg("[AUTO_WAKE] sendCommandToExternalAgent failed (will retry on next tick if still stuck)")
		return
	}

	log.Info().
		Str("stuck_interaction_id", stuck.ID).
		Int("attempt", newCount).
		Msg("✅ [AUTO_WAKE] Wake-up sent in-place")
}
