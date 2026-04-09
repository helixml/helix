# Design: Seamless Zed Reconnection After API Restart

## Current Architecture

```
Zed (sandbox)                         Helix API (Go)                  Frontend
─────────────                         ──────────────                  ────────
WebSocketSync ──ws──► handleExternalAgentSync()    ◄──ws── ReconnectingWebSocket
  outgoing_tx (unbounded mpsc)          contextMappings (in-memory)       streaming.tsx
  is_connected (AtomicBool)             requestToSessionMapping           wsWasDisconnectedRef
  exponential backoff 1-30s             requestToInteractionMapping       invalidates queries on reconnect
                                        Session + ZedThreadID (Postgres)
```

### What Already Works (after fixes in Apr 2-7)
1. **Zed reconnects automatically** — exponential backoff, infinite retries (`websocket_sync.rs:139-189`)
2. **API rebuilds contextMappings from DB** — `ZedThreadID` persisted in session metadata (`websocket_external_agent_sync.go:341-349`, fixed Apr 5 PR #2158)
3. **open_thread fast path** — when thread is already loaded in Zed's registry, `ensure_thread_subscription` is called without reloading (`thread_service.rs:1463-1473`, fixed Apr 7)
4. **Events buffer during downtime** — `outgoing_tx` is an unbounded mpsc channel; events queue while disconnected and drain on reconnect
5. **Frontend auto-recovers** — `ReconnectingWebSocket` reconnects; `openHandler` invalidates React Query caches on reconnection (`streaming.tsx:584-598`)
6. **DB fallback for routing** — `handleMessageAdded` falls back to `findSessionByZedThreadID` when `contextMappings` misses (`websocket_external_agent_sync.go:1018-1024`)

### Remaining Gaps

**Gap 1: Event dropped on send failure** (`websocket_sync.rs:310-312`)
When `ws_sink.send()` fails, the event is consumed and lost. The comment says "this is a known limitation."

**Gap 2: Duplicate interaction processing**
After restart, `pickupWaitingInteraction` finds interactions still in `waiting` state and re-sends `chat_message`. But Zed may have already completed the work during downtime. Result: AI processes the same prompt twice.

**Gap 3: No request_id deduplication in Zed**
Zed has no mechanism to detect that it already processed a `chat_message` with a given `request_id`. It always starts a fresh send.

**Gap 4: In-flight streaming restart**
If the AI was mid-response when the API died, the partial response in the frontend is lost. After reconnect, the whole response starts over (if Gap 2 isn't fixed, it's a full duplicate).

## Design

### Change 1: Re-queue failed events in Zed (Gap 1)

**File:** `zed/crates/external_websocket_sync/src/websocket_sync.rs`

In `run_connection`, when `ws_sink.send()` fails at line 307-312, push the event back to the front of the queue instead of dropping it.

Since `mpsc::UnboundedReceiver` doesn't support push-back, use a `VecDeque<SyncEvent>` as a local pending buffer that drains before reading `outgoing_rx`:

```rust
let mut pending_retry: VecDeque<SyncEvent> = VecDeque::new();

loop {
    // Drain pending retries first
    let event = if let Some(retry) = pending_retry.pop_front() {
        retry
    } else {
        // Then read from channel
        match outgoing_rx.recv() { ... }
    };

    if let Err(e) = ws_sink.send(...).await {
        pending_retry.push_back(event); // Re-queue instead of dropping
        return; // Trigger reconnection
    }
}
```

The `pending_retry` VecDeque lives on the stack of `run_connection` and survives across iterations of the reconnection loop since it's defined in the outer `reconnection_loop` function. Wait — `run_connection` is called each time, so the VecDeque would be lost. Instead, define it in `reconnection_loop` and pass as `&mut` to `run_connection`.

**Risk:** Low. The retry buffer holds at most 1 event (the one that failed to send). Channel-buffered events are already safe.

### Change 2: Request-ID deduplication in Zed (Gaps 2 & 3)

**File:** `zed/crates/external_websocket_sync/src/thread_service.rs`

When Zed receives `chat_message`, check if the `request_id` was already processed:

1. Maintain a `HashMap<String, CompletedRequest>` of recently completed request IDs (keep last 50, or TTL 10 minutes).
2. When `chat_message` arrives with a `request_id` that's in the completed set:
   - Skip re-processing
   - Re-send the cached `message_completed` event so the API marks the interaction as done
3. This handles the case where Zed completed work during API downtime and the API re-queues the interaction.

```rust
struct CompletedRequest {
    request_id: String,
    acp_thread_id: String,
    completed_at: Instant,
}

static COMPLETED_REQUESTS: Lazy<Mutex<HashMap<String, CompletedRequest>>> = ...;
```

In `handle_chat_message` (or equivalent), before dispatching to the thread:
```rust
if let Some(completed) = COMPLETED_REQUESTS.lock().get(&request_id) {
    // Already processed — replay completion
    send_websocket_event(SyncEvent::MessageCompleted { ... });
    return Ok(());
}
```

Record completion in the `Stopped` event handler (where `message_completed` is already sent).

**Risk:** Low. Small in-memory cache. False positives impossible since request_ids are unique. Cache survives API restarts (it's in Zed's process, not the API).

### Change 3: API checks completion status before re-queuing (Gap 2, defense-in-depth)

**File:** `helix/api/pkg/server/websocket_external_agent_sync.go`

In `pickupWaitingInteraction`, after finding a waiting interaction, add a `check_request_status` command to the protocol:

1. API sends `check_request_status { request_id }` to Zed
2. Zed responds with `request_status { request_id, status: "completed" | "unknown" }`
3. If completed: API marks interaction as done (fetch final content from the buffered events that will arrive)
4. If unknown: API proceeds to send `chat_message` as normal

**Alternative (simpler):** Skip this and rely entirely on Change 2 (Zed-side dedup). The `check_request_status` protocol addition is more complex and may not be worth it if Change 2 is reliable.

**Decision:** Start with Change 2 only. Add Change 3 later if dedup proves insufficient.

### Change 4: Mark interaction as errored on API restart if no response arrives (Gap 4)

**File:** `helix/api/pkg/server/websocket_external_agent_sync.go`

When `pickupWaitingInteraction` re-queues a waiting interaction, set a timeout (e.g., 60 seconds). If no `message_completed` arrives within the timeout (neither from buffered events nor from re-processing), mark the interaction as `error` with a user-visible message: "Response interrupted by system restart. Please try again."

This prevents interactions from being stuck in `waiting` state forever.

**Current behavior:** Interactions can stay in `waiting` indefinitely if `message_completed` never arrives.

## Architecture Decisions

1. **Zed-side dedup over API-side protocol** — Adding a new WebSocket command (`check_request_status`) increases protocol complexity. Since Zed has all the state (it knows what it already processed), dedup belongs there.

2. **Re-queue over replay** — For failed sends, re-queuing the single failed event is simpler than implementing a full event replay log. The channel already buffers events queued while disconnected.

3. **No full event journal** — A persistent event journal (write-ahead log) for all outgoing events would guarantee zero loss, but is overkill. The combination of channel buffering + single-event re-queue + dedup covers the practical cases.

4. **Frontend already handles reconnection** — No frontend changes needed. `ReconnectingWebSocket` + query invalidation on reconnect already refreshes the chat view.

## Codebase Patterns Found

- WebSocket sync events flow: GPUI entity event → `ensure_thread_subscription` callback → `send_websocket_event()` → `outgoing_tx` (mpsc) → `run_connection` select loop → `ws_sink.send()`
- Reconnection loop is in `reconnection_loop()` (`websocket_sync.rs:139`), calls `run_connection()` each iteration with `&mut outgoing_rx`
- `open_existing_thread_sync` has a fast path when thread is already in registry (line 1465) — just re-ensures subscription, no reload
- All persistent state lives in Postgres (sessions, interactions, ZedThreadID). In-memory state (contextMappings, request mappings, streaming contexts) is rebuilt from DB on reconnect.
