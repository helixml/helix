# Stop dropping genuine message_completed events that carry a stale request_id

## Summary

A spec task on meta showed as **running** for ~5 minutes after the agent had
visibly finished. The completion event had been delivered and Helix
**deliberately discarded it**: it carried the `request_id` of a turn interrupted
23 minutes earlier, so `handleMessageCompleted` hit the `mappingConsumed` early
return and dropped it. A finished 182,510-character answer was left in
`state=waiting` forever, with a queued prompt parked behind it.

The consumed-sentinel check cannot tell two very different events apart:

| Situation | Correct action |
|---|---|
| A second copy of a completion already applied | Suppress |
| The **only** completion this turn will ever get, carrying a stale id | **Apply it** |

Both look identical to a `request_id` lookup, so the second was thrown away —
and the DB fallback directly below it, which would have settled the turn
correctly, was unreachable.

This is the same wedge `2186abcda` fixed in April by swapping a timing-based
dedup for a state-based one. That change narrowed the window but kept the
premise that the agent's echoed id is a reliable turn identity. It is not.
`design/2026-04-28-stale-request-id-rebind-loses-zed-updates.md` then closed the
*opposite* hole, so the two guards now pull against each other. Rather than
re-tune either, this PR removes the premise.

Full write-up: `design/2026-08-04-message-completed-stale-request-id-wedge.md`.

## Changes

- **`handleMessageCompleted` no longer silently drops a completion when a turn
  is waiting.** The `mappingConsumed` early return is replaced by
  `scheduleStaleCompletionSettle`, which resolves against state Helix owns and,
  where that is ambiguous, **asks the agent**:

  | Thread state | Agent answer | Action |
  |---|---|---|
  | nothing waiting | (not probed) | suppress — genuine duplicate |
  | waiting | `running=false` | settle the waiting interaction |
  | waiting | `running=true` | leave alone — the 2026-04-28 replay case |
  | waiting | no answer | leave alone — **fail closed** |

- **New read-only `turn_status` probe.** Zed already knew the truth but only
  volunteered it as a side effect of `cancel_current_turn`, which *cancels* a
  live turn. Using a destructive verb to ask a question is not acceptable on a
  path that fired three times in one day, so `turn_status` /
  `turn_status_response` was added instead.

- **A real backstop in `auto_wake_stuck_interactions.go`.** That worker
  deliberately declined to touch connected sessions and
  `desktopResumeReapStaleThreshold` only fires with no live WebSocket, so
  "agent connected, turn finished, completion dropped" had **no recovery path at
  all**. Its terminal branch now probes the agent and, on a definite "no turn
  running", settles the interaction, publishes to the frontend and drains the
  prompt queue.

  This is **not** a timer. The pre-existing `lastPublish` gate documents that it
  cannot see inside a single long tool call; asking the agent removes that whole
  class of false positive, because a running tool call is a `Generating` thread.

- Surfacing: the disagreement between "Helix thinks a turn is running" and "the
  agent says it isn't" is now a WARN with session, interaction and thread ids.

- `sandbox-versions.txt`: bump `ZED_COMMIT` for the paired Zed change.

## Testing

- New handler tests in `websocket_external_agent_sync_test.go`: the wedge
  (reproduces the exact production log line and **fails on the old code**),
  genuine-duplicate suppression, live-turn-not-completed (the 2026-04-28
  regression guard **and** the long-tool-call guarantee), and unanswered-probe
  fail-closed.
- New `AutoWakeBackstopSuite` covering settle / live-turn / unanswered /
  no-thread / lost-the-race.
- `go test ./api/pkg/server/` — green.
- **Dockerized Zed WS e2e run against a Zed binary built from the paired
  changes: all 18 phases PASSED**, including "Rapid 3-turn cancel", "Mid-stream
  interrupt", "Queue busy-defer", "Queue interrupt", and a new phase 18 that
  exercises the `turn_status` round trip over a real WebSocket.

### Not tested

Live spec-task verification in the inner Helix was **not** possible: that
instance has no Anthropic models registered (only vllm/ollama), so Zed reports
`configured NativeAgent model did not become available within 15s` and no agent
turn can run. The e2e harness configures Zed directly and does exercise real
agent turns, so it is the strongest available end-to-end evidence.

The `claude` e2e round also cannot run here — the local proxy serves no Claude
models to the agent package, and it fails at phase 1 thread creation, before any
code touched by this PR. The `zed-agent` round exercises the same handler code
and passes in full.
