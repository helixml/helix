# Implementation Tasks: Fix Stale request_id Dropping message_completed and Wedging Sessions

All work in the sandbox's inner Helix at `http://localhost:8080`. Never touch meta.

## Phase 1 — Reproduce before changing anything

- [x] Spec task created and sandbox provisioned with the rebuilt desktop image — but **Zed could not obtain a model** (`configured NativeAgent model did not become available within 15s`), so no live turn was possible. Root cause recorded in design.md
- [x] Trigger driven instead by e2e Phase 9 (rapid 3-turn cancel) + the unit repro, since no live turn was possible
- [x] Consumed-mapping WARN reproduced verbatim by the unit repro against pre-fix code
- [x] Equivalent asserted in the unit repro (waiting interaction with full response never completed)
- [x] Queue-blocked-behind-waiting behaviour covered by e2e Phase 16 (queue busy-defer)
- [x] **Deterministic repro landed as a unit test** — `TestMessageCompleted_StaleRequestID_SettlesWaitingInteraction` reproduces the exact production log line and the dropped completion, without needing the live stack
- [x] **Defect B root cause found — but it is NOT the planning hypothesis.** Confirmed by source analysis (see design.md "Corrected Zed root cause"): `last_completed_request_id` *is* written on the interrupt path. The real freeze is that the `Stopped` fallback updates `last_completed_request_id` to the fallback id but leaves `turn_request_id` on the old one, so the two diverge permanently and the rotation equality can never hold again

## Phase 2 — Helix: shared turn resolver

- [x] Extract `resolveTurnTarget(threadID, requestID)` implementing the 4-step ladder in design.md §2 (mapping → streaming context → DB most-recent-waiting → unroutable)
- [x] Make `handleMessageAdded` and `handleMessageCompleted` both use it, removing the recover-to-waiting asymmetry between content and completion routing
- [x] Delete the `mappingConsumed` early return at `websocket_external_agent_sync.go:2718-2724`; the DB fallback (~L2734) must become reachable
- [x] Reimplement duplicate suppression as "has a completion already been applied to this interaction?" — suppress when the resolved interaction is already `complete`/`interrupted`/`error`
- [x] Guard against the 2026-04-28 case: do not complete a waiting interaction the agent is still actively streaming into (use the streaming context's activity timestamp)
- [ ] Use the existing `MarkInteractionCompleteIfWaiting` guarded transition rather than read-modify-write
- [x] Ensure `signalExternalAgentResponseDone` and `publishInteractionUpdateToFrontend` still fire on every path

## Phase 3 — Zed: stop echoing a dead turn's id

- [x] Make turn rotation fire on turn end by **any** means, including `turn_cancelled`/interrupt, not only on `message_completed`
- [x] ~~Move turn-boundary rotation ahead of the `is_external_originated_entry` early return~~ — **not needed.** The real root cause is the divergence above; the one-line fix keeps `turn_request_id` in lockstep with the id actually reported, which restores rotation for every later turn
- [x] Keep the existing protection: a follow-up message overwriting the global map mid-turn must not poison the in-flight turn's id
- [x] Verify in `Zed.log` against the Phase 1 repro that `message_completed` and `message_added` now carry the current turn's `request_id`

## Phase 4 — Backstop (probe, never a blind timer)

- [x] Resolve Open Question 3 with the user: reuse `cancel_current_turn`→`noop` as the probe, or add a dedicated read-only `turn_status` request/response to the sync protocol
- [x] Add the probe path to `auto_wake_stuck_interactions.go`, covering the gap it and `desktopResumeReapStaleThreshold` both decline (agent connected + completion dropped)
- [x] Gate the probe on "waiting interaction AND no agent activity for the idle threshold", using Zed's `touch_activity` proof-of-life (any event), not token flow alone
- [x] On `status == "noop"`, settle the waiting interaction and WARN with session, interaction and both request_ids
- [x] On `status == "cancelled"` or no answer, leave the interaction waiting — never complete a turn the agent says is live

## Phase 5 — Surface it

- [x] WARN-level log whenever Helix has a `waiting` interaction and the agent reports no turn running
- [x] Publish the interaction update to the frontend so the spinner clears instead of lying
- [ ] Confirm with the user whether an explicit frontend banner is in scope (Open Question 6) — backend WARN + frontend publish shipped; banner deferred

## Phase 6 — Tests

- [x] Add to `websocket_external_agent_sync_test.go` (`WebSocketSyncSuite`): completion with a stale `request_id` while another interaction is waiting → the waiting interaction completes
- [x] Add: genuine duplicate completion is still suppressed (no double-complete, no premature completion of a live turn)
- [x] Add: a completion arriving mid-stream for a live turn is NOT applied (agent-probe based, plus an unanswered-probe fail-closed case)
- [x] Unit-test the backstop's decision function across the live/idle/noop/no-answer matrix
- [x] Repro green at unit level + e2e phases 16/17 (queue busy-defer, queue interrupt) prove the queue drains after completion. **Live spec-task run NOT possible** — inner Helix has no Anthropic models registered, so Zed never gets a model (see design.md)
- [x] **Test the next operation** — e2e Phase 17 sends a follow-up after an interrupt and asserts it is *delivered and completed*; Phase 2/7 assert follow-ups on an existing thread
- [x] Long silent tool call not prematurely completed — proven by `TestMessageCompleted_StaleRequestID_LiveTurnNotCompleted` and `TestDoesNotSettleWhileAgentReportsTurnRunning`: the backstop only ever settles on a definite `running=false`, and a live tool call is a `Generating` thread
- [x] **Mandatory e2e run — DONE and GREEN.** Installed a Rust toolchain + gcc/cmake (absent from this sandbox), rebuilt Zed via `./stack build-zed dev`, copied the binary, ran `run_docker_e2e.sh`: all 17 phases PASSED including Phase 9 "Rapid 3-turn cancel" and Phase 14 "Cancel no-op"

## Phase 7 — Ship

- [x] Write `design/2026-08-04-message-completed-stale-request-id-wedge.md`, including why `2186abcda`'s approach did not hold and why the 2026-04-28 sentinel could not simply be removed
- [x] Commit in the Zed repo and capture `git rev-parse HEAD`
- [x] Bump `ZED_COMMIT` in `sandbox-versions.txt` (currently `1bac4bf841140cf562da9ac680beb4cc0338b0bc`)
- [x] Branches pushed; the Helix platform opens the PRs (per task rules agents must not run `gh pr create`). PR descriptions written per repo
- [ ] Merge Zed first, then Helix (reviewer action — `sandbox-versions.txt` already pins the Zed branch HEAD)
- [x] Report branches/PR info in the summary
