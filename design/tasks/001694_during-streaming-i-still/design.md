# Design: Fix Truncated Sentences Before Tool Calls During Streaming

## Root Cause: Per-Entry Throttling + Tool Call Transition

The "~100ms" issue is not fuzzy — it's the **exact** `STREAMING_THROTTLE_INTERVAL = 100ms` constant in Zed (`thread_service.rs:71`).

### The Problem

When Zed transitions from a **text entry** to a **tool call entry**, the text entry's final content can be stuck in the throttle buffer:

1. **T=0ms**: Zed receives text tokens, emits `EntryUpdated(0)`. The 100ms throttle fires, sends `message_added` to Helix.
2. **T=50ms**: More tokens arrive. Throttle says "too soon" — content stored in `pending_content`, not sent.
3. **T=60ms**: Tool call begins. Zed creates a new entry (idx=1), emits `EntryUpdated(1)`. This is a **different entry**, so its throttle fires immediately — sends `message_added` for the tool call.
4. **Result**: Helix receives the tool call `message_added` **before** the text entry's final content. The frontend sees entry_count=2 and shows the tool call, but entry[0] is still missing its last ~50ms of text.

### Why The Existing Flush-On-Entry-Change Doesn't Fully Work

Zed's `throttled_send_message_added` has code at lines 196-207 that **does** try to flush pending content for **other** entries when a new entry arrives:

```rust
// Flush pending content for all OTHER entries in this thread.
for (k, state) in map.iter_mut() {
    if k.starts_with(&thread_prefix) && *k != key {
        if let Some(pending) = state.pending_content.take() {
            stale_pending.push(pending);
        }
    }
}
```

However, there are **two issues**:

**Issue A**: The stale pending messages are sent **before** the current entry, but the Helix API's **50ms publish throttle** (`publishInterval = 50ms` at `websocket_external_agent_sync.go:58`) may batch them with the tool call — or delay the text patch while the tool call patch goes through.

**Issue B**: The Zed streaming text buffer (`streaming_text_buffer` in `acp_thread.rs`) is separate from the throttle buffer. `flush_streaming_text()` empties the **streaming buffer** into the Markdown entity, then `EntryUpdated` fires, which hits the **throttle logic**. If the throttle says "too soon", the flushed content sits in `pending_content` and isn't sent until the turn ends (`Stopped` event).

### Verification

The 100ms throttle is at `zed-4/crates/external_websocket_sync/src/thread_service.rs:71`:
```rust
const STREAMING_THROTTLE_INTERVAL: Duration = Duration::from_millis(100);
```

The 50ms publish throttle is at `helix-4/api/pkg/server/websocket_external_agent_sync.go:58`:
```go
publishInterval = 50 * time.Millisecond
```

## Fix Strategy

### Option A: Force-Flush in Helix API (Recommended)

When Helix receives a `message_added` event with `entry_type=tool_call` for a **new entry index** (higher than the current max), immediately call `publishEntryPatchesToFrontend` for all pending patches **before** processing the tool call. This ensures the frontend sees the complete text entry before the tool call appears.

**Location**: `handleMessageAdded` in `websocket_external_agent_sync.go`, around line 1186.

**Why this works**: The Go accumulator already has the final text content from the stale-pending flush that Zed sent. The issue is that Helix's 50ms publish throttle holds it. Force-flushing bypasses the throttle at the critical moment.

### Option B: Flush Throttle When Entry Type Changes (Zed)

In `throttled_send_message_added`, after flushing stale pending entries (lines 196-207), also **send** the current entry immediately (bypass the throttle) if:
- The current entry is `entry_type=tool_call`
- The previous entry was `entry_type=text`

This ensures the text entry's final content is on the wire before the tool call's first `message_added`.

### Option C: Both (Belt and Suspenders)

Apply both fixes. Option A is the safety net on the server side; Option B ensures correct ordering on the wire even before Helix sees the events.

## Key Files

| File | Line | Purpose |
|------|------|---------|
| `zed-4/crates/external_websocket_sync/src/thread_service.rs` | 71 | `STREAMING_THROTTLE_INTERVAL = 100ms` |
| `zed-4/crates/external_websocket_sync/src/thread_service.rs` | 171-274 | `throttled_send_message_added` — the throttle logic |
| `zed-4/crates/external_websocket_sync/src/thread_service.rs` | 196-207 | Stale-pending flush for other entries |
| `zed-4/crates/acp_thread/src/acp_thread.rs` | 1631-1642 | `flush_streaming_text` — flushes streaming buffer to Markdown |
| `helix-4/api/pkg/server/websocket_external_agent_sync.go` | 58 | `publishInterval = 50ms` |
| `helix-4/api/pkg/server/websocket_external_agent_sync.go` | 1184-1197 | Throttled frontend publish |
| `helix-4/api/pkg/server/websocket_external_agent_sync.go` | 1468-1490 | `flushAndClearStreamingContext` — final unthrottled publish |

## Recommendation

Implement **Option A** (Helix API force-flush) as the primary fix. It's a single, contained change in Go that handles the race regardless of Zed's internal timing. Option B can be added later as an optimization.
