# Design: Timer-Based Flush for Streaming Throttles

## Architecture

The streaming pipeline has two throttle layers, both exhibiting the same bug pattern:

```
LLM tokens → Zed (100ms throttle) → WebSocket → Helix (50ms throttle) → Frontend
                 ↑ no drain timer          ↑ no drain timer
```

Both throttles suppress duplicate sends within their interval, but neither has a background timer to flush pending content when events stop arriving. The fix is the same pattern applied to both layers: **a trailing-edge timer that fires once after the throttle interval elapses with no new events**.

## Codebase Findings

### Zed Layer (`zed/crates/external_websocket_sync/src/thread_service.rs`)

- **Throttle state**: `STREAMING_THROTTLE` — global `Mutex<HashMap<String, StreamingThrottleState>>` keyed by `"{thread_id}:{entry_idx}"` (line 96)
- **Throttle logic**: `throttled_send_message_added()` (line 241) — if <100ms since last send, stores content in `pending_content` and returns `sent = false`
- **Existing flush triggers**: `flush_stale_pending_for_thread()` (line 201, on new entry), `flush_streaming_throttle()` (line 353, on Stopped)
- **Missing**: No timer-based drain — `pending_content` stays buffered until next event

### Helix Layer (`helix/api/pkg/server/websocket_external_agent_sync.go`)

- **Throttle state**: `streamingContext.lastPublish` (line 38) — per-session timestamp
- **Throttle logic**: `handleMessageAdded()` (line 1229) — skips publish if <50ms since `lastPublish`
- **Existing flush triggers**: `forceFlushToolCall` (line 1162, on tool_call boundary), `flushAndClearStreamingContext()` (on message_completed)
- **Missing**: No background goroutine/ticker to publish when events pause

## Fix Design

### Layer 1: Zed — Trailing-edge flush timer

When `throttled_send_message_added` buffers content (returns `sent = false`), spawn a delayed task that fires after `STREAMING_THROTTLE_INTERVAL` (100ms). On firing, if `pending_content` still exists for that key, send it and clear the buffer. If a new event arrives first (and sends or replaces the pending content), the timer becomes a no-op.

**Implemented approach**: Used `smol::spawn` with `smol::Timer::after()` since `throttled_send_message_added` is a standalone function without `cx` access. Added a `flush_scheduled: bool` field to `StreamingThrottleState` to prevent spawning duplicate timers. The async `trailing_flush_timer` function sleeps for `STREAMING_THROTTLE_INTERVAL`, then checks under lock if `pending_content` still exists and sends it if so.

The timer is a no-op if the content was already sent or replaced because:
- If the next event sends the content (throttle window elapsed), `pending_content` is cleared → timer finds nothing to flush
- If the next event replaces the content (within throttle window), the new content gets its own timer (only if `flush_scheduled` is false, but the existing timer will pick up the replacement content)

### Layer 2: Helix — Trailing-edge publish goroutine

When `handleMessageAdded` skips a publish due to the 50ms throttle, schedule a `time.AfterFunc` that fires after `publishInterval`. On firing, if `lastPublish` still hasn't been updated (no subsequent event published), compute and publish the pending patches.

**Approach**: Store a `*time.Timer` in `streamingContext`. On each throttled skip, reset/create the timer. On each successful publish, stop the timer. The timer callback grabs the lock, checks if patches are pending, and publishes them.

**Key constraint**: The timer callback runs in a separate goroutine, so it must lock the streaming context. Use the existing session-level mutex that `handleMessageAdded` already uses.

## Decision: Why trailing-edge timers, not a periodic ticker

A periodic ticker (e.g., 50ms ticker that checks all sessions) would add constant overhead even when nothing is streaming. Trailing-edge timers are zero-cost when idle — they only exist during active streaming gaps.

## Risk

Low. The timers are fire-once and self-cancelling. The worst case is a double-publish (timer fires right as the next event publishes), which is harmless because both the Zed WebSocket protocol and the Helix patch protocol are idempotent (overwrite semantics for known message_ids, patch diffing).
