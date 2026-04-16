# Lost Responses: Zed-Originated Messages vs Helix Queue Race Condition

**Date:** 2026-04-16
**Status:** Analysis complete, fix partially addressed by recent Zed changes
**Task:** spt_01kp9z6hjn9jmpgq001mqkawx2 (helix-next/mirantis email)
**Session:** ses_01kp9z6k5rxnfgas7xv1w0xw9t

## Summary

User sent two messages at nearly the same time (~6ms apart) during an implementation session. One was typed directly in Zed, the other was queued from the Helix UI. The Zed-typed message got a response visible in Zed but the response never appeared in the Helix session. The Helix-queued message was immediately bounced with an empty `message_completed`.

**Note:** This session was running an older Zed binary that did NOT include the April 10-14 fixes for interrupt handling and request_id desync. Those fixes likely address one of the two bugs identified here, but not both.

## Incident Timeline (all times UTC, 2026-04-16)

| Time | Event |
|------|-------|
| 03:10:16 | `message_completed` for `int_01kpa48f8sh0csa7atma1tw8nb` ("dev site" prompt) |
| 03:11:04 | `message_completed` for `int_01kpa496b42hvck99fz9ckdjay` ("update claude.md") — triggers queue processing |
| 03:11:05.027 | Zed user message arrives → creates `int_01kpa4ana3j40mnfcdze2ec3bs` ("startup scripts + dropdown") |
| 03:11:05.033 | Queue processor fires → creates `int_01kpa4ana94t49c3knmmgx1pm0` ("use cases page") and sends to Zed |
| 03:11:05.0xx | Zed immediately bounces "use cases" with `message_completed(message_id="0", request_id=int_01kpa4ana94t49c3knmmgx1pm0)` — **empty response** |
| 03:11:05.0xx | Helix logs: `⚠️ message_completed but response_message is EMPTY` — marks interaction complete with 0 bytes |
| 03:11:10 | Zed starts streaming response to "startup scripts" into `int_01kpa4ana3j40mnfcdze2ec3bs` |
| 03:11:10-03:17 | Response content grows (605+ bytes of tool calls, nav dropdown analysis, etc.) |
| 03:17:18 | `message_completed` arrives with `request_id=int_01kpa496b42hvck99fz9ckdjay` — **wrong request_id!** (this is the claude.md prompt, already completed at 03:11:04). Ignored by dedup cache. |
| never | No `message_completed` ever arrives for `int_01kpa4ana3j40mnfcdze2ec3bs` → stays `waiting` forever |

## Final DB State

| Interaction | Prompt | State | Response |
|-------------|--------|-------|----------|
| `int_01kpa4ana3j40mnfcdze2ec3bs` | "startup scripts..." (from Zed) | **waiting** | Has content (tool calls, analysis) but never completed |
| `int_01kpa4ana94t49c3knmmgx1pm0` | "use cases page..." (from queue) | **complete** | **Empty** — bounced immediately |

The user saw responses in Zed for both messages. Only the "startup scripts" response partially streamed to Helix (visible in the interaction's `response_message`) but was never marked complete. The "use cases" response never made it to Helix at all.

## Root Causes

### Bug 1: Zed bounces queued prompts while busy (old Zed code)

When the queue processor sent the "use cases" prompt to Zed via WebSocket, the Zed agent was already busy starting to process the user's directly-typed "startup scripts" message. The old Zed code immediately returned `message_completed` with `message_id=0` and no content — effectively dropping the message on the floor.

**Status:** This is the "n-1 response shift" / request_id desync bug. The following Zed commits address this:

- `1a3fc57adc` (Apr 3): Add request_id to message_added events for interaction routing
- `a7e4d8b850` (Apr 10): Implement real interrupt — cancel running turn before queuing new message
- `90bdb6cf75` (Apr 10): Emit Stopped synchronously in cancel() to fix FIFO ordering
- `2f182e64d6` (Apr 14): Prevent request_id desync from background events and duplicate Stopped

The session was running a Zed binary built before these fixes were deployed. The current pinned Zed commit (`2f182e64d6`) includes all of them. **A session restart with the latest `helix-ubuntu` image would have had these fixes.**

With the new Zed code:
- If a `chat_message` arrives while the agent is busy, it cancels the running turn (interrupt) and processes the new message
- `message_completed` uses turn-scoped request_ids, preventing the n-1 shift
- The "startup scripts" response's `message_completed` would have had its own correct request_id instead of the stale `int_01kpa496b42hvck99fz9ckdjay`

### Bug 2: Zed user messages have no requestToInteractionMapping entry (Helix-side, unfixed)

When a user types a message directly in Zed, the `handleMessageAdded(role=user)` code creates a Helix interaction (lines 1279-1325 in `websocket_external_agent_sync.go`) but does **NOT** store it in `requestToInteractionMapping`. This means:

1. When the agent responds with `message_added(role=assistant)` tokens, the streaming context resolves the interaction via DB fallback (scan for most recent Waiting interaction)
2. When `message_completed` arrives, there's no mapping entry to match it to the Zed-originated interaction
3. If the request_id in `message_completed` doesn't match any mapping entry, the fallback scan picks the most recent Waiting interaction — which might be the wrong one

Even with the latest Zed fixes, this remains a problem:
- User types in Zed → interaction created without mapping
- Zed responds → sends request_id from its turn-scoped tracking
- But Helix has no mapping for that request_id → falls through to DB fallback
- If another interaction is also Waiting (e.g. from the queue), the wrong one could be matched

The contrast with Helix-originated queue prompts is clear:

| Source | requestToInteractionMapping entry? | message_completed routing |
|--------|-----------------------------------|--------------------------|
| Helix queue prompt | Yes (interaction ID used as request_id) | Direct mapping lookup |
| Helix design review comment | Yes (request_id stored at send time) | Direct mapping lookup |
| User types in Zed | **No** | DB fallback scan only |

### Bug 3: Queue processor fires during Zed message arrival (Helix-side, unfixed)

The queue processor (`processPromptQueue`) is triggered asynchronously when `message_completed` arrives:

```go
go apiServer.processPromptQueue(context.Background(), helixSessionID)
```

It checks whether the last interaction is in `Waiting` state before sending the next queued prompt. But there's no coordination with the "Zed user message arriving" path. At 03:11:05, both happened within the same millisecond:

1. Zed user message arrived → created interaction in `Waiting` state
2. Queue processor checked DB → last interaction might not yet have been the new one
3. Queue processor sent the "use cases" prompt → Zed bounced it

Even if the queue processor correctly saw the new Waiting interaction and deferred, the fundamental problem remains: there's no mechanism to tell the queue processor "a Zed user message is in flight, don't send anything until the agent finishes responding to it."

## Proposed Fixes

### Fix A: Register Zed user message interactions in the mapping (Helix-side)

When `handleMessageAdded(role=user)` creates an interaction for a Zed-originated message, also store a mapping entry. The key could be the interaction ID itself, or a synthetic request_id generated from the context_id + message_id.

Then, when the agent responds, the streaming tokens include a request_id that can be matched. For Zed-originated messages where Zed doesn't have a Helix-assigned request_id, Helix could use the `message_id` from the `message_added` event as the fallback mapping key.

**Complexity:** Medium. Requires coordination on what key to use, since Zed doesn't know the Helix interaction ID.

### Fix B: Add in-flight tracking to the queue processor (Helix-side)

Add a per-session flag or timestamp that is set when a Zed user message arrives and cleared when `message_completed` is received. The queue processor checks this flag before sending queued prompts.

```go
// In handleMessageAdded for user messages:
apiServer.sessionInFlight[helixSessionID] = time.Now()

// In processPromptQueue:
if t, ok := apiServer.sessionInFlight[sessionID]; ok && time.Since(t) < 5*time.Minute {
    log.Debug().Msg("Session has in-flight Zed message, deferring queue")
    return
}
```

**Complexity:** Low. Simple flag check. The flag is cleared in `handleMessageCompleted`.

### Fix C: Don't mark interactions complete when response is empty (Helix-side)

When `message_completed` arrives and the response is empty (the `⚠️` warning case), don't mark the interaction as complete. Instead, leave it in `waiting` state. This prevents the "immediate bounce" scenario from permanently losing the message.

The queue processor could then retry the prompt when the session becomes idle again.

**Complexity:** Low, but could mask other bugs. The warning already exists at line 2268 — this would just change the behavior from "warn and proceed" to "warn and leave waiting."

### Fix D: Confirm recent Zed fixes are sufficient for Bug 1

Restart a session with the latest `helix-ubuntu` image (which includes Zed built from `2f182e64d6`) and reproduce the scenario: type a message in Zed while there's a queued prompt. Verify that:

1. The queued prompt is handled correctly (interrupt behavior)
2. The `message_completed` for the Zed response has the correct request_id
3. No empty bounces occur

If the new Zed code properly handles concurrent messages (via the interrupt mechanism), Bug 1 may be fully resolved. But Bug 2 (no mapping entry for Zed user messages) and Bug 3 (queue processor firing during Zed message arrival) remain Helix-side issues that should be addressed regardless.

## Recommended Priority

1. **Fix D first** — verify whether the new Zed code already resolves the user-visible symptom
2. **Fix B** — cheapest Helix-side fix, prevents queue from stomping on Zed messages
3. **Fix A** — proper fix for Zed user message routing, prevents future edge cases
4. **Fix C** — defense in depth, prevents permanent message loss from any future bounce scenarios
