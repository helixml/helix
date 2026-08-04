# `message_completed` dropped for a stale `request_id` — sessions wedged "running" forever

**Date:** 2026-08-04
**Session that surfaced it:** `ses_01kz5zgx69c2r68vy1bbeqpbr8` (spec task `spt_01kz5zgx4v53spxtr9qyz8939e`)
**Zed thread:** `7f88913e-9e55-49ae-9870-dae61489c951`
**Wedge duration:** 12:28:23Z → 12:33:11Z (~4m48s), ended only by a user-forced interrupt

## Symptom

A spec task showed as **still running** long after the agent had visibly finished. A
182,510-character final report sat complete in the DB, in `state=waiting`, with
`sessions.config->>'external_agent_status' = 'running'`. A queued prompt sat undeliverable
behind it while the prompt-queue busy gate logged every ~2s, forever:

```
12:32:01Z DBG prompt_history_handlers.go:386 > Session is busy (interaction waiting),
          queue prompts will be processed after message_completed  queue_count=1
```

The completion it was waiting for **had already been received and deliberately discarded.**

## The two defects

### Defect A — Helix dropped a genuine completion

```
12:28:23Z INF > 🎯 [HELIX] RECEIVED MESSAGE_COMPLETED FROM EXTERNAL AGENT
                request_id=int_01kz6akbywh1b4avvbc3zq0h53
12:28:23Z WRN > ⚠️ [HELIX] Duplicate message_completed for consumed request_id mapping — ignoring
```

`requestToInteractionMapping["int_01kz6akby…"]` was `""` — the consumed sentinel, set when
that interaction completed 23 minutes earlier at 12:05:29Z. `handleMessageCompleted` set
`mappingConsumed = true` and returned `nil`. The DB fallback immediately below, which scans
for the most recent `state=waiting` interaction and would have settled it correctly, was
**unreachable** — the early return fired first.

The sentinel conflates two situations that a single-value map lookup cannot tell apart:

| Situation | Correct action |
|---|---|
| A second copy of a completion already applied | Suppress |
| The **only** completion this turn will ever get, carrying a stale id | **Apply it** |

Note the asymmetry with content routing: the same event burst logged
`⚠️ [HELIX] No interaction to route assistant message_added … content dropped`, yet all
182,510 chars still landed on the waiting interaction, because
`getOrCreateStreamingContext` has a recover-to-waiting path. Completion routing had none.

### Defect B — Zed echoed a dead turn's id (the actual source)

Three interactions were created in 42 seconds by rapid interrupt→resend cycles:

```
int_01kz6akbywh1b4avvbc3zq0h53   created 12:05:00Z   interrupted 12:05:29Z   1767 chars
int_01kz6am8ammjxf66kar1yc783k   created 12:05:29Z   interrupted 12:05:42Z      0 chars
int_01kz6ammh8caqx7s654gvfwbbp   created 12:05:42Z   WAITING             182510 chars
```

Zed's thread→request_id association stuck on the first, and stayed stuck for 23 minutes.

**The mechanism** (`crates/external_websocket_sync/src/thread_service.rs`). The
subscription keeps two per-turn values: `turn_request_id` (the id events are tagged with)
and `last_completed_request_id` (the id most recently reported). The assistant-side rotation
only advances `turn_request_id` when

```rust
current.is_empty() || current == last_completed_request_id
```

i.e. "the turn I'm holding has already been reported, move on". The `Stopped` handler has a
fallback for turns that produced no assistant entries: it reports the id from the global
`THREAD_REQUEST_MAP` instead of the stale held one — and then wrote **only**
`last_completed_request_id`, leaving `turn_request_id` untouched.

That single omission is the bug. Walk the incident:

1. Turn 1 (`…akby`) streams, is interrupted. `Stopped` → both values = `…akby`.
2. Turn 2 (`…am8a`) produces **0 chars**, so no assistant `NewEntry` ever rotates
   `turn_request_id`. Its `Stopped` sees `current == last_completed` (`…akby`), takes the
   fallback, and reports `…am8a`. Now `last_completed = …am8a` but
   `turn_request_id = …akby` — **permanently diverged.**
3. Turn 3 (`…ammh`) streams 182,510 chars. Every assistant entry checks
   `current(…akby) == completed(…am8a)` → false → **never rotates**. All of turn 3's
   `message_added` *and* its final `message_completed` go out tagged `…akby`.

Once the two diverge, the equality can never hold again: the thread is frozen on the first
turn's id for as long as it lives. This is why the guard fired **3 times that day on the
same session** (11:09:09Z, 12:05:00Z, 12:28:23Z), each with an older interaction's id.

The first two were masked because a follow-up prompt arrived and the
`[TRANSITION] Auto-completed` path cleaned up. It only becomes user-visible when the agent
finishes and nobody sends anything else — i.e. **exactly at end of task**, the worst
possible moment.

Zed itself was not confused about the *turn*, only about the *id*: it replied `noop` to the
eventual cancel and logged `Thread exists but no turn running for request_id=…ammh`.
Nothing had crashed; Zed, `claude-agent-acp` and the SDK process were all alive.

## Why `2186abcda`'s approach did not hold

`git log -S mappingConsumed` returns exactly one commit — `2186abcda` (2026-04-16),
*"replace timing-based message_completed dedup with state-based approach"*. Its message
describes **this same wedge**:

> Zed reuses the same request_id across different turns. The old `completedRequestIDs`
> dedup permanently blocked any request_id that was ever seen … This stalled the prompt
> queue indefinitely.

It replaced a heuristic keyed on `request_id` with a different heuristic keyed on
`request_id`. It narrowed the window — a permanently-blocked id became a
blocked-once-consumed id — but kept the premise that the agent's echoed id is a reliable
turn identity. It is not, and never was.

Then `design/2026-04-28-stale-request-id-rebind-loses-zed-updates.md` closed the *opposite*
hole: Zed's wrapper buffers events that aren't direct ACP responses and flushes them later
tagged with the last `request_id` it saw, so rebinding a consumed sentinel let a stale
replay prematurely mark a mid-stream interaction Complete.

The two guards pull in opposite directions, and both are correct responses to the same
broken premise. Keep the sentinel → genuine completions get dropped (this incident).
Remove it → live turns get completed early (April). **Neither can be tuned into
correctness.** The premise had to go.

## The fix

### 1. Helix: never silently drop a completion when a turn is waiting

The `mappingConsumed` early return is replaced by `scheduleStaleCompletionSettle`. The
decision is no longer made from the echoed id — it is resolved against state Helix owns,
and where that is ambiguous, **the agent is asked**:

| Thread state | Agent answer | Action |
|---|---|---|
| nothing waiting | (not probed) | suppress — genuine duplicate, as `2186abcda` intended |
| waiting | `running=false` | **settle the waiting interaction** |
| waiting | `running=true` | leave alone — the 2026-04-28 replay case |
| waiting | no answer | leave alone — **fail closed** |

### 2. A read-only `turn_status` probe

Zed already knew the truth ("no turn running") but only volunteered it as a side effect of
`cancel_current_turn`, which *cancels* a live turn. Using a destructive verb to ask a
question is not acceptable on a path that fires during ordinary operation — this guard
fired three times in one day. So `turn_status` / `turn_status_response` was added: a
read-only query answered from the same `ThreadStatus::Generating` check the cancel path
uses to decide cancelled-vs-noop.

An unanswered probe is treated as "running". That is what makes this safe to deploy ahead
of the Zed change and on sandboxes pinned to an older `ZED_COMMIT`: the worst case is
today's behaviour, never a prematurely completed live turn.

### 3. Zed: keep `turn_request_id` in lockstep

One line, at the point where the `Stopped` fallback reports a different id than the one
held:

```rust
*turn_request_id.borrow_mut() = completed_rid.clone();
*last_completed_request_id.borrow_mut() = completed_rid.clone();
```

This restores rotation for every subsequent turn on the thread. Note what was *not* done:
the guard was not widened, and the existing protection against a mid-turn follow-up
poisoning the in-flight id is untouched.

### 4. A real backstop, not a blind timer

`auto_wake_stuck_interactions.go` documented that it deliberately leaves connected
interactions waiting "so a late completion can still settle them", and
`desktopResumeReapStaleThreshold` only reaps with **no** live WebSocket. So
"agent connected + completion dropped" had **no recovery path at all** and was unbounded.

The worker's terminal "replay suppressed" branch now probes the agent. If it reports no
turn running, the interaction is settled, the frontend is updated, and the prompt queue is
drained.

Critically, this is not a timer. The pre-existing `lastPublish` gate carries an explicit
caveat that it cannot see inside a single long tool call — a 10-minute `npm install`
streams nothing. Asking the agent fixes that class of false positive outright: a running
tool call is a `Generating` thread, and a `Generating` thread is never settled.

## Tests

`api/pkg/server/websocket_external_agent_sync_test.go`:
- `TestMessageCompleted_StaleRequestID_SettlesWaitingInteraction` — the wedge. Fails on the
  old code with the exact production log line.
- `TestMessageCompleted_GenuineDuplicateStillSuppressed` — `2186abcda`'s intent preserved;
  asserts the agent is not even probed.
- `TestMessageCompleted_StaleRequestID_LiveTurnNotCompleted` — the 2026-04-28 regression
  guard, and the long-tool-call guarantee.
- `TestMessageCompleted_StaleRequestID_UnansweredProbeLeavesTurnAlone` — old `ZED_COMMIT`
  compatibility, fails closed.

`api/pkg/server/auto_wake_stuck_interactions_test.go`: `AutoWakeBackstopSuite` covers the
settle / live-turn / unanswered / no-thread / lost-the-race matrix.

## Notes for whoever touches this next

- **`request_id` is not a turn identity in this system.** Zed reuses it across turns, the
  wrapper replays it on buffered events, and (until this fix) a fallback could freeze it
  permanently. Treat it as a hint; resolve turns from state Helix owns.
- **Read both prior design docs before changing the dedup** — `2186abcda` and
  2026-04-28 pull in opposite directions, and satisfying only one regresses the other.
- **Any interaction stuck in `waiting` silently blocks every queued prompt for that
  session.** A wedged turn is never just one wedged turn.
- `handleMessageCompleted`'s first parameter is the *agent* session id, not the Helix
  session id — the settle path re-invokes the handler and passing the wrong one silently
  breaks comment finalization.
