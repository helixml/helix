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

## What was implemented

### The resume protocol (`api/pkg/server/external_agent_resume.go`, new)

Reconnect is now two steps instead of one. `resolveWaitingInteraction` correlates the
waiting turn to the new connection — request_id maps, the durable
`ExternalAgentRequestID` binding, the dispatch claim — and sends nothing.
`applyResumeDecision` runs later, once the agent has had its say, and is the only path
that may send a `chat_message` on reconnect.

`decideResume` is the single gate:

| Agent report | Turn state | Verdict |
|---|---|---|
| names this request_id (`queued` or `running`) | any | **attach** — routing only |
| reported, does not name it | any | **deliver** |
| absent (legacy agent / readiness timeout) | already dispatched | **attach + verify** |
| absent | never dispatched | **deliver** |

`pickupWaitingInteraction` is gone; the agent-switch handoff calls
`deliverWaitingInteractionNow`, which is the same decision with no report available (its
interaction was created moments earlier, so it resolves to *deliver*).

### The authoritative handshake (Zed)

`REQUEST_LIFECYCLES` now stores `TrackedRequest { lifecycle, acp_thread_id }` and
`active_turns_snapshot()` renders it as the `active_turns` array on `agent_ready`.
`Cancelled` turns are deliberately not reported — those are turns Helix must be free to
replace. The field is **always serialized**, because absence (an older agent) and an
empty array (an agent that is running nothing) mean opposite things.

### Two agent_ready emitters, one shape

Zed emits `agent_ready` from two places: `send_agent_ready()` after the thread service
loads a thread, and a 5-second fallback timer in the connection loop for when no
`open_thread` arrives. The fallback hand-rolled its JSON rather than going through
`SyncEvent::AgentReady`, so instrumenting only the typed path left it silently missing
`active_turns` — and since Helix reads an ABSENT field as "this agent cannot report", a
current Zed would have been indistinguishable from an old one on exactly the connect where
no thread was loaded, pinning the compat branch on forever.

The fallback now builds the same typed event. A hand-built duplicate of a typed protocol
event is the bug, not the missing field; this was found by running the E2E, not by any
unit test.

### Idempotency at the Zed ingress

`register_queued_request` returns `RequestAdmission::{Accepted, AlreadyActive}`.
`request_thread_creation` drops an `AlreadyActive` request instead of calling
`thread.send()` into a live ACP session. This holds regardless of what Helix decides, so
an old Helix talking to a new Zed is also protected.

### Errors no longer kill live turns (`api/pkg/server/external_agent_turn_error.go`, new)

An error is keyed by request_id, and a request_id names a *turn*, not a *delivery
attempt* — so a rejected duplicate reports against the turn it duplicated.
`applyTurnError` therefore decides on evidence rather than message text: if content is
still flowing into the interaction, the error is deferred one `liveTurnEvidenceWindow`
and re-examined. A turn that genuinely aborted goes quiet and the error lands seconds
later; a turn that is alive keeps streaming and the error is discarded.

`thread_load_error` is stricter still — it is by definition a *delivery* failure, so
against a streaming turn it is dropped outright rather than deferred.

### Turns are resolved durably, and a live thread can un-stick a dead interaction

`interactionForRequest` falls back from the in-memory map (which does not survive a
restart) to `GetInteractionByExternalAgentRequestID`. `handleThreadLoadError` now selects
its target this way instead of scanning for the newest waiting interaction.

`getOrCreateStreamingContext` gained the same durable lookup, and revives an interaction
that is in `error` while its agent is still streaming that request_id — the recovery that
would have saved the session in this report.

The revive is deliberately narrow, because Zed replays thread history as `message_added`
on `open_thread` and those replays carry the thread's current request_id. Reviving on a
replay would resurrect a dead turn into a spinner that never resolves. Three conditions,
all required:

- state is `error` — `complete` is a legitimate terminal state and is never touched;
- `Completed` is zero — every *deliberate* terminal decision (`message_completed`,
  `turn_cancelled`, `thread_load_error`) stamps it, so only a turn killed mid-flight
  without a terminal handshake qualifies, which is exactly the wrongly-errored case;
- the recorded error is not a known agent crash — if the process died, the turn is over
  and a replayed entry is not evidence otherwise.

### Selecting a turn from an event

Anything carrying a request_id resolves through `interactionForRequest`. The one
exception is an **uncorrelated** `thread_load_error` (empty request_id — Zed could not tie
an `open_thread` failure to a turn), which has no key to select by and keeps the
newest-waiting behaviour via `newestWaitingInteraction`. Using newest-waiting for a
*correlated* event is how an unrelated turn ends up wearing another turn's failure, so the
two selectors are named and separated rather than blended.

## Test coverage

| Test | Pins |
|---|---|
| `TestDecideResume` (10 cases) | the full verdict table, including that an *empty* report delivers while an *absent* one does not |
| `TestParseAgentTurnReport` | absent ≠ empty; malformed entries skipped without losing authority |
| `TestResumeOnReconnect_*` (3, real WebSocket via the production handler) | reconnect with a dispatched `waiting` turn: owns → no resend; empty report → resend onto the existing thread; legacy → no resend |
| `TestStreamingContext_RevivesErroredTurnStillStreaming` | the incident's recovery |
| `TestStreamingContext_DoesNotReviveConcludedOrCrashedTurns` | the two revive guards |
| Rust `duplicate_ingress_is_rejected_while_active` | ingress idempotency, and that a finished turn's id is admissible again |
| Rust `active_turns_snapshot_reports_queued_and_running_only` | cancelled turns are not reported as owned |

The three WebSocket tests were checked against a deliberately reverted `decideResume`
(always-deliver): the "owns" and "legacy" cases fail, the "empty report" case still
passes. They pin the behaviour, not the implementation.

## Backwards compatibility, and its removal

The legacy branch is one `if` in `decideResume` plus `verifyResumedTurn`, the bounded
probe that corrects it. It exists only for the deploy window where a new API reconnects
to an in-flight sandbox running an older Zed. Tracked for deletion in
<https://github.com/helixml/helix/issues/3047>.

The probe waits `autoWakeStuckThreshold()` (default 180 s, the same budget the auto-wake
worker uses, because a turn inside a long tool call emits nothing while the tool runs)
and re-delivers only if *every* signal still says the turn is dead: never left `waiting`,
no content ever reached the frontend, no cancellation, and the deciding connection is
still current.

Note the asymmetry that justifies preferring attach when we cannot tell: a turn wrongly
assumed alive is corrected by the probe minutes later, whereas a turn wrongly re-sent
corrupts a live ACP session immediately — and the auto-wake worker does **not** replay
into a connected agent, so it is not a safety net for the delivering mistake.

## Known limitation this does not close

If a turn *finishes* while the API is down, its `message_completed` is written to a dead
socket and lost. The row stays `waiting`, the agent no longer owns the request_id, and the
reconnect correctly concludes "the agent does not have this turn" — so it re-sends the
prompt and the agent redoes the work. That was also the old behaviour (the compat path
just delays it by the probe budget), so this is not a regression, but it is the next thing
worth fixing. Closing it needs Zed to replay terminal turn state on reconnect, not just
thread entries.

## Remaining work

- **Ship the Zed side.** `active_turns` needs `./stack build-zed` → `build-ubuntu`, then a
  `ZED_COMMIT` bump in `sandbox-versions.txt` following the ordering rule in CLAUDE.md
  (commit Zed locally → bump the hash → open the Helix PR → push/merge Zed → merge Helix).
  Until that lands, every reconnect takes the compat branch.
- **E2E: codex lane 17/17 PASSED** with `E2E_AGENTS=codex`, `E2E_MODEL_PROVIDER=openai`,
  `E2E_CODEX_MODEL=gpt-5.6-terra`. That covers Phase 12 (reconnect), Phase 15 (streaming),
  Phase 16 (queue busy-defer), Phase 17 (queue interrupt) and all store-state checks.
  Both `agent_ready` emitters were observed carrying `active_turns` on the wire, populated
  while a turn was in flight and empty otherwise — the handshake verified against live
  traffic rather than mocks.
- **Two zed-agent-lane failures are harness artefacts, not regressions.** Phase 16 and 17
  fail when the native agent is driven from OpenAI. Both wait for the turn to *start*
  streaming and then assume it is still running: Phase 16's turn A completed 4 s in (the
  FAIL is logged in the same second as A's `message_completed`), and Phase 17's turn X
  completed ~3 s in, so it reached `complete` instead of `interrupted`. GPT-5.6 with
  `reasoning_effort` forced to `none` — mandatory on `/v1/chat/completions` with function
  tools — answers in one burst. Reproduced identically on `gpt-5.6-luna` and
  `gpt-5.6-terra`, so it is not model choice within the family. Decisive evidence it is
  not this change: phases 16/17 exercise the **agent-agnostic** production queue path, and
  the codex lane passes them with the same Helix build.
- **CI settings.** Use `gpt-5.6-terra`, not `gpt-5.6-luna[low]` — luna's bursty streaming
  trips Phase 15's cadence limits (29.8 s gap against a 20 s budget; 95 % of content in
  the final 20 % of stream time against a 90 % limit). The `claude` lane needs a live
  `ANTHROPIC_API_KEY`; the harness injects a key into the agent env and has no
  subscription/OAuth path.
- **E2E coverage to add once it can run.** A phase that drops and re-establishes the Helix
  WebSocket mid-turn and asserts the turn completes exactly once, with no duplicate
  `chat_message`. Per CLAUDE.md, adding it is not done until the full dockerized run has
  actually been executed green.
- **Reconnect debounce.** Not required for correctness now that redelivery is gated, but a
  rebuild storm still produces one resolve/`open_thread` per reconnect.
- **UI RETRY on a live turn.** The button re-sends the prompt by hand, which is the
  original bug performed manually. It should re-attach when the thread is live.
- **Dead code.** `SessionReadinessState.NeedsContinue` is hardcoded `false`, making
  `sendContinuePromptIfNeeded` unreachable. Left alone to keep this change scoped; noted
  in <https://github.com/helixml/helix/issues/3047>.
