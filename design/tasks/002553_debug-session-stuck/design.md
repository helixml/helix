# Design: Fix Stale request_id Dropping message_completed and Wedging Sessions

## 1. What is actually wrong

Two independent defects compound into the wedge. Both must be fixed; either alone leaves a
hole.

### Defect A — Helix (the drop)

`handleMessageCompleted` treats `request_id` as *authoritative turn identity*:

```go
// websocket_external_agent_sync.go:2700-2724
if mappedID, ok := apiServer.requestToInteractionMapping[messageRequestID]; ok {
    if mappedID == "" {
        mappingConsumed = true          // "stale duplicate"
    } else { ... }
}
...
if mappingConsumed {
    log.Warn()...
    return nil                          // <-- the drop
}
```

The consumed sentinel `""` conflates two very different situations:

| Situation | What it means | Correct action |
|---|---|---|
| Second copy of a completion already applied to interaction X | Genuine duplicate | Suppress |
| Completion whose id is stale, but the **only** completion this turn will get | Mis-tagged, genuine | **Apply to the waiting turn** |

Because both look identical to a single-value map lookup, the second case is discarded —
and the DB fallback ~L2734 that scans for the most recent `waiting` interaction, which
handles this correctly, sits *below* the early return and is unreachable.

`handleMessageAdded` does not have this problem: its chokepoint,
`getOrCreateStreamingContext`, resolves via the thread's session and falls back to the most
recent `Waiting`/restart-interrupted interaction. **That is why the same event burst logged
"No interaction to route assistant message_added … content dropped" while all 182,510 chars
still landed on the waiting interaction.** Content routing has a recover-to-waiting path;
completion routing does not. That asymmetry is the structural bug.

### Defect B — Zed (the stale id) — *hypothesis, confirm against repro*

In `crates/external_websocket_sync/src/thread_service.rs`, `turn_request_id` is per-turn
state that is supposed to rotate at each turn boundary. Three mechanisms rotate it, and in
the interrupt→resend sequence **all three fail**:

1. **User-message rotation (L946-957)** — force-rotates from the global
   `THREAD_REQUEST_MAP` when a `UserMessage` entry appears. Unreachable for Helix-sent
   prompts: `is_external_originated_entry(...) { return; }` at **L901** fires first,
   precisely because Helix-originated entries must not be echoed back.
2. **Assistant rotation (L961-972)** — rotates only when
   `current.is_empty() || current == last_completed_request_id`.
3. **`Stopped` fallback (L1151-1169)** — falls back to the global map only when
   `captured_rid == last_completed`.

Both (2) and (3) are gated on `last_completed_request_id`, which is written **only when a
`message_completed` is actually emitted** (L1171). An *interrupted* turn emits
`turn_cancelled`, not `message_completed` — so `last_completed_request_id` stays `""` while
`turn_request_id` still holds turn 1's id. `current != completed`, so rotation never fires
again. The thread is frozen on the first turn's id for as long as the thread lives.

This matches the evidence exactly: 3 interactions in 42s, only the first ever seen by Zed's
tagger, and a completion 23 minutes later still carrying that first id — and the
`message_added` events in the same burst carrying it too.

> **Verify before fixing.** This came from source reading during planning. Step 1 of the
> task is the deliberate repro; confirm this mechanism in `Zed.log` before committing to it.

### Why `2186abcda` did not hold

That commit replaced a *timing-based* dedup (`completedRequestIDs`, which permanently
blocked any id ever seen) with a *state-based* one. Its own message says Zed reuses the
same `request_id` across turns and that the old dedup "stalled the prompt queue
indefinitely" — the same wedge, in April.

The fix swapped one heuristic keyed on `request_id` for another heuristic keyed on
`request_id`. It narrowed the failure window; it did not remove the assumption that the
agent's echoed id is a reliable turn identity. `design/2026-04-28-stale-request-id-rebind-loses-zed-updates.md`
then closed the opposite hole (rebinding a consumed sentinel prematurely completes live
turns), which is exactly why we cannot simply delete the sentinel now. Both guards are
correct responses to a broken identity model. **The identity model is the thing to fix.**

---

## 2. Key decision: thread-first resolution

**Truth lives in Helix, keyed by thread.** A completion for thread `T` settles whatever
turn Helix currently has in flight on `T`. The agent's echoed `request_id` is a *hint* used
to disambiguate and to detect genuine duplicates — never the sole gate for dropping.

Rejected alternative: **monotonic turn sequence number echoed by the agent.** Cleaner in
principle — it makes staleness explicit and totally ordered rather than inferred. Rejected
*for this change* because it requires a coordinated Zed protocol addition plus a
compatibility window for sandboxes pinned to older `ZED_COMMIT`s, and it still trusts the
agent to echo correctly, which is the assumption that just failed twice. Recommended as a
follow-up once the thread-first resolver is the safety net underneath it. (Open Question 2.)

### The resolver

One function, used by **both** `handleMessageAdded` and `handleMessageCompleted`:

```
resolveTurnTarget(threadID, requestID) -> (interactionID, confidence)

1. requestToInteractionMapping[requestID] -> non-empty  => exact match, high confidence
2. streamingContexts[sessionID].interactionID           => the turn Helix has in flight
3. DB: most recent interaction with state=waiting        => durable fallback (restart-safe)
4. nothing                                               => genuinely unroutable; WARN loudly
```

Step 3 is today's unreachable ~L2734 fallback, promoted to a shared, reachable position.
The `mappingConsumed` early return is **deleted**; consumed-ness becomes an input to
duplicate detection, not a reason to return early.

### Duplicate detection, done properly

Replace "have I seen this request_id?" with "**have I already applied a completion to this
turn?**" — a property of the *interaction*, not the id:

- Resolve the target interaction first.
- If the resolved interaction is already `complete`/`interrupted`/`error` → suppress
  (this is the genuine duplicate, and it is what `2186abcda` was really trying to express).
- If the resolved interaction is `waiting` → **apply the completion**, whatever id it
  carried. A waiting turn with no completion applied is precisely the case that must never
  be dropped.

This satisfies both historical constraints simultaneously:
- **2026-04-28 case** (wrapper replays a stale completion mid-stream): the replay resolves
  to the *current* waiting interaction — which is a real problem only if that interaction
  has not finished. Guard: a completion is applied to a waiting interaction only when the
  agent is not still actively streaming into it (reuse the streaming context's activity
  timestamp), so a mid-stream replay is deferred rather than completing the turn early.
- **This case** (only completion the turn will ever get, arriving after streaming stopped):
  applied.

`MarkInteractionCompleteIfWaiting` already exists and is the right primitive — a
`WHERE state='waiting'` guarded transition, so concurrent handlers cannot clobber each
other. Reuse it rather than read-modify-write.

---

## 3. Zed-side fix

Make turn rotation depend on **turn boundaries**, not on whether the previous turn happened
to emit `message_completed`.

- Rotate `turn_request_id` when the turn *ends by any means* — including
  `turn_cancelled`/interrupt. Today only the `message_completed` path writes
  `last_completed_request_id`; the cancel path must mark the turn closed too.
- Better: make `set_thread_request_id` (called when Helix dispatches a prompt) the single
  authoritative "a new turn starts now" signal, and have the subscription read it at turn
  start rather than inferring boundaries from entry events. The global
  `THREAD_REQUEST_MAP` already holds the correct current id in every failure case observed
  — the bug is that the per-turn cache refuses to re-read it.
- Move the turn-boundary rotation **before** the `is_external_originated_entry` early return
  at L901, or rotate in the `chat_message` handler, so Helix-originated prompts are not
  excluded from turn rotation. Suppressing the *echo* of an external entry must not also
  suppress the *turn bookkeeping* for it.

Preserve the existing safety property: a follow-up message that overwrites the global map
mid-turn must not poison the in-flight turn's id. Rotation happens at boundaries only.

### Cross-repo release flow (from CLAUDE.md — order matters)

1. Commit in the Zed repo; `git rev-parse HEAD`.
2. Bump `ZED_COMMIT` in `sandbox-versions.txt` in the Helix repo
   (currently `1bac4bf841140cf562da9ac680beb4cc0338b0bc`).
3. **Open the Helix PR before pushing the Zed branch.**
4. Merge Zed first, then Helix.

---

## 4. Backstop: probe, don't guess

The gap is real and unbounded:

- `auto_wake_stuck_interactions.go` (`defaultAutoWakeStuckThreshold=180s`,
  `autoWakeScanInterval=10s`) documents in its header that it deliberately leaves
  *connected* interactions waiting "so a late completion can still settle them".
- `desktopResumeReapStaleThreshold` (3 min, `prompt_history_handlers.go:17`) only reaps
  when `isOrphanedWaitingInteraction` sees **no live WebSocket**.

Neither covers "agent connected, turn finished, completion dropped".

**Design: ask the agent.** Zed already answers `turn_cancelled{status:"noop"}` for a
request_id with no live turn — the exact signal needed, and in the incident it answered
`noop` correctly while Helix insisted the turn was running. Add a probe path in the
auto-wake worker:

```
for each session with a live WS and a waiting interaction:
    if no streaming activity for > probeIdleThreshold (assumed 60s):
        probe the agent for that thread
        status == "noop"      -> no turn running: settle the waiting interaction
                                 (content is already accumulated) + WARN + surface
        status == "cancelled" -> a turn WAS running: we just cancelled a live turn (bad)
        no answer             -> leave waiting; unchanged behaviour
```

**The long-tool-call constraint is the hard part.** A silent 10-minute tool call must never
be killed. Two protections:

1. The probe is only a *question*; the decision to complete comes from the agent's own
   answer, not from elapsed time. A live tool call means a live turn, and Zed answers
   accordingly.
2. `touch_activity` is already called in Zed's subscription for **any** event, so
   proof-of-life exists independently of token output — use it, not just token flow, for
   the idle threshold.

**Caveat, flagged as Open Question 3:** the only probe verb available today is
`cancel_current_turn`, which *cancels* if a turn is live. Using a destructive verb as a
liveness question is wrong in principle even when gated. Preferred shape is a dedicated
read-only `turn_status` request/response added to the sync protocol; that is a second Zed
change and needs the same `ZED_COMMIT` flow. Decide before implementing step 3.

**Explicitly not doing:** a blind timer that completes turns after N seconds. That would
kill long tool calls and is the failure mode the auto-wake header comment warns against.

## 5. Surfacing

When the backstop detects "waiting interaction, agent reports no turn running", that is a
Helix-side inconsistency and must not be silent:

- WARN with `session_id`, `interaction_id`, `request_id`, and the id actually received.
- Publish the interaction update to the frontend via the existing
  `publishInteractionUpdateToFrontend` path so the spinner clears rather than lying.
- Frontend banner treated as optional scope (Open Question 6).

---

## 6. Files in scope

| Area | Path |
|---|---|
| Completion handler; `mappingConsumed` drop (2718-2724); unreachable fallback (~2734) | `api/pkg/server/websocket_external_agent_sync.go:2640-2760` |
| `handleMessageAdded` + its recover-to-waiting chokepoint | same file, `:1116`, `:1440-1520` |
| `getOrCreateStreamingContext`, mapping population, consumed-sentinel guard | same file, `:1588-1700`, `:1795-1840` |
| `[TRANSITION] Auto-completed` accidental rescue | same file, `:1620-1690` |
| `turn_cancelled` / `noop` handling | same file, `:2214-2265` |
| Prompt-queue busy gate; orphan reap; threshold const | `api/pkg/server/prompt_history_handlers.go:17`, `:340-390` |
| Auto-wake worker (backstop lives here) | `api/pkg/server/auto_wake_stuck_interactions.go` |
| Zed turn/request_id association | `zed/crates/external_websocket_sync/src/thread_service.rs:867-1000`, `:1087-1180` |
| Zed `noop` reply | same file, `:1828-1885` |
| Handler tests (testify `WebSocketSyncSuite`) | `api/pkg/server/websocket_external_agent_sync_test.go` |
| Dockerized e2e (**mandatory if Zed sync is touched**) | `zed/crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` |
| Zed commit pin | `helix/sandbox-versions.txt` |

## 7. Notes for future agents

- **Two prior fixes in this exact area pull in opposite directions.** Read
  `design/2026-04-28-stale-request-id-rebind-loses-zed-updates.md` *and* commit `2186abcda`
  before touching the dedup. One prevents dropping genuine completions; the other prevents
  applying stale ones. Any change that satisfies only one of them will regress the other,
  which is how we got here.
- **`request_id` is not a turn identity in this system.** Zed reuses it across turns, the
  wrapper replays it on buffered events, and interrupts freeze it. Treat it as a hint.
- **`is_external_originated_entry` suppresses echo, and accidentally suppresses turn
  bookkeeping too.** Watch for other logic that sits behind that early return and should not.
- **The prompt-queue busy gate is the amplifier.** Any interaction stuck in `waiting`
  silently blocks all queued prompts for that session — a wedged turn is never just one
  wedged turn.
- Helix runs at `http://localhost:8080` in the sandbox; `psql` and the API log are the
  primary evidence tools. Never point diagnosis at meta.

---

# Implementation Notes (2026-08-04)

## Change of approach vs the original plan: probe instead of a grace window

The plan proposed resolving the stale-completion ambiguity with a short
"no stream activity for N seconds" grace window. **That was dropped during
implementation because it cannot survive a long tool call.** A tool call can
stream nothing for minutes and still be very much alive, so any activity-recency
threshold either kills long tool calls (too short) or leaves the wedge unfixed
for minutes (too long). There is no safe value.

The implemented design instead **asks the agent**, via a new non-destructive
`turn_status` request/response added to the sync protocol (Open Question 3,
resolved in favour of the dedicated read-only verb rather than reusing
`cancel_current_turn`, which *cancels* a live turn — unacceptable on a path that
fires during ordinary operation; the guard fired 3× in one day in production).

Decision table now implemented in `scheduleStaleCompletionSettle`:

| Thread state | Agent answer | Action |
|---|---|---|
| nothing waiting | (not probed) | suppress — genuine duplicate, as `2186abcda` intended |
| waiting | `running=false` | **settle the waiting interaction** — the 2026-08-04 wedge |
| waiting | `running=true` | leave alone — the 2026-04-28 wrapper-replay case |
| waiting | no answer / old ZED_COMMIT | leave alone — **fail closed** |

Failing closed on an unanswered probe is what makes this safe to ship ahead of
the Zed change and on sandboxes pinned to an older `ZED_COMMIT`: worst case is
today's behaviour, never a prematurely completed live turn.

## Files changed (Helix)

- `api/pkg/server/websocket_external_agent_sync.go`
  - `mappingConsumed` early return at the old L2718-2724 replaced with
    `scheduleStaleCompletionSettle` — the event is no longer discarded.
  - Added `probeTurnRunning`, `handleTurnStatusResponse`,
    `findWaitingInteraction`, `scheduleStaleCompletionSettle`.
  - Added `turn_status_response` to the sync dispatch switch.
  - Added `streamingContext.lastActivity`, set on every assistant
    `message_added` — the proof-of-life signal the Phase 4 backstop gates on.
- `api/pkg/server/server.go` — `pendingTurnStatusChannels`, plus a
  `turnStatusProbe` hook (nil in production) so the decision table is unit-testable
  without a live WebSocket.

## Gotchas found while implementing

- **`handleMessageCompleted`'s first parameter is the *agent* session id, not the
  Helix session id.** The settle path re-invokes the handler, and passing
  `helixSessionID` there silently broke `finalizeCommentResponse`, which keys off
  the request id derived from it. Pass the original `sessionID` through.
- **CGo is not installed in this sandbox** (`gcc` missing), so
  `CGO_ENABLED=1 go test` fails. The default (CGo off) builds and runs the
  `WebSocketSyncSuite` fine — ignore the CLAUDE.md CGo note for this package.
- **gomock is strict**, which is useful here: the "must not touch a live turn"
  tests need no assertion beyond registering no `UpdateInteraction` expectation.
  But a settle goroutine that outlives the test panics with "Fail in goroutine
  after test has completed" — tests must drain it before returning.
- `finalizeCommentResponse` calls `GetCommentByRequestID`, which only fires when
  the event carries a non-empty `request_id`. That is why the pre-existing
  `TestMessageCompleted_Normal` never needed that expectation and the new tests do.

---

# Corrected Zed root cause (found during implementation)

**The planning hypothesis was wrong, and the real cause is simpler and provable
from the source.** Recording both so a future agent does not re-derive it.

**Planning hypothesis (WRONG):** that `last_completed_request_id` is never
written on the interrupt path, because an interrupted turn emits
`turn_cancelled` rather than `message_completed`.

**Why it is wrong:** `thread.cancel(cx)` emits `Stopped(Cancelled)`
synchronously, so the `Stopped` handler *does* run for a cancelled turn and
*does* write `last_completed_request_id`.

**Actual cause:** the `Stopped` handler has a fallback for turns that produced no
assistant entries — it reports the id from the global `THREAD_REQUEST_MAP`
instead of the stale held one. It wrote **only** `last_completed_request_id`,
leaving `turn_request_id` untouched. The assistant rotation requires
`turn_request_id == last_completed_request_id`, so once the fallback makes them
diverge, **that equality can never hold again** and the thread is frozen on the
first turn's id for its whole life.

Trace of the incident:

| Turn | id | What happens |
|---|---|---|
| 1 | `…akby` | streams, interrupted. `Stopped` → both values = `…akby` |
| 2 | `…am8a` | **0 chars**, so nothing rotates `turn_request_id`. `Stopped` takes the fallback → `last_completed = …am8a`, `turn_request_id` still `…akby` — **diverged** |
| 3 | `…ammh` | streams 182,510 chars. Rotation checks `…akby == …am8a` → false → never rotates. Everything goes out as `…akby` |

Fix: one line, setting `turn_request_id` alongside `last_completed_request_id`
when the fallback fires. The `is_external_originated_entry` reordering the plan
proposed is **not needed**.

# What was actually verified (and what was not)

**Verified:**
- Deterministic repro as a unit test — reproduces the exact production log line
  and fails on the pre-fix code.
- `go test ./api/pkg/server/` green, including the new decision-matrix suites.
- `cargo test -p external_websocket_sync` — 57 passed.
- **Dockerized e2e, all 18 phases PASSED**, against a Zed binary rebuilt from the
  Rust changes. Includes a new phase 18 exercising the `turn_status` round trip.

**NOT verified — live spec-task run in the inner Helix.** Blocked: that instance
has **no Anthropic models registered** (`models` table holds only vllm/ollama
rows), so Zed logs `configured NativeAgent model did not become available within
15s` and no agent turn can start. Confirmed the proxy at `:8081` *does* serve
`claude-sonnet-4-6` on `/v1/messages` — it simply does not list Claude models in
`/v1/models`, which is presumably why Helix never registers them. Anyone wanting
a live agent turn in this sandbox must seed those model rows first.

# Environment gotchas (this sandbox)

- **No Rust toolchain, no gcc, no cmake by default.** Installing them
  (`rustup` + `apt-get install gcc libc6-dev cmake pkg-config libssl-dev`) is
  required before `cargo check` or `./stack build-zed`. Do not conclude the Zed
  changes are untestable without doing this — the e2e is runnable here.
- **`CGO_ENABLED=1 go test` fails** (no gcc initially); the default CGo-off build
  runs the server suite fine, contrary to the CLAUDE.md note.
- **The `claude` e2e round cannot pass here** — the proxy serves no Claude models
  *to the agent package*, so it times out at phase 1 thread creation, before any
  handler code. The `zed-agent` round covers the same Helix code.
- **`syncEventCallback` in the e2e test server already holds `d.mu` on entry.**
  Locking again inside a new `case` deadlocks the whole run with no error output.
  This was caught only by running the e2e — exactly the reason CLAUDE.md makes
  running it mandatory.
