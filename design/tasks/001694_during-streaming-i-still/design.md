# Design: Fix Truncated Sentences Before Tool Calls During Streaming

## Root Cause: Zed Sends Tool Call Entry Before Final Text Content

### Evidence from Logs

Captured during a real streaming session (inner Helix, 2026-04-04T12:47:00Z):

```
12:47:00Z 📝 [HELIX] New distinct message, last_message_id=1, new_message_id=2  ← tool call entry arrives
12:47:00Z 📝 Updated in-memory (content_length=122), db_written=false
12:47:03Z 📝 [HELIX] New distinct message, last_message_id=2, new_message_id=1  ← text entry's final content arrives AFTER
12:47:03Z 📝 Updated in-memory (content_length=122), db_written=true
12:47:03Z 📤 Published entry patches to frontend, entry_count=2, entry_patches=1
```

**Key observation**: `message_id=2` (the tool call) arrived at T=0. `message_id=1` (the preceding text entry's corrected content) arrived 3 seconds later. The message order was reversed: tool call → then text correction.

This is the "Stopped flush" that Zed's code comment describes (`thread_service.rs:526-529`):
> AcpThread calls flush_streaming_text before emitting Stopped, so all Markdown entities now have their complete buffered text. Send corrected content for ALL entries — EntryUpdated events during streaming carried incomplete content.

But this flush happens at **turn end**, not at **tool call boundary**. During streaming, the tool call entry is sent immediately when it's created, while the preceding text entry's content is still stuck in Zed's 100ms throttle buffer.

### The Exact Throttle Constants

| Layer | Constant | Value | Location |
|-------|----------|-------|----------|
| Zed | `STREAMING_THROTTLE_INTERVAL` | 100ms | `thread_service.rs:71` |
| Helix API | `publishInterval` | 50ms | `websocket_external_agent_sync.go:58` |

### Why This Happens

1. **T=0ms**: Zed receives text tokens, `EntryUpdated(0)` fires, throttle allows send
2. **T=50ms**: More tokens arrive, throttle says "too soon" — content stored in `pending_content`
3. **T=60ms**: Tool call begins. Zed creates entry idx=1, `EntryUpdated(1)` fires.
   - This is a **different entry**, so its throttle fires immediately
   - Zed's stale-pending flush (lines 196-207) sends entry 0's pending content **before** entry 1
   - BUT: Helix API's 50ms `publishInterval` may batch or delay the text patch
4. **Frontend**: Sees `entry_count=2`, starts rendering tool call, but text entry is incomplete

### Zed's Stale-Pending Flush Already Exists (But Isn't Enough)

Zed's `throttled_send_message_added` (`thread_service.rs:196-207`) does flush pending content for **other** entries when a new entry arrives:

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

This sends the text entry's pending content **on the wire** before the tool call. However:
1. The Helix API's 50ms `publishInterval` may hold the patch in its buffer
2. The frontend may receive and render both patches in the same frame, but sees `entry_count=2` before entry 0 is complete

## Fix Strategy

### Option A: Force-Flush in Helix API (Recommended)

When Helix receives a `message_added` event with a **higher entry index** than the current max, force-flush all pending patches immediately (bypass the 50ms throttle) before processing the new entry.

**Location**: `handleMessageAdded` in `websocket_external_agent_sync.go`, around line 1186.

**Why this works**: The Go accumulator has the final text content from Zed's stale-pending flush. The issue is that the 50ms throttle holds it. Force-flushing at entry transition ensures the frontend sees complete content before `entry_count` grows.

### Option B: Log Entry Type in Helix (For Debugging)

Currently, the logs don't show `entry_type`. Add logging like:
```go
log.Debug().Str("entry_type", entryType).Int("entry_idx", entryIdx)...
```

This would make future debugging easier to correlate which messages are tool calls.

## Key Files

| File | Line | Purpose |
|------|------|---------|
| `zed-4/crates/external_websocket_sync/src/thread_service.rs` | 71 | `STREAMING_THROTTLE_INTERVAL = 100ms` |
| `zed-4/crates/external_websocket_sync/src/thread_service.rs` | 196-207 | Stale-pending flush for other entries |
| `helix-4/api/pkg/server/websocket_external_agent_sync.go` | 58 | `publishInterval = 50ms` |
| `helix-4/api/pkg/server/websocket_external_agent_sync.go` | 1184-1197 | Throttled frontend publish |
| `helix-4/api/pkg/server/websocket_external_agent_sync.go` | 1152 | "New distinct message detected" log |

## Implementation Notes

### What Was Implemented
The primary fix (Option A) was implemented in the Helix API. When a tool_call entry arrives:
1. Detect the condition: new message_id + entry_type=tool_call + existing entries
2. Force-flush pending patches by calling `publishEntryPatchesToFrontend` immediately
3. Reset `lastPublish` so the throttle window restarts cleanly
4. Log the event for debugging: `"📤 [FLUSH] Force-published patches before tool_call entry"`

### Key Implementation Decision
Initially tracked `maxEntryCount` to avoid redundant flushes, but this was over-engineering. The simpler approach: always force-flush before any tool_call entry when there are existing entries. Tool calls are infrequent enough that the overhead is negligible.

### Verification
Logs confirmed the fix triggers correctly:
```
📤 [FLUSH] Force-published patches before tool_call entry entry_count=1 entry_type=tool_call
📤 [FLUSH] Force-published patches before tool_call entry entry_count=2 entry_type=tool_call
📦 [FLUSH] Published final corrected entry patches to frontend before completion
```

### Zed-Side Fix (Secondary)
The API-side fix alone was insufficient — the user still observed truncation. Root cause: the `NewEntry` handler in Zed sends new entries (including tool_calls) directly via `send_websocket_event` *without* flushing pending throttled content for other entries. The stale-pending flush only existed in `throttled_send_message_added`, which `NewEntry` doesn't call.

Two changes in `thread_service.rs`:
1. **`flush_stale_pending_for_thread()` helper**: Extracted the stale-pending flush logic into a standalone function. Flushes pending throttled content for all entries in a thread except the specified one.
2. **`NewEntry` handler**: Calls `flush_stale_pending_for_thread()` before sending the new entry. This ensures the preceding text entry's final content reaches the API before the tool_call entry.
3. **`throttled_send_message_added`**: Tool call entries (`entry_type == "tool_call"`) bypass the 100ms throttle entirely. This ensures tool_call status updates (e.g. "In Progress" → "Completed") are sent immediately.
