# Design: Seamless Zed Reconnection After API Restart

## The Problem

When the API restarts while Zed is working on a user's request, two things go wrong:

1. **The user's in-flight request is killed.** On startup, the API runs `ResetRunningInteractions()` which marks every `waiting` interaction as `error` with "Interrupted". The user sees an error and has to manually retry their message.

2. **If Zed was about to send its response, that response is lost.** The `message_completed` event never reaches the API because the WebSocket connection died. Even after Zed reconnects, it doesn't re-send the lost event.

The reconnection itself works fine — Zed reconnects automatically, the API re-establishes the thread, and the frontend refreshes. But the user's work is lost.

## What Already Works

These things are solid after fixes in April 2-7:

1. **Zed reconnects automatically** — exponential backoff 1-30s, infinite retries
2. **API rebuilds its routing tables from the database** — `contextMappings` are restored from the session's persisted `ZedThreadID` on reconnect
3. **Thread re-establishment** — API sends `open_thread` immediately on reconnect, Zed re-subscribes
4. **Events buffer while disconnected** — Zed's unbounded channel queues events during downtime and drains them on reconnect
5. **Frontend refreshes** — `ReconnectingWebSocket` reconnects and invalidates React Query caches
6. **Partial responses are saved** — `flushAndClearStreamingContext` writes any in-progress response to the DB before clearing

## Evidence from Testing

I built a Go WebSocket test client that simulates Zed's behavior and ran it against the live Helix stack. Three tests:

### Test 1: What happens to a waiting request when the API restarts

I created a `waiting` interaction in the database, then restarted the API.

```
-- Before restart:
int_test_gap2_1775873519 | waiting |

-- After restart:
int_test_gap2_1775873519 | error | Interrupted
```

**Root cause:** `ResetRunningInteractions` runs unconditionally on every API startup (`serve.go:277`). It does a blanket UPDATE on all `waiting` interactions:

```go
// store_interactions.go:15-27
s.gdb.Model(&types.Interaction{}).
    Where("state = ?", types.InteractionStateWaiting).
    Updates(map[string]any{
        "state": types.InteractionStateError,
        "error": "Interrupted",
    })
```

This makes sense for non-Zed sessions (where the API was doing the processing), but for `zed_external` sessions Zed is still alive in the sandbox and can finish the work.

### Test 2: Does reconnection work end-to-end

I connected the test client, let it handle a request, then created a new `waiting` interaction, restarted the API, and watched what happened.

```
=== Phase 1: Client handles request A ===
[CONNECTED] at 03:13:09
[RECV] type=chat_message ... request_id=int_test_reconnect_A
[SENT] message_added
[SENT] message_completed

=== API restart ===
[DISCONNECTED] websocket: close 1006 (abnormal closure): unexpected EOF
[RECONNECT] Waiting 2s...
[ERROR] Connection failed: connection reset by peer
[RECONNECT] Waiting 4s...

=== Phase 2: Client reconnects ===
[CONNECTED] at 03:13:31
[SENT] agent_ready
[RECV] type=open_thread ... acp_thread_id=test-thread-001
[SENT] agent_ready (after open_thread)
-- NO chat_message received for request B --
```

**Result:** interaction A completed fine. Interaction B (created while client was connected, before restart) was errored out by `ResetRunningInteractions` before the client even reconnected. The reconnection itself worked perfectly — API sent `open_thread`, client reconnected — but there was nothing left to do.

API log confirms:
```
02:13:25  resetting running interactions              ← B gets errored here
02:13:31  [CONNECT] Session loaded for reconnect      ← client reconnects
02:13:31  [CONNECT] open_thread written to WebSocket  ← but no chat_message queued
```

### Test 3: What happens when the API dies while Zed is mid-response

I started the client with a 10-second delay before sending `message_completed`, then restarted the API during that delay.

```
[CONNECTED] at 03:14:05
[RECV] type=chat_message ... request_id=int_test_event_loss
[SENT] message_added
[INFO] Waiting 10s before sending message_completed...

--- API restarts here ---

[SENT] message_completed for request_id=int_test_event_loss  ← sent into the void
[DISCONNECTED] websocket: close 1006 (abnormal closure): unexpected EOF
[CONNECTED] at 03:14:17  ← reconnects, but too late

--- Interaction state ---
int_test_event_loss | error | Interrupted
```

Two things went wrong:

1. `message_completed` was sent after the connection died — the event was lost. The API log shows zero `message_completed` events received.
2. `ResetRunningInteractions` had already errored the interaction before the client could finish.

But there's good news: the `message_added` content ("Response #1 from simulated Zed agent") WAS saved:
```sql
SELECT response_message FROM interactions WHERE id = 'int_test_event_loss';
-- "Response #1 from simulated Zed agent"
```

The partial response was preserved by the streaming context flush on reconnect.

### Event loss in Zed code (confirmed by code review)

`websocket_sync.rs:307-312`:
```rust
if let Err(e) = ws_sink.send(Message::Text(json.into())).await {
    // Re-queue the event so it's not lost
    // (The event is already consumed, so we lose it - this is a known limitation)
    return; // Exit to trigger reconnection
}
```

The event (in `json`) was consumed from the channel and is dropped when the function returns. There's no retry buffer.

## Proposed Changes

### Change 1: Re-queue failed events in Zed

**File:** `websocket_sync.rs`

When `ws_sink.send()` fails, save the event instead of dropping it. Add a `VecDeque` retry buffer that lives in `reconnection_loop` (the outer function that survives across reconnections) and pass it as `&mut` to `run_connection`. Before reading new events from the channel, drain the retry buffer first.

```rust
// In reconnection_loop:
let mut retry_buffer: VecDeque<SyncEvent> = VecDeque::new();
loop {
    run_connection(&mut retry_buffer, &mut outgoing_rx, ...).await;
    // retry_buffer survives across reconnections
}

// In run_connection:
let event = if let Some(retry) = retry_buffer.pop_front() {
    retry  // Send failed events first
} else {
    outgoing_rx.recv()  // Then new events
};

if let Err(_) = ws_sink.send(...).await {
    retry_buffer.push_back(event);  // Save it for next connection
    return;
}
```

The buffer will only ever hold 1 event (the one that failed to send). Events that were queued in the channel while disconnected are already safe.

### Change 2: Remember completed requests in Zed (dedup)

**File:** `thread_service.rs`

Keep a small in-memory cache of recently completed `request_id`s. When a `chat_message` arrives with a `request_id` that's already in the cache, skip re-processing and replay `message_completed` instead.

This matters because after we fix Change 3, `pickupWaitingInteraction` will re-send `chat_message` for interactions that Zed already finished during the API downtime. Without dedup, the same prompt would get processed twice.

The cache is just a `HashMap<String, CompletedRequest>` with a 10-minute TTL. It lives in Zed's process, so it survives API restarts by definition.

### Change 3: Stop killing zed_external interactions on restart

**File:** `store_interactions.go`

Currently `ResetRunningInteractions` errors out ALL `waiting` interactions. Change it to skip `zed_external` sessions:

```go
s.gdb.Model(&types.Interaction{}).
    Where("state = ?", types.InteractionStateWaiting).
    Where("session_id NOT IN (SELECT id FROM sessions WHERE config->>'agent_type' = 'zed_external')").
    Updates(...)
```

This is the most important change. After this:
- Non-Zed interactions still get reset (API was doing the work, so it can't resume)
- Zed interactions survive, and `pickupWaitingInteraction` re-sends them to Zed on reconnect
- Combined with Change 2 (dedup), Zed either finishes the work or recognizes it already did

This change MUST be paired with Change 4 (timeout), otherwise a stuck Zed interaction could stay in `waiting` forever.

### Change 4: Timeout for stuck interactions

**File:** `websocket_external_agent_sync.go`

After `pickupWaitingInteraction` re-sends a `chat_message`, start a 120-second timer. If `message_completed` doesn't arrive within that window, mark the interaction as error with "Response interrupted by system restart — please try again."

This is the safety net for Change 3: if Zed never responds (crashed, lost the event, etc.), the interaction doesn't stay in `waiting` forever.

## Why This Order

The changes are interdependent:

- Change 1 (retry buffer) is standalone — it just prevents event loss on send failure
- Change 3 (stop killing interactions) requires Change 2 (dedup) to avoid duplicates
- Change 3 also requires Change 4 (timeout) to avoid stuck interactions
- Changes 2 + 3 + 4 together make the happy path work: user sends message → API restarts → Zed reconnects → interaction continues → user sees the response

## What's NOT Needed

- **No frontend changes** — the frontend already reconnects and refreshes
- **No new WebSocket protocol messages** — dedup happens in Zed's existing `chat_message` handler
- **No persistent event journal** — the channel + retry buffer + dedup covers the practical cases without adding complexity
