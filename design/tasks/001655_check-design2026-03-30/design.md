# Design: Fix Truncated Text Entries in Zed→Helix Streaming Sync

## Verified Root Cause

The design doc at `design/2026-03-30-truncated-text-entries-bug.md` is accurate. The bug is confirmed in:

**File:** `zed-4/crates/external_websocket_sync/src/thread_service.rs`, lines 499–518

The `Stopped` handler uses `.rev().find_map()` to locate only the **last** non-empty `AssistantMessage`. All earlier text entries (which had incomplete content during streaming because text was still in the Markdown streaming buffer) never get their corrected content sent.

By the time `Stopped` fires, `flush_streaming_text` has already been called on the `AcpThread`, so **all** entries have complete content — but only the last text entry is re-sent.

## Fix

Replace the single-entry `find_map` lookup with a loop over all entries:

```rust
AcpThreadEvent::Stopped(_) => {
    flush_streaming_throttle(&thread_id_for_sub);

    let thread = thread_entity.read(cx);
    let entries = thread.entries();

    // Send corrected content for ALL entries.
    // EntryUpdated events during streaming carried incomplete content
    // (text was still in the streaming buffer). Now that flush_streaming_text
    // has been called, all entries have their complete content.
    for (idx, entry) in entries.iter().enumerate() {
        match entry {
            acp_thread::AgentThreadEntry::AssistantMessage(msg) => {
                let content = msg.content_only(cx);
                if !content.is_empty() {
                    crate::send_websocket_event(SyncEvent::MessageAdded {
                        acp_thread_id: thread_id_for_sub.clone(),
                        message_id: idx.to_string(),
                        role: "assistant".to_string(),
                        content,
                        entry_type: "text".to_string(),
                        tool_name: String::new(),
                        tool_status: String::new(),
                        timestamp: chrono::Utc::now().timestamp(),
                    }).log_err();
                }
            }
            acp_thread::AgentThreadEntry::ToolCall(tool_call) => {
                let content = tool_call.to_markdown(cx);
                if !content.is_empty() {
                    let name = tool_call.label.read(cx).source().to_string();
                    let status = tool_call.status.to_string();
                    crate::send_websocket_event(SyncEvent::MessageAdded {
                        acp_thread_id: thread_id_for_sub.clone(),
                        message_id: idx.to_string(),
                        role: "assistant".to_string(),
                        content,
                        entry_type: "tool_call".to_string(),
                        tool_name: name,
                        tool_status: status,
                        timestamp: chrono::Utc::now().timestamp(),
                    }).log_err();
                }
            }
            _ => {}
        }
    }

    let rid = crate::get_thread_request_id(&thread_id_for_sub)
        .unwrap_or_default();
    eprintln!(
        "📤 [THREAD_SERVICE] Stopped event: sending message_completed for thread {} (request_id={})",
        thread_id_for_sub, rid
    );
    let _ = crate::send_websocket_event(SyncEvent::MessageCompleted {
        acp_thread_id: thread_id_for_sub.clone(),
        message_id: "0".to_string(),
        request_id: rid,
    });
}
```

## Key Design Notes

**Why the loop is safe:** The Go accumulator uses overwrite semantics for known `message_id`s — re-sending an entry with the same content is a no-op. Cost is slightly more WebSocket messages at turn completion (one per entry), which is negligible.

**Error handling:** The original code uses `let _ = send_websocket_event(...)`, silently discarding errors — violating the zed-4 CLAUDE.md guideline. The fix should use `.log_err()` for visibility. The existing `MessageCompleted` send can keep `let _ =` as a pre-existing pattern or be updated separately.

**Tool call entries:** Including them in the loop ensures their final status is sent even if the last `EntryUpdated` for a tool call was throttled.

**APIs confirmed from existing code (lines 434–474):**
- `msg.content_only(cx)` — AssistantMessage
- `tool_call.to_markdown(cx)` — ToolCall content
- `tool_call.label.read(cx).source().to_string()` — tool name
- `tool_call.status.to_string()` — tool status

## Files to Change

| File | Lines | Change |
|------|-------|--------|
| `zed-4/crates/external_websocket_sync/src/thread_service.rs` | 499–518 | Replace single-entry `find_map` with all-entry loop |
