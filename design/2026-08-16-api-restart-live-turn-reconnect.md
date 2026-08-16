# RCA: API restart kills a live agent turn (chat dies, agent keeps working)

Session: `ses_01m02zsryp9kqthe9zb2ttmtxw` (spec task `spt_01m02zsrxf305q84txzms77261`,
runtime `opencode`, Zed thread `ses_ff5d518a1ffeRYRSJUjIzIBZtC`).
Interaction: `int_01m052zcj118dtwrt425shkf8b`.

Symptom: after an API restart the sandbox and the coding agent keep running, but the
Helix chat shows `agent turn aborted: the ACP agent process exited mid-turn or hit max
tokens` and never updates again.

## Timeline (from `helix-api-1` logs + Postgres)

| Time (UTC) | Event |
|---|---|
| 10:48:13 | Queue dispatches prompt `continue`. Interaction created `waiting`, `request_id = int_01m052zcj…`, `chat_message` sent to Zed. Streaming works. |
| 10:57:01 | **Air rebuild → API process exits.** External-agent WS drops. Zed/opencode keep running. |
| 10:57:16 | Zed reconnects. `pickupWaitingInteraction` finds the still-`waiting` interaction and **re-sends the same prompt** ("Queued initial chat_message for Zed"). |
| 10:57:50, 11:02:20, 11:02:56, 11:03:01 | Four more restarts, four more duplicate deliveries of the *same* prompt into the *same* live turn. |
| 11:03:01 | Zed: `thread_load_error` — `Failed to send follow-up: Internal error: Connection reset by server`. The original turn is still streaming fine at this point (content_length 129 980, 68 entries). |
| ~11:03:35 | `chat_response_error` for `request_id = int_01m052zcj…` → `handleChatResponseError` sets the interaction `state=error`. **The live turn's own interaction is killed by its duplicate's failure.** |
| 11:05:19 | Next reconnect: no `waiting` interaction any more → `has_interaction=false`, `chat_response_error: no mapping or channel`. |
| 11:05:19 → now | Zed keeps streaming `message_added` for the thread. Helix logs `⚠️ No interaction to route assistant message_added … — content dropped` on every chunk, forever. |

DB confirms: `external_agent_dispatched_at = 2026-08-16 11:03:01` — overwritten by the
last duplicate dispatch, i.e. the durable dispatch marker exists but is never used to
suppress redelivery.

## Root causes

### 1. Reconnect cannot tell "never delivered" from "delivered and still running"

`pickupWaitingInteraction` (`api/pkg/server/websocket_external_agent_sync.go:648`) treats
`state == waiting` as "the agent never got this prompt" and re-sends it.

The only in-flight guard, `interactionDispatchClaims`, is **in-memory**, and
`handleExternalAgentSync` deliberately clears it on every connect
(`releaseSessionDispatchClaims`, line 458) precisely so redelivery can happen. So a
process restart guarantees redelivery.

A durable marker *does* exist — `Interaction.ExternalAgentDispatchedAt`
(`api/pkg/types/types.go:90`) — but `MarkInteractionExternalAgentDispatched`
(`api/pkg/store/store_interactions.go:287`) only guards on `state = 'waiting'`; it
neither checks nor refuses when the column is already set. It is used for cancel
semantics only.

`ResetRunningInteractions` correctly exempts `zed_external` sessions from being marked
errored on boot (`store_interactions.go:17`) — that part is right. But leaving the row
`waiting` is exactly what triggers redelivery.

### 2. There is no resume handshake — Helix never asks Zed what it is doing

The sync protocol (`zed/crates/external_websocket_sync/src/types.rs`) has no event that
reports live turn state. `agent_ready` carries `agent_name` + optional `thread_id` only.
`ui_state_response` has `entry_count` but no "generating" flag and is request/response
only (E2E use).

Zed already holds exactly the needed state: `REQUEST_LIFECYCLES`
(`external_websocket_sync.rs:100`) maps `request_id → Queued | Running | Cancelled` and
survives the Helix WS drop because the Zed process does not restart. It is simply never
reported.

### 3. Zed accepts a duplicate `chat_message` for a request it is already running

`start_registered_request` (`external_websocket_sync.rs:147`) on an existing `Running`
entry re-marks it `Running` and calls `start()` anyway.
`handle_follow_up_message` (`thread_service.rs:2302`) then calls `thread.send()` into a
busy ACP session. For opencode that surfaces as
`Connection reset by server` / `Cannot connect to API`.

There is no request_id idempotency anywhere on the Zed ingress path.

### 4. A failed *duplicate* delivery aborts the *live* turn

Because the duplicate carries the same `request_id`, the resulting
`chat_response_error` routes to the original interaction.
`handleChatResponseError` (`websocket_external_agent_sync.go:4416`) sets
`state = error` unconditionally — no check for "content is actively streaming into this
interaction right now". `handleThreadLoadError` (line 4173) does the same via a
"newest waiting interaction" scan.

### 5. Terminal state is a one-way door — live output is dropped forever

`getOrCreateStreamingContext` (line 1855) has exactly one restart-recovery rule: resurrect
the last interaction only if `state == error && Error == "Interrupted"` (the
`ResetRunningInteractions` marker). Our row is `error` with the generic ACP abort text, so
it does not qualify. Every subsequent `message_added` hits the "content dropped" branch at
line 1729 — even though the `request_id` on the incoming message *exactly matches*
`interaction.external_agent_request_id`.

This is why the chat is permanently dead while the agent visibly keeps working: not a
routing failure, a refusal to route.

### 6. No dampening on a restart storm

Five reconnects in eight minutes produced five duplicate prompt deliveries. Each also
injected a stray `continue` into the agent's conversation context. In production a deploy
is one restart, so this is an amplifier rather than a cause — but it is what made the
failure certain here.

## Scope note

Sandbox/desktop reconnect is **not** broken. `Recovered container from sandbox
discovery` and `Stream WebSocket connection established, starting resilient proxy` both
fire correctly after each restart, and the desktop view kept rendering. The defect is
confined to agent-turn continuity.

## Plan

Ordered by leverage. Phases 1 and 2 are independently shippable; each removes a distinct
failure mode.

### Phase 1 — Stop killing live turns (Helix-only, no Zed change, no version bump)

1. **Suppress redelivery of an already-dispatched turn.**
   In `pickupWaitingInteraction`, skip the `chat_message` when
   `ExternalAgentDispatchedAt != nil` and the session has a `ZedThreadID`. Still restore
   `requestToSessionMapping` / `requestToInteractionMapping`, still send `open_thread`
   with the correlated `request_id`, still return that `request_id`. Re-attach, don't
   re-prompt.
   Make `MarkInteractionExternalAgentDispatched` refuse when the column is already set
   (add `external_agent_dispatched_at IS NULL` to the WHERE), so the invariant is enforced
   at the store, not only at the call site.

2. **Route by `request_id` before dropping content.**
   In `getOrCreateStreamingContext`, before falling through to "content dropped": if the
   incoming `request_id` matches an interaction's `external_agent_request_id`, adopt that
   interaction regardless of state, resetting `error → waiting` when the agent is
   demonstrably still streaming into it. This alone recovers the session in the report.

3. **Don't let an error abort an actively-streaming interaction.**
   In `handleChatResponseError` and `handleThreadLoadError`, if a streaming context for
   that interaction has received content within the last few seconds, log and drop the
   error instead of marking `state = error`. Keep the existing behaviour for genuinely
   idle turns.

4. **Fix `handleThreadLoadError`'s target selection.** It scans for "newest waiting
   interaction" rather than resolving `request_id → interaction`. Resolve by
   `request_id`, matching `handleChatResponseError`.

**Verification:** live spec task with a connected Zed, prompt mid-turn, `docker compose
restart api` (and a second restart 30 s later), confirm — no duplicate `chat_message` in
logs, interaction stays `waiting`, streaming resumes in the UI, turn completes. Then
exercise the *next* operation: send a follow-up and confirm it lands.

### Phase 2 — Resume handshake (Zed + Helix; needs `sandbox-versions.txt` bump)

5. **Zed reports live turn state on connect.** Extend `agent_ready` (or add
   `agent_state`) with `active_turns: [{ acp_thread_id, request_id, state }]`, sourced
   from `REQUEST_LIFECYCLES`. This replaces the Phase-1 heuristic with an authoritative
   answer to "is this turn still running?".

6. **Gate the pickup decision on that report.** Defer `pickupWaitingInteraction`'s send
   decision until `agent_ready` arrives (bounded by the existing readiness timeout): if
   the waiting interaction's `request_id` appears in `active_turns`, re-attach only;
   otherwise deliver. Removes the `ExternalAgentDispatchedAt` heuristic.

7. **Idempotency on Zed ingress.** `start_registered_request` returns a distinct
   `AlreadyRunning` outcome for a `Running` request_id; the `chat_message` handler acks
   and returns instead of calling `thread.send()`. Defence in depth — this alone would
   have prevented the fatal follow-up.

**Verification:** the 9-phase dockerized E2E (`run_docker_e2e.sh`) plus a new phase that
drops and re-establishes the Helix WS mid-turn and asserts the turn completes exactly
once. Per CLAUDE.md, e2e changes are not done until the full dockerized run is green.

### Phase 3 — Hardening

8. **Reconnect debounce** per session, so a rebuild storm collapses into one
   re-attach decision.

9. **UI honesty.** The RETRY button on a killed-but-live turn currently re-sends the
   prompt into a running agent — the same bug by hand. Once Phase 1 lands, RETRY should
   re-attach when the thread is live.

## Open decisions for review

- **Phase 1 alone, or straight to Phase 2?** Phase 1 is a heuristic
  (`dispatched_at != nil` ⇒ assume alive) and would *not* redeliver a prompt that was
  genuinely lost because Zed itself restarted mid-turn. Phase 2 makes it authoritative.
  Recommendation: ship Phase 1 now (it is strictly better than today and unblocks the
  user), then Phase 2 to remove the guess.
- **Version skew.** A Helix API newer than an already-running Zed in a live sandbox will
  not receive `active_turns`. Phase 2 needs an explicit answer for that window —
  either "treat absent `active_turns` as unknown and fall back to the Phase-1 rule"
  (pragmatic, but a second code path) or "require the bumped Zed" (clean, but strands
  in-flight sandboxes across a deploy).
