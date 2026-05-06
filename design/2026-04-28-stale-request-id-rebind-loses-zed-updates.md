# Stale `request_id` rebind loses Zed→Helix updates

**Date:** 2026-04-28
**Spec task that surfaced it:** `spt_01kq8cnjzfqc51nn0c6ddxkw8r`
**Helix session:** `ses_01kq8cnnkmww35bacpscbrehn0`
**Zed thread:** `7dbafa8b-6ff7-40c0-b667-9e81c530e99e`

## Symptom (user report)

> "another example of a session where zed is pushing out updates and helix is failing to receive them"
>
> Later: "now it's sync'd up again but at the point at which i told you it was stuck..."

Helix is receiving the WebSocket frames from Zed (the `External agent added message` log fires ~50k times in 2h on this session, the connection never disconnects), but the user-visible chat in the Helix UI lags behind, drops content, or shows "stale" completions while the next turn is still in flight.

## Evidence

`requestToInteractionMapping` lookup-by-`request_id` is the authoritative router that decides which interaction a given `message_completed` belongs to. `handleMessageCompleted` consumes the mapping by setting it to the empty string `""` after first use; subsequent events with the same `request_id` are intended to be dropped as duplicates (`websocket_external_agent_sync.go:2287-2315`).

In the failing window the same `request_id` `req_eead10e3-d5b8-40a4-a75c-5f0ed91e3c9f` was processed as `message_completed` **four times**, each routed to a **different** Helix interaction:

```
07:48:09Z  Sent comment to agent             request_id=req_eead10e3...  interaction_id=int_01kq9gym4v16n4zz2fxegv3q5e
07:57:23Z  RECEIVED MESSAGE_COMPLETED        request_id=req_eead10e3...
07:57:23Z  Matched interaction by mapping    request_id=req_eead10e3...  interaction_id=int_01kq9gym4v16n4zz2fxegv3q5e   (correct - original turn)

07:57:56Z  Populated requestToInteractionMapping from streaming context (Zed-initiated message)
                                             request_id=req_eead10e3...  interaction_id=int_01kq9hgcee4f6pmjve4e990cgq   ← OVERWRITES the consumed sentinel
07:58:42Z  RECEIVED MESSAGE_COMPLETED        request_id=req_eead10e3...
07:58:42Z  Matched interaction by mapping    request_id=req_eead10e3...  interaction_id=int_01kq9hgcee4f6pmjve4e990cgq   ← stale completion routed to new interaction
07:58:42Z  Marked interaction as complete    interaction_id=int_01kq9hgcee4f6pmjve4e990cgq                                ← prematurely completed mid-stream

08:04:41Z  Populated requestToInteractionMapping ...  interaction_id=int_01kq9hws3pjchmrq4rsns6c3zy
08:04:44Z  RECEIVED MESSAGE_COMPLETED       request_id=req_eead10e3...
08:04:44Z  Matched interaction by mapping   request_id=req_eead10e3...  interaction_id=int_01kq9hws3pjchmrq4rsns6c3zy
08:04:44Z  Marked interaction as complete   interaction_id=int_01kq9hws3pjchmrq4rsns6c3zy                                 ← same pattern, again

08:04:49Z  Populated requestToInteractionMapping ...  interaction_id=int_01kq9hx01evpeh9g89dgc11r00
08:04:52Z  RECEIVED MESSAGE_COMPLETED       request_id=req_eead10e3...
08:04:52Z  Matched interaction by mapping   request_id=req_eead10e3...  interaction_id=int_01kq9hx01evpeh9g89dgc11r00
08:04:52Z  Marked interaction as complete   interaction_id=int_01kq9hx01evpeh9g89dgc11r00                                 ← and again
```

`req_eead10e3...` was the request id Helix generated when it dispatched a *single* design review comment at 07:48:09. The wrapper inside Zed buffered events that were not directly downstream of an ACP `session/prompt` (see `auto_wake_stuck_interactions.go:1-50` for the full background — it documents the wrapper's event-buffering behaviour and the `agentclientprotocol/agent-client-protocol#554` upstream issue) and flushed them later, tagging each flushed `message_completed` with the *last* `request_id` it saw — the now-stale `req_eead10e3`.

## Root cause

`getOrCreateStreamingContext` rebuilds `requestToInteractionMapping` on every `message_added` whose `request_id` ↔ `interaction_id` pair doesn't already match (`websocket_external_agent_sync.go:1590-1606`):

```go
// Populate requestToInteractionMapping for Zed-initiated messages.
// When the user types in Zed, the interaction is created without a mapping.
// Zed reuses the same request_id for the response, so we need to register
// it here so handleMessageCompleted can route correctly.
if requestID != "" && newInteractionID != "" && newInteractionID != expectedInteractionID {
    apiServer.contextMappingsMutex.Lock()
    if apiServer.requestToInteractionMapping == nil {
        apiServer.requestToInteractionMapping = make(map[string]string)
    }
    apiServer.requestToInteractionMapping[requestID] = newInteractionID   // ← unconditional overwrite
    apiServer.contextMappingsMutex.Unlock()
    ...
}
```

`expectedInteractionID` was read with a single-value lookup at line 1438-1441, so it returns `""` for *both* "no entry" and "consumed entry" states. The condition `newInteractionID != expectedInteractionID` therefore can't tell them apart, and the unconditional `requestToInteractionMapping[requestID] = newInteractionID` clobbers the `""` sentinel that `handleMessageCompleted` planted.

Once the sentinel is gone, the next stale `message_completed` carrying `req_eead10e3` no longer hits the dedup branch (`mappingConsumed = true`); it matches the freshly-bound interaction and runs the full completion path on it — flushing the streaming context, marking state Complete, publishing a final session update — even though that interaction is mid-turn and has its own real completion still pending.

## Why the user sees "Helix failing to receive updates"

From the user's perspective the UI shows:

- The current turn's interaction transitions to Complete prematurely (the stale `message_completed` from the wrapper's buffer).
- Subsequent `message_added` tokens for the *real* current turn are still appended (the streaming context still points there) but the turn is already "done" in the UI, so the chat composer un-greys, the spinner stops, etc.
- When the *real* `message_completed` for the current turn finally arrives, it hits the early-return at `websocket_external_agent_sync.go:2391-2397` (`Interaction already complete ... skipping redundant completion`) and the actual final response state is never published. The UI stops updating until the next turn pulls a fresh state.

The user described this as "Zed pushing updates and Helix failing to receive them": Helix is receiving fine, but it's mis-routing the completions and dropping the publish on the floor.

## Hypothesis tests

| Hypothesis | Result |
|---|---|
| WebSocket disconnect/reconnect dropping events. | **Refuted.** Single `External agent WebSocket connected` at 06:24:00, no unregister/disconnect for this session in the next 2 h. |
| Real session-level error from Zed (`thread_load_error` etc.). | **Refuted.** Only one warning in this session in 2 h, and it's a comment-response timeout unrelated to chat. No `thread_load_error` events. |
| Wrapper crash (the Bug-2 case from PR #2311). | **Refuted.** No `Claude Agent process exited` / `Session not found` in the logs for this session. |
| Stale `request_id` defeats dedup → mis-routes `message_completed`. | **Confirmed.** Trace above shows the rebind→re-match→premature-complete pattern repeating four times for the same `request_id`. |

## Fix

`getOrCreateStreamingContext` must distinguish "request_id never seen" from "request_id consumed by completion". Two-value lookup:

```go
if requestID != "" && newInteractionID != "" {
    apiServer.contextMappingsMutex.Lock()
    if apiServer.requestToInteractionMapping == nil {
        apiServer.requestToInteractionMapping = make(map[string]string)
    }
    existing, alreadySeen := apiServer.requestToInteractionMapping[requestID]
    switch {
    case alreadySeen && existing == "":
        // Consumed sentinel — wrapper is replaying buffered events with a stale
        // request_id (see auto_wake_stuck_interactions.go for background). Do
        // NOT rebind: that would defeat the duplicate-completion dedup in
        // handleMessageCompleted and prematurely complete an unrelated mid-turn
        // interaction. Stale streaming tokens still flow into the current
        // interaction via the streaming context fallback, but the stale
        // completion is dropped where it should be.
        apiServer.contextMappingsMutex.Unlock()
        log.Debug().
            Str("session_id", helixSessionID).
            Str("request_id", requestID).
            Str("would_have_bound", newInteractionID).
            Msg("🛡️ [HELIX] Ignoring stale request_id rebind (mapping previously consumed)")
    case existing != newInteractionID:
        apiServer.requestToInteractionMapping[requestID] = newInteractionID
        apiServer.contextMappingsMutex.Unlock()
        log.Info().
            Str("session_id", helixSessionID).
            Str("request_id", requestID).
            Str("interaction_id", newInteractionID).
            Msg("🗺️ [HELIX] Populated requestToInteractionMapping from streaming context (Zed-initiated message)")
    default:
        apiServer.contextMappingsMutex.Unlock()
    }
}
```

The `alreadySeen && existing == ""` branch is the new behaviour. The other two branches preserve the existing semantics: register on first sight, rebind only when the request_id is now associated with a different live interaction.

## Why this is safe

- **Live conversations (existing == newInteractionID):** no-op — same as today.
- **First-time request_ids (!alreadySeen):** registered as today.
- **Cross-turn rebinds where the previous turn is still in flight (existing != "" && existing != newInteractionID):** still rebinds — preserves the "user sent a follow-up while the previous turn was running" path.
- **Stale wrapper-buffered events (existing == ""):** dropped from the routing table. Their content still flows into the current Waiting interaction via the streaming context fallback at line 1551-1558 (`find most recent waiting interaction`), so no streaming tokens are lost. What we *don't* let through is the final `message_completed`, which would otherwise prematurely complete the current turn — that's the actual bug.

## Test plan

1. **Unit test in `websocket_external_agent_sync_test.go`:** simulate the trace — register a request_id via `sendMessageToSpecTaskAgent`, fire a `message_completed` (consumes), then fire a `message_added` whose Waiting interaction differs from the consumed one, then a second `message_completed` carrying the same stale request_id. Assert: second completion is logged as `Duplicate message_completed for consumed request_id mapping — ignoring`, the mid-stream interaction is *not* marked Complete.
2. **Manual:** dispatch a comment to a long-running spec task, wait for the first turn to complete, then trigger more agent activity (e.g. a Zed user-typed message). Confirm the new turn streams through to completion without spurious "Marked interaction as complete" events for it before its real `message_completed` arrives.
