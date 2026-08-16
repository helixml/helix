// Auto-wake worker for stuck `state=waiting` interactions.
//
// Sessions without an external-agent WebSocket may be stuck because the dev
// container never started, so this worker retries the container auto-start.
// Once an agent is connected, replaying the original prompt is unsafe because
// the agent may already be processing it. The worker leaves connected
// interactions waiting so a late completion can still settle them.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// setupFailedSentinelPath is where helix-workspace-setup.sh writes a JSON
// failure report on non-zero exit. Reading it from outside the container
// lets us replace the generic "agent never connected" banner with the real
// reason setup failed (e.g. a git clone 403, a missing dependency).
const setupFailedSentinelPath = "/home/retro/.helix-setup-failed"

// setupFailedSentinel matches the JSON shape written by
// helix-workspace-setup.sh's cleanup_and_prompt trap.
type setupFailedSentinel struct {
	ExitCode int    `json:"exit_code"`
	LogTail  string `json:"log_tail"`
}

// readSetupFailureSentinel pulls ~/.helix-setup-failed from the session's
// dev container via hydra and returns its parsed contents. Returns nil if
// the file isn't present, the container or sandbox aren't reachable, or the
// JSON is malformed — every code path the caller hits when the sentinel
// can't be read should fall back to the generic banner.
func (apiServer *HelixAPIServer) readSetupFailureSentinel(ctx context.Context, sessionID, sandboxID string) *setupFailedSentinel {
	if sandboxID == "" {
		return nil
	}
	hydraRunnerID := fmt.Sprintf("hydra-%s", sandboxID)
	hydraClient := hydra.NewRevDialClient(apiServer.connman, hydraRunnerID)

	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	data, err := hydraClient.ReadSandboxFile(ctxTimeout, sessionID, setupFailedSentinelPath)
	if err != nil {
		log.Debug().Err(err).
			Str("session_id", sessionID).
			Str("sandbox_id", sandboxID).
			Msg("[AUTO_WAKE] No setup-failed sentinel found (or unreadable) — falling back to generic banner")
		return nil
	}
	var parsed setupFailedSentinel
	if err := json.Unmarshal(data, &parsed); err != nil {
		log.Warn().Err(err).
			Str("session_id", sessionID).
			Int("bytes", len(data)).
			Msg("[AUTO_WAKE] Setup-failed sentinel present but unparseable; falling back to generic banner")
		return nil
	}
	return &parsed
}

const (
	// autoWakeScanInterval is how often the worker checks for stuck
	// interactions. Tighter than this just wastes DB queries; looser
	// leaves the user staring at a frozen chat for longer than needed.
	autoWakeScanInterval = 10 * time.Second

	// defaultAutoWakeStuckThreshold is the minimum age of a
	// `state=waiting` interaction before we consider it stuck.
	//
	// 180 s targets the dominant failure mode: agent emits a few early
	// chunks (tool_call, thinking) then goes silent for minutes while
	// claude-agent-acp buffers the rest of the turn on its outbound
	// channel. The streaming-context gate below requires `lastPublish`
	// to be older than this same threshold before considering the
	// session quiescent.
	//
	// Why 180 s and not 60 s (the previous default):
	//
	// The agent emits ACP `session/update` events around tool calls,
	// not during them. A single long synchronous tool — `git push`
	// over a slow network, `npm install`, `gh pr view` on a chatty PR,
	// `find /` over a large tree — produces zero streamed events for
	// the entire duration. With a 60 s threshold the gate decayed
	// past the cutoff during a normal ~90 s tool call, the worker
	// fired, and the agent's mid-flight turn was interrupted by an
	// unnecessary re-prompt. 180 s covers the realistic envelope of
	// common synchronous tools with a 3× safety margin on the
	// empirically-observed ~61 s gap.
	//
	// This is defence in depth. The load-bearing fix is at the org
	// layer: the activation spawner no longer releases its per-Worker
	// serialisation lane on a stale 5-min `ActivationTimeout`, so
	// long-running healthy sessions no longer spawn a "decoy" empty
	// `state=waiting` interaction on top of themselves. With that fix
	// in place, the SQL filter has nothing to match on a healthy
	// session — this threshold only matters for genuinely stuck rows.
	//
	// After this threshold, automatic replay remains suppressed for a
	// connected interaction because the agent may still be processing it.
	// The row stays waiting so a late completion can still settle it.
	//
	// Override at runtime with HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS.
	defaultAutoWakeStuckThreshold = 180 * time.Second

	// autoWakeMaxRetries caps how many cold-start kicks we attempt for a
	// session without a WebSocket before marking the interaction state=error.
	autoWakeMaxRetries = 2

	// defaultColdStartGracePeriod is how long we wait for an in-flight
	// container boot to bring up the dev container + Zed + claude-agent-acp
	// before we count the wait against the cold-start retry budget.
	//
	// While the container boot is in progress, re-kicking
	// `autoStartDevContainerForSession` is a no-op (StartDesktop holds a
	// per-session lock and short-circuits with "Dev container already
	// running" once the container is up) but still increments
	// AutoWakeCount on every scan tick. With autoWakeMaxRetries=2 and
	// ~10s ticks the budget burned in <90s, while a cold helix-ubuntu
	// boot routinely takes 90–150s. Result: `state=error` ("Agent never
	// connected after auto-wake cold-start retries") fired ~30s before
	// WS actually connected.
	//
	// "Container boot in progress" means `ExternalAgentStatus` in
	// {"starting", "running"}. StartDesktop flips status to "running" the
	// moment desktop-bridge is reachable (~T+25s on cold boot) — long
	// before Zed inside the container has finished initialising GNOME,
	// launched claude-agent-acp, and dialled the external-agent
	// WebSocket back to the API (typically T+90–120s). The gate has to
	// cover both substates or it engages too early to matter.
	//
	// Witnessed:
	//   - spt_01kreb7sevt5ecyagxhctv3ejh: container created T+18s,
	//     retries exhausted T+93s, WS connected T+123s.
	//   - spt_01ktnvz9y1grjqaaa1rq72z5tx: container + bridge ready T+25s
	//     (status→"running"), retries exhausted T+89s, WS connected
	//     T+98s. The original "starting"-only gate never engaged because
	//     status flipped to "running" 64s before the budget was burned.
	//
	// 5 min covers the realistic boot envelope (ZFS clone of large
	// snapshots, golden cache unpack, GNOME + Zed init, claude-agent-acp
	// dial) with margin. After this we fall through to the normal
	// retry-and-error path so genuinely-failed boots don't churn forever.
	//
	// Override at runtime with HELIX_COLD_START_GRACE_SECONDS.
	defaultColdStartGracePeriod = 5 * time.Minute
)

// autoWakeStuckThreshold returns the configured stuck threshold.
// Reads HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS once per call so
// operators can tune live without redeploying (the worker reads it
// every scan tick, ~10 s).
func autoWakeStuckThreshold() time.Duration {
	if raw := os.Getenv("HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultAutoWakeStuckThreshold
}

// coldStartGracePeriod returns how long an in-flight `StartDesktop` is
// allowed to run before we count it against the cold-start retry budget.
// Reads HELIX_COLD_START_GRACE_SECONDS so operators can dial it up on
// slow disks (large ZFS clones, cold golden caches) without redeploying.
func coldStartGracePeriod() time.Duration {
	if raw := os.Getenv("HELIX_COLD_START_GRACE_SECONDS"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultColdStartGracePeriod
}

// startAutoWakeStuckInteractionsWorker launches the periodic scanner.
// Idempotent in spirit but the API server only calls it once at init.
func (apiServer *HelixAPIServer) startAutoWakeStuckInteractionsWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(autoWakeScanInterval)
		defer ticker.Stop()

		log.Info().
			Dur("scan_interval", autoWakeScanInterval).
			Dur("stuck_threshold", autoWakeStuckThreshold()).
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
	cutoff := time.Now().Add(-autoWakeStuckThreshold())

	stuck, err := apiServer.Store.ListStuckWaitingInteractions(ctx, cutoff, 50)
	if err != nil {
		log.Warn().Err(err).Msg("[AUTO_WAKE] Failed to query stuck interactions")
		return
	}

	for _, interaction := range stuck {
		apiServer.maybeAutoWake(ctx, interaction)
	}
}

// maybeAutoWake handles a single stuck interaction after the safety gates pass.
func (apiServer *HelixAPIServer) maybeAutoWake(ctx context.Context, stuck *types.Interaction) {
	threshold := autoWakeStuckThreshold()

	// Gate 1 — WebSocket connection AND grace period since connect.
	//
	// If the agent isn't connected yet (desktop still booting after a
	// fresh session start, or after a reconnect) or has dropped,
	// sending a chat_message wake-up is pointless — there's no peer to
	// receive it.
	//
	// Special case (helixml/helix#2397): some sessions enter this state
	// because nothing ever woke the dev container in the first place —
	// e.g. a session created via POST /sessions/chat that errored on
	// first dispatch, leaving the container running but Zed inside
	// never connected the WebSocket. For these, sending a wake-up
	// message is hopeless; what we need is to (re)kick the container
	// auto-start so Zed dials home. Bound by autoWakeMaxRetries via the
	// existing AutoWakeCount column so a permanently-broken session
	// doesn't churn forever.
	//
	// Also: even when the connection has just come up, the agent may
	// be processing a prompt that was queued during boot — give it the
	// same grace period (autoWakeStuckThreshold) before deciding it's
	// stuck. This prevents the worker from firing on a 5-minute-old
	// interaction that the agent only started processing 5 seconds ago
	// because the WS reconnect just completed.
	conn, connected := apiServer.externalAgentWSManager.getConnection(stuck.SessionID)
	if !connected || conn == nil {
		// Only consider a re-kick once the interaction is genuinely old.
		// `created < now - threshold` was already enforced at the SQL
		// level by ListStuckWaitingInteractions, but recheck defensively.
		if time.Since(stuck.Created) < threshold {
			return
		}
		apiServer.maybeKickColdStart(ctx, stuck)
		return
	}

	// The "stuck enough to wake" clock should anchor on the most
	// recent of:
	//   - When the WebSocket connected (agent only able to receive
	//     prompts after this)
	//   - When the session last had activity (a chat_message went out,
	//     a message_completed came back — both call TouchSession and
	//     bump session.updated)
	//
	// This handles three failure modes the connection-only gate misses:
	//
	//   - User sends a follow-up message while X is stuck: session.updated
	//     refreshes; we shouldn't wake X immediately because the user is
	//     already manually doing what we'd do.
	//   - A previous turn completed recently (message_completed bumps
	//     session.updated): the agent was demonstrably alive
	//     `threshold` seconds ago; X probably isn't stuck yet, just
	//     queued behind that completion's flush.
	//   - WS reconnected long ago but the agent has been chatty since
	//     (each turn touches session.updated): no need to wake on age
	//     of a single old-but-still-processing prompt.
	session, err := apiServer.Store.GetSession(ctx, stuck.SessionID)
	if err != nil {
		log.Warn().Err(err).
			Str("interaction_id", stuck.ID).
			Msg("[AUTO_WAKE] Failed to load session for activity anchor; skipping")
		return
	}
	anchor := conn.ConnectedAt
	if session.Updated.After(anchor) {
		anchor = session.Updated
	}
	if time.Since(anchor) < threshold {
		return
	}

	// Gate 1b — quiescence-aware streaming-context check.
	//
	// `getOrCreateStreamingContext` creates an entry on the first
	// content-bearing event of a turn (text chunk, thinking, tool_call)
	// and `flushAndClearStreamingContext` only removes it on
	// `message_completed` or a fresh WS reconnect. So an active context
	// is *not* proof of recent activity — when the wrapper buffers the
	// tail of a turn (the bug this worker exists to mitigate) the
	// context lives on for minutes after the last visible chunk, with
	// no `message_completed` ever arriving. Skipping unconditionally on
	// "context exists" defeats the whole worker for the dominant
	// failure mode.
	//
	// Instead: skip only if the context exists AND its `lastPublish`
	// (the most recent time we forwarded a chunk to the frontend) is
	// within `threshold`. After `threshold` of in-context silence we
	// treat the session as quiescent for replay-suppression logging.
	//
	// Caveat — this gate cannot see *inside* a long synchronous tool
	// call. The agent emits ACP `session/update` events around tool
	// calls (assistant text → tool_call → tool_result → assistant
	// text), not during them. A cascade of many short tools touches
	// `lastPublish` on every event so the gate stays above the
	// threshold reliably. But a *single* tool that runs for longer
	// than `threshold` — `git push` over a slow network, `npm install`,
	// a long `find` — produces no streamed events while it runs, so
	// `lastPublish` decays past the cutoff and this gate stops
	// protecting against false-positives. The 180 s default is
	// calibrated to cover common slow-tool durations. The actual fix
	// for the underlying problem (a decoy `state=waiting` row spawned
	// on top of a still-running session when the org-layer activation
	// timeout fired) lives in the org-layer spawner, not here — see
	// `api/pkg/org/infrastructure/runtime/helix/spawner.go`.
	apiServer.streamingContextsMu.RLock()
	sctx := apiServer.streamingContexts[stuck.SessionID]
	apiServer.streamingContextsMu.RUnlock()
	if sctx != nil {
		sctx.mu.Lock()
		lastPublish := sctx.lastPublish
		sctx.mu.Unlock()
		if time.Since(lastPublish) < threshold {
			return
		}
	}

	// Same threshold, computed against interaction age now that we know
	// the connection has been live long enough and there's no active
	// streaming on the session. Caller has already filtered on
	// `created < now - autoWakeStuckThreshold` at the SQL level using
	// the previous threshold value, but recheck here defensively in
	// case the env var was bumped UP between the SQL filter and now.
	if time.Since(stuck.Created) < threshold {
		return
	}

	log.Debug().
		Str("interaction_id", stuck.ID).
		Str("session_id", stuck.SessionID).
		Msg("[AUTO_WAKE] Connected agent remained quiescent; automatic prompt replay suppressed")
}

// maybeKickColdStart handles stuck interactions on sessions with no live
// WebSocket. Re-running the dev container auto-start is the only thing that
// can make Zed dial home — sending a chat_message has no peer to deliver to.
// Bounded by autoWakeMaxRetries via the AutoWakeCount column so a session
// that genuinely cannot start doesn't churn forever; after exhaustion the
// stuck interaction is marked state=error.
//
// Why a column UPDATE instead of a Save: the streaming path concurrently
// calls UpdateInteraction (which uses GORM Save and so writes every column
// from its in-memory copy). A Save here would race-clobber AutoWakeCount
// back to a stale value, and the cap would never engage.
//
// Container-state-aware retry budget: before bumping AutoWakeCount we look
// at `session.Metadata.ExternalAgentStatus`. If the container is in any
// active boot substate ("starting" or "running") and the interaction is
// younger than the cold-start grace period, we skip without touching the
// budget — the existing boot will either finish (Zed dials home and
// pickupWaitingInteraction delivers) or trip the StartDesktop hard
// timeout (20 min) and clear the status.
//
// Why "running" counts as "still booting" here: StartDesktop sets status
// to "running" the moment the container exists and desktop-bridge is
// reachable, which on a cold boot is ~T+25s. Zed itself doesn't dial
// the external-agent WebSocket back to the API until GNOME has come up
// and claude-agent-acp has launched (typically T+90–120s). The 60–90s
// gap between "running" and a live WS is the dominant cold-start
// failure mode the grace period exists to cover — gating on "starting"
// alone never engaged for it (see spt_01ktnvz9y1grjqaaa1rq72z5tx).
//
// Re-kicking during this window only races against StartDesktop's
// per-session lock (which short-circuits with "Dev container already
// running") and burns retry budget for nothing.
func (apiServer *HelixAPIServer) maybeKickColdStart(ctx context.Context, stuck *types.Interaction) {
	// Load the session once for the two checks below: the
	// StartDesktop-in-progress gate, and (on cap-exhaustion) the
	// SandboxID lookup used to read the workspace-setup failure
	// sentinel. Failing to load is non-fatal — fall through so a
	// transient store error doesn't permanently disable cold-start.
	session, err := apiServer.Store.GetSession(ctx, stuck.SessionID)
	if err != nil {
		log.Warn().Err(err).
			Str("interaction_id", stuck.ID).
			Msg("[AUTO_WAKE] Failed to load session for cold-start status check; proceeding with kick")
		session = nil
	}

	// A desktop-quota block is NOT a transient cold-start — retrying can't help,
	// and the generic "agent never connected" banner hides the real reason. If
	// the session's org is at its concurrent-desktop limit (and quotas are
	// enforced), surface the actual limit to the user immediately and stop.
	// Mirrors hydra_executor.checkLimits (gated on EnforceQuotas).
	if session != nil && apiServer.quotaManager != nil {
		if settings, sErr := apiServer.Store.GetSystemSettings(ctx); sErr == nil && settings.EnforceQuotas {
			if resp, qErr := apiServer.quotaManager.LimitReached(ctx, &types.QuotaLimitReachedRequest{
				UserID:         session.Owner,
				OrganizationID: session.OrganizationID,
				Resource:       types.ResourceDesktop,
			}); qErr == nil && resp != nil && resp.LimitReached {
				reason := fmt.Sprintf("Desktop limit reached (%d). Stop a running desktop session, or raise your organization's concurrent-desktop limit, then retry.", resp.Limit)
				transitioned, uErr := apiServer.Store.MarkInteractionErrorIfWaiting(ctx, stuck.ID, stuck.GenerationID, reason)
				if uErr != nil {
					log.Warn().Err(uErr).Str("interaction_id", stuck.ID).Msg("[AUTO_WAKE] Failed to mark interaction errored for desktop limit")
					return
				}
				if !transitioned {
					log.Debug().Str("interaction_id", stuck.ID).Msg("[AUTO_WAKE] Interaction already transitioned before desktop-limit error")
					return
				}
				if _, clearErr := apiServer.Store.ClearSessionStartingStatus(ctx, stuck.SessionID); clearErr != nil {
					log.Warn().Err(clearErr).Str("session_id", stuck.SessionID).Msg("[AUTO_WAKE] Failed to clear starting status after surfacing desktop limit")
				}
				log.Warn().Str("interaction_id", stuck.ID).Str("session_id", stuck.SessionID).Int("limit", resp.Limit).
					Msg("[AUTO_WAKE] Desktop limit reached — surfaced to user, not retrying cold-start")
				return
			}
		}
	}

	// Skip if a container boot is genuinely in progress and we're still
	// inside the grace period. Both "starting" and "running" count as
	// in-progress here: see the function header for why the post-bridge,
	// pre-WS substate ("running" with no live WS) is the case the grace
	// period most often needs to cover.
	if session != nil &&
		(session.Metadata.ExternalAgentStatus == "starting" ||
			session.Metadata.ExternalAgentStatus == "running") &&
		time.Since(stuck.Created) < coldStartGracePeriod() {
		log.Debug().
			Str("interaction_id", stuck.ID).
			Str("session_id", stuck.SessionID).
			Str("external_agent_status", session.Metadata.ExternalAgentStatus).
			Dur("interaction_age", time.Since(stuck.Created)).
			Dur("grace_period", coldStartGracePeriod()).
			Msg("[AUTO_WAKE] Container boot in progress (no WS yet) — deferring cold-start kick (no budget burn)")
		return
	}

	if stuck.AutoWakeCount >= autoWakeMaxRetries {
		reason := "Agent never connected after auto-wake cold-start retries (no WebSocket — see helixml/helix#2397)"
		// Before giving up with the generic banner, see if the dev
		// container's workspace-setup script wrote a failure sentinel.
		// If it did, the real cause is in there (e.g. a clone 403) and
		// the user deserves to see that instead of an infrastructure
		// timeout. Reuses the session loaded at the top of this
		// function — no extra store roundtrip (and no extra mock
		// expectation in the cap-exhausted unit tests).
		if session != nil {
			if sentinel := apiServer.readSetupFailureSentinel(ctx, stuck.SessionID, session.SandboxID); sentinel != nil {
				// Truncate aggressively: the Interaction.Error column
				// renders inline in the UI. Full log lives in the
				// container's ~/.helix-setup.log for follow-up.
				const maxTail = 1200
				tail := sentinel.LogTail
				if len(tail) > maxTail {
					tail = "…" + tail[len(tail)-maxTail:]
				}
				reason = fmt.Sprintf("Workspace setup failed (exit code %d): %s", sentinel.ExitCode, tail)
				log.Info().
					Str("interaction_id", stuck.ID).
					Str("session_id", stuck.SessionID).
					Int("exit_code", sentinel.ExitCode).
					Msg("[AUTO_WAKE] Found workspace setup failure sentinel")
			}
		}
		transitioned, err := apiServer.Store.MarkInteractionErrorIfWaiting(ctx, stuck.ID, stuck.GenerationID, reason)
		if err != nil {
			log.Warn().Err(err).
				Str("interaction_id", stuck.ID).
				Msg("[AUTO_WAKE] Failed to mark cold-start-exhausted interaction as error")
			return
		}
		if !transitioned {
			log.Debug().Str("interaction_id", stuck.ID).Msg("[AUTO_WAKE] Interaction already transitioned before cold-start exhaustion")
			return
		}
		// Revert any sync-time "starting" mark left behind by
		// syncPromptHistory's markCanonicalSessionStartingForSync. Without
		// this, the spec-task detail page sits on a perpetual
		// "Starting Desktop..." spinner instead of reverting to
		// "Desktop Paused". Targeted JSONB merge gated on
		// status='starting' so we don't clobber a status that hydra has
		// since updated. See spec design/tasks/002047_yet-again-sending-a/.
		if cleared, clearErr := apiServer.Store.ClearSessionStartingStatus(ctx, stuck.SessionID); clearErr != nil {
			log.Warn().Err(clearErr).
				Str("session_id", stuck.SessionID).
				Msg("[AUTO_WAKE] Failed to clear sync-time starting status on cold-start exhaustion")
		} else if cleared {
			log.Info().
				Str("session_id", stuck.SessionID).
				Msg("[AUTO_WAKE] Cleared sync-time starting status after cold-start exhaustion — spinner will return to paused")
		}
		log.Warn().
			Str("interaction_id", stuck.ID).
			Str("session_id", stuck.SessionID).
			Int("retries_attempted", stuck.AutoWakeCount).
			Msg("⚠️ [AUTO_WAKE] Exhausted cold-start retries — marked stuck interaction as error")
		return
	}

	// Bump first via a targeted column UPDATE so the cap engages even
	// if the auto-start fails.
	newCount, err := apiServer.Store.IncrementInteractionAutoWakeCount(ctx, stuck.ID)
	if err != nil {
		log.Warn().Err(err).
			Str("interaction_id", stuck.ID).
			Msg("[AUTO_WAKE] Failed to increment auto_wake_count for cold-start; skipping kick")
		return
	}

	log.Info().
		Str("stuck_interaction_id", stuck.ID).
		Str("session_id", stuck.SessionID).
		Int("attempt", newCount).
		Time("stuck_created_at", stuck.Created).
		Msg("🔌 [AUTO_WAKE] No WS for stuck interaction — kicking dev container auto-start (helixml/helix#2397)")

	go apiServer.autoStartDevContainerForSession(stuck.SessionID)
}
