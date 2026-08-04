# Implementation Tasks: Fix Stale request_id Dropping message_completed and Wedging Sessions

All work in the sandbox's inner Helix at `http://localhost:8080`. Never touch meta.

## Phase 1 — Reproduce before changing anything

- [ ] Create a spec task so a live `claude_code` / `zed_external` agent connects (a bare `agent_type=zed_external` chat session never connects one)
- [ ] Drive the trigger: send prompt → interrupt → send another → interrupt → send a third, in quick succession; let the third run to completion and send nothing afterwards
- [ ] Confirm in the API log: `message_completed` carrying the first turn's `request_id`, and the consumed-mapping WARN at `websocket_external_agent_sync.go:2722`
- [ ] Confirm with `psql`: interaction `state=waiting` with a full `response_message`, and `sessions.config->>'external_agent_status' = 'running'`
- [ ] Confirm the ~2s "Session is busy (interaction waiting)" loop with a queued prompt undeliverable
- [ ] Read `~/.local/share/zed/logs/Zed.log` in the container and confirm (or refute) the Defect B hypothesis in design.md §1 — that `last_completed_request_id` is never written on the interrupt path, freezing `turn_request_id` on turn 1

## Phase 2 — Helix: shared turn resolver

- [ ] Extract `resolveTurnTarget(threadID, requestID)` implementing the 4-step ladder in design.md §2 (mapping → streaming context → DB most-recent-waiting → unroutable)
- [ ] Make `handleMessageAdded` and `handleMessageCompleted` both use it, removing the recover-to-waiting asymmetry between content and completion routing
- [ ] Delete the `mappingConsumed` early return at `websocket_external_agent_sync.go:2718-2724`; the DB fallback (~L2734) must become reachable
- [ ] Reimplement duplicate suppression as "has a completion already been applied to this interaction?" — suppress when the resolved interaction is already `complete`/`interrupted`/`error`
- [ ] Guard against the 2026-04-28 case: do not complete a waiting interaction the agent is still actively streaming into (use the streaming context's activity timestamp)
- [ ] Use the existing `MarkInteractionCompleteIfWaiting` guarded transition rather than read-modify-write
- [ ] Ensure `signalExternalAgentResponseDone` and `publishInteractionUpdateToFrontend` still fire on every path

## Phase 3 — Zed: stop echoing a dead turn's id

- [ ] Make turn rotation fire on turn end by **any** means, including `turn_cancelled`/interrupt, not only on `message_completed`
- [ ] Move turn-boundary rotation ahead of the `is_external_originated_entry` early return at `thread_service.rs:901` (or rotate in the `chat_message` handler) so Helix-originated prompts rotate the turn id
- [ ] Keep the existing protection: a follow-up message overwriting the global map mid-turn must not poison the in-flight turn's id
- [ ] Verify in `Zed.log` against the Phase 1 repro that `message_completed` and `message_added` now carry the current turn's `request_id`

## Phase 4 — Backstop (probe, never a blind timer)

- [ ] Resolve Open Question 3 with the user: reuse `cancel_current_turn`→`noop` as the probe, or add a dedicated read-only `turn_status` request/response to the sync protocol
- [ ] Add the probe path to `auto_wake_stuck_interactions.go`, covering the gap it and `desktopResumeReapStaleThreshold` both decline (agent connected + completion dropped)
- [ ] Gate the probe on "waiting interaction AND no agent activity for the idle threshold", using Zed's `touch_activity` proof-of-life (any event), not token flow alone
- [ ] On `status == "noop"`, settle the waiting interaction and WARN with session, interaction and both request_ids
- [ ] On `status == "cancelled"` or no answer, leave the interaction waiting — never complete a turn the agent says is live

## Phase 5 — Surface it

- [ ] WARN-level log whenever Helix has a `waiting` interaction and the agent reports no turn running
- [ ] Publish the interaction update to the frontend so the spinner clears instead of lying
- [ ] Confirm with the user whether an explicit frontend banner is in scope (Open Question 6)

## Phase 6 — Tests

- [ ] Add to `websocket_external_agent_sync_test.go` (`WebSocketSyncSuite`): completion with a stale `request_id` while another interaction is waiting → the waiting interaction completes
- [ ] Add: genuine duplicate completion is still suppressed (no double-complete, no premature completion of a live turn)
- [ ] Add: a completion arriving mid-stream for an actively-streaming interaction is deferred, not applied
- [ ] Unit-test the backstop's decision function across the live/idle/noop/no-answer matrix
- [ ] Run the Phase 1 repro end-to-end and confirm green: turn completes, session leaves `running`, queued prompt is delivered
- [ ] **Test the next operation**: after the completion lands, send another message and confirm it is delivered *and answered*
- [ ] Prove a long silent tool call is not prematurely completed by the backstop
- [ ] **Mandatory if the Zed WebSocket sync was touched**: run `crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` — "compiles" is not acceptable evidence here

## Phase 7 — Ship

- [ ] Write `design/2026-08-04-message-completed-stale-request-id-wedge.md`, including why `2186abcda`'s approach did not hold and why the 2026-04-28 sentinel could not simply be removed
- [ ] Commit in the Zed repo and capture `git rev-parse HEAD`
- [ ] Bump `ZED_COMMIT` in `sandbox-versions.txt` (currently `1bac4bf841140cf562da9ac680beb4cc0338b0bc`)
- [ ] **Open the Helix PR before pushing the Zed branch**
- [ ] Merge Zed first, then Helix
- [ ] Report the full PR URL against `helixml/helix` in the summary
