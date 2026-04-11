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

**Gap 1: Event dropped on send failure** (`websocket_sync.rs:307-312`)
When `ws_sink.send()` fails, the event is consumed and lost. The comment says "this is a known limitation."

**Gap 2: Waiting interactions errored on restart** (`serve.go:277-282`, `store_interactions.go:15-27`)
On every API startup, `ResetRunningInteractions()` marks ALL `waiting` state interactions as `error` with message "Interrupted". This means:
- The user's in-flight request is LOST (not duplicated)
- The user sees "Interrupted" and must manually retry
- There is no attempt to resume or re-queue the work

**Gap 3: No request_id deduplication in Zed**
If `ResetRunningInteractions` were removed (to allow re-queuing), Zed has no mechanism to detect that it already processed a `chat_message` with a given `request_id`. It would start a fresh send, causing duplicate processing.

**Gap 4: No timeout for stuck interactions**
If `message_completed` never arrives for a `waiting` interaction (e.g., Zed crashes without sending it), the interaction stays in `waiting` state indefinitely — unless the API restarts, at which point `ResetRunningInteractions` force-errors it.

## Evidence from Testing

### Test Setup
- Session `ses_01knx2qjhs3vjjbef8697z2qb2` with `agent_type=zed_external` and `zed_thread_id=test-thread-001`
- Go WebSocket test client (`ws_reconnect_client.go`) simulating Zed agent behavior
- API key `hl-test-reconnect-key-1234567890abcdef` scoped to real user `usr_01knx2rmjvy4g92mkw4fr62svk`

### Evidence 1: ResetRunningInteractions destroys waiting interactions on restart

```
-- Before restart:
int_test_restart_002 | waiting |

-- After restart:
int_test_restart_002 | error | Interrupted
```

Code path (`api/pkg/store/store_interactions.go:15-27`):
```go
func (s *PostgresStore) ResetRunningInteractions(ctx context.Context) error {
    err := s.gdb.WithContext(ctx).Model(&types.Interaction{}).
        Where("state = ?", types.InteractionStateWaiting).
        Updates(map[string]any{
            "state": types.InteractionStateError,
            "error": "Interrupted",
        }).Error
    ...
}
```

Called unconditionally at startup (`api/cmd/helix/serve.go:277-282`).

### Evidence 2: WebSocket reconnection and open_thread work correctly

API logs on agent connect:
```
INF websocket_external_agent_sync.go:280 External agent WebSocket connected agent_id=ses_01knx2qjhs3vjjbef8697z2qb2
INF websocket_external_agent_sync.go:338 [CONNECT] Session loaded for reconnect agent_type=zed_external zed_thread_id=test-thread-001
INF websocket_external_agent_sync.go:384 [CONNECT] Sending open_thread directly on new connection before agent_ready gate
INF websocket_external_agent_sync.go:417 [CONNECT] ✅ open_thread written directly to WebSocket
```

Client logs:
```
[CONNECTED] at 03:05:12 (total chat_messages received so far: 0)
[SENT] agent_ready
[RECV] type=open_thread raw={"type":"open_thread","data":{"acp_thread_id":"test-thread-001","agent_name":"zed-agent","session_id":"ses_01knx2qjhs3vjjbef8697z2qb2"}}
[SENT] agent_ready (after open_thread)
```

### Evidence 3: pickupWaitingInteraction queues chat_message and routing works

When a `waiting` interaction exists (before `ResetRunningInteractions` clears it):
```
INF websocket_external_agent_sync.go:470 🔧 [HELIX] Created request_id mapping from waiting interaction ID request_id=int_01knx2qjhs3vjjbef86cc7k2g0
INF websocket_external_agent_sync.go:513 ✅ [HELIX] Queued initial chat_message for Zed (will send when agent_ready)
```

Client receives:
```
[RECV] type=chat_message raw={"type":"chat_message","data":{"acp_thread_id":null,"agent_name":"zed-agent","message":"...","request_id":"int_01knx2qjhs3vjjbef86cc7k2g0"}}
[SENT] message_added
[SENT] message_completed for request_id=int_01knx2qjhs3vjjbef86cc7k2g0
```

### Evidence 4: contextMappings rebuilt from DB

```
INF websocket_external_agent_sync.go:343-344 [HELIX] Restored contextMappings from session metadata zed_thread_id=test-thread-001
```

(Log level is TRACE in production; confirmed via code at lines 341-349)

### Evidence 5: Event loss in Zed (code review)

`websocket_sync.rs:307-312`:
```rust
if let Err(e) = ws_sink.send(Message::Text(json.into())).await {
    log::error!("❌ [WEBSOCKET-OUT] Failed to send WebSocket message: {} - will reconnect", e);
    // Re-queue the event so it's not lost
    // (The event is already consumed, so we lose it - this is a known limitation)
    return; // Exit to trigger reconnection
}
```

The `json` variable (serialized from the event consumed from `outgoing_rx`) is dropped when the function returns. No retry buffer exists.

### Evidence 6: Streaming context is flushed on reconnect

`websocket_external_agent_sync.go:1500-1520`: `flushAndClearStreamingContext` writes any dirty `sctx.interaction` back to the DB before clearing the context. This preserves partial responses.

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

### Change 2: Request-ID deduplication in Zed (Gap 3)

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

### Change 3: Remove ResetRunningInteractions for zed_external sessions (Gap 2)

**File:** `api/pkg/store/store_interactions.go` and `api/cmd/helix/serve.go`

**Current behavior:** `ResetRunningInteractions` blanket-errors ALL waiting interactions on startup. This is correct for non-external-agent sessions (where the API itself was processing the interaction), but wrong for `zed_external` sessions where Zed may still be running and will deliver results on reconnect.

**Change:** Exclude `zed_external` interactions from `ResetRunningInteractions`:

```go
func (s *PostgresStore) ResetRunningInteractions(ctx context.Context) error {
    err := s.gdb.WithContext(ctx).Model(&types.Interaction{}).
        Where("state = ?", types.InteractionStateWaiting).
        Where("session_id NOT IN (SELECT id FROM sessions WHERE config->>'agent_type' = 'zed_external')").
        Updates(map[string]any{
            "state": types.InteractionStateError,
            "error": "Interrupted",
        }).Error
    ...
}
```

With this change, `zed_external` waiting interactions survive API restarts. Combined with Change 2 (dedup), this allows:
- If Zed already completed the work: dedup catches it, replays `message_completed`
- If Zed is still working: `pickupWaitingInteraction` re-sends `chat_message`, Zed dedup handles it
- If Zed didn't process it: `pickupWaitingInteraction` sends `chat_message` normally

**Risk:** Medium. Must be paired with Change 4 (timeout) to avoid stuck interactions.

### Change 4: Timeout stuck waiting interactions (Gap 4)

**File:** `api/pkg/server/websocket_external_agent_sync.go`

In `pickupWaitingInteraction`, after queuing the `chat_message` for the agent, start a goroutine with a 120-second timeout:

```go
go func() {
    timer := time.NewTimer(120 * time.Second)
    defer timer.Stop()
    
    select {
    case <-completionCh:
        // message_completed arrived, all good
    case <-timer.C:
        // Timeout - mark interaction as error
        interaction.State = types.InteractionStateError
        interaction.Error = "Response interrupted by system restart"
        store.UpdateInteraction(ctx, interaction)
    case <-ctx.Done():
        // Connection closed
    }
}()
```

The `completionCh` channel is signalled when `handleMessageCompleted` processes the corresponding `request_id`.

**Risk:** Low. This is defense-in-depth. The channel/context ensures the goroutine is always cleaned up.

## Architecture Decisions

1. **Zed-side dedup over API-side protocol** — Adding a new WebSocket command (`check_request_status`) increases protocol complexity. Since Zed has all the state (it knows what it already processed), dedup belongs there.

2. **Re-queue over replay** — For failed sends, re-queuing the single failed event is simpler than implementing a full event replay log. The channel already buffers events queued while disconnected.

3. **No full event journal** — A persistent event journal (write-ahead log) for all outgoing events would guarantee zero loss, but is overkill. The combination of channel buffering + single-event re-queue + dedup covers the practical cases.

4. **Frontend already handles reconnection** — No frontend changes needed. `ReconnectingWebSocket` + query invalidation on reconnect already refreshes the chat view.

5. **Selective ResetRunningInteractions** — Instead of blanket-erroring all waiting interactions, exclude `zed_external` sessions that can recover via Zed reconnection + dedup. This is the key insight from testing: the current approach is too aggressive.

## Codebase Patterns Found

- WebSocket sync events flow: GPUI entity event → `ensure_thread_subscription` callback → `send_websocket_event()` → `outgoing_tx` (mpsc) → `run_connection` select loop → `ws_sink.send()`
- Reconnection loop is in `reconnection_loop()` (`websocket_sync.rs:139`), calls `run_connection()` each iteration with `&mut outgoing_rx`
- `open_existing_thread_sync` has a fast path when thread is already in registry (line 1465) — just re-ensures subscription, no reload
- All persistent state lives in Postgres (sessions, interactions, ZedThreadID). In-memory state (contextMappings, request mappings, streaming contexts) is rebuilt from DB on reconnect.
- `ResetRunningInteractions` is called at API startup (`serve.go:279`) — this is the primary mechanism that handles stuck interactions today
- `flushAndClearStreamingContext` preserves partial responses to DB on reconnect (`websocket_external_agent_sync.go:1500-1520`)
