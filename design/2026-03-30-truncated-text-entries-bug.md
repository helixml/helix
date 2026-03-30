# Bug: Truncated Text Entries in Zed→Helix Streaming Sync

**Date**: 2026-03-30
**Observed in**: Session `spt_01kmyy3hsz5nqdwdg1avqfzk4p` / `ses_01kmyy3q2x26m9j0b4jg1j3zqa`
**Interaction**: `int_01kmz0bbdtgqzs5r1m9nrngxf8`

## Symptom

Text entries in the interaction's `response_entries` JSON are truncated mid-sentence. They all share the same pattern: a text entry ends abruptly, and the next entry is a `tool_call`.

### Examples from the interaction

| Entry | Content ending (truncated) | Next entry |
|-------|---------------------------|------------|
| 0 | `"So tapping moves the "` | tool_call: Read file |
| 35 | `"Now add \`touchMode\` to the \`ha"` | tool_call: Read file |
| 38 | `"Now commit and "` | tool_call: git commit |

Entry 44 (the final text entry) has complete content — it's only the **intermediate** text entries (those followed by tool calls) that are truncated.

## Root Cause Analysis

The bug is in the Zed-side `Stopped` event handler in `zed/crates/external_websocket_sync/src/thread_service.rs` (lines 489-530).

### How streaming works

1. Each assistant entry (text or tool_call) gets its own `entry_idx` used as `message_id`
2. `EntryUpdated` events fire during streaming with 100ms throttle
3. Text content is buffered in Zed's streaming buffer — `content_only()` may return **incomplete content** during streaming because the Markdown entity hasn't been flushed yet (comment at line 492-496 confirms this)
4. When a new entry starts streaming (e.g., tool_call after text), the throttle correctly flushes the **pending** content for the previous entry

### The problem: pending content ≠ complete content

The throttle flush at entry transition (lines 191-198) sends whatever was last buffered in `pending_content` for the old entry. But this pending content was captured from a `content_only()` call during streaming — **before** `flush_streaming_text` was called on the AcpThread. The text was still in the streaming buffer, not yet materialized into the Markdown entity.

So the flushed content is the last throttled snapshot, which is mid-word/mid-sentence.

### The Stopped handler only fixes the LAST text entry

```rust
// lines 499-507
if let Some((final_idx, final_content)) = entries.iter().enumerate().rev().find_map(|(idx, entry)| {
    if let acp_thread::AgentThreadEntry::AssistantMessage(msg) = entry {
        let content = msg.content_only(cx);
        if !content.is_empty() {
            return Some((idx, content));
        }
    }
    None
}) {
    // sends corrected content with message_id = final_idx
}
```

This uses `.rev().find_map()` — it finds only the **last** non-empty `AssistantMessage` and sends its corrected content. All earlier text entries that were truncated during streaming never get their corrected content sent.

By the time `Stopped` fires, `flush_streaming_text` has been called on the AcpThread, so ALL text entries now have complete content in their Markdown entities. But only the last one gets sent.

## Fix

In the `Stopped` handler, iterate over **all** entries (not just the last AssistantMessage) and send corrected content for each:

```rust
AcpThreadEvent::Stopped(_) => {
    flush_streaming_throttle(&thread_id_for_sub);

    let thread = thread_entity.read(cx);
    let entries = thread.entries();

    // Send corrected content for ALL entries, not just the last text entry.
    // During streaming, EntryUpdated events carried incomplete content
    // (text was still in the streaming buffer). Now that flush_streaming_text
    // has been called, all entries have their complete content.
    for (idx, entry) in entries.iter().enumerate() {
        match entry {
            acp_thread::AgentThreadEntry::AssistantMessage(msg) => {
                let content = msg.content_only(cx);
                if !content.is_empty() {
                    let _ = crate::send_websocket_event(SyncEvent::MessageAdded {
                        acp_thread_id: thread_id_for_sub.clone(),
                        message_id: idx.to_string(),
                        role: "assistant".to_string(),
                        content,
                        entry_type: "text".to_string(),
                        tool_name: String::new(),
                        tool_status: String::new(),
                        timestamp: chrono::Utc::now().timestamp(),
                    });
                }
            }
            acp_thread::AgentThreadEntry::ToolCall(tool_call) => {
                let content = tool_call.to_markdown(cx);
                if !content.is_empty() {
                    let name = tool_call.label.read(cx).source().to_string();
                    let status = tool_call.status.to_string();
                    let _ = crate::send_websocket_event(SyncEvent::MessageAdded {
                        acp_thread_id: thread_id_for_sub.clone(),
                        message_id: idx.to_string(),
                        role: "assistant".to_string(),
                        content,
                        entry_type: "tool_call".to_string(),
                        tool_name: name,
                        tool_status: status,
                        timestamp: chrono::Utc::now().timestamp(),
                    });
                }
            }
            _ => {}
        }
    }

    // Then send message_completed as before
    let rid = crate::get_thread_request_id(&thread_id_for_sub)
        .unwrap_or_default();
    let _ = crate::send_websocket_event(SyncEvent::MessageCompleted {
        acp_thread_id: thread_id_for_sub.clone(),
        message_id: "0".to_string(),
        request_id: rid,
    });
}
```

### Why this is safe

- The accumulator on the Go side uses overwrite semantics for known `message_id`s — re-sending an entry that already has the correct content is a no-op (the content is replaced with the same value)
- The only cost is slightly more WebSocket messages at turn completion (one per entry instead of one), which is negligible since this happens once per turn
- Tool call entries also benefit — their final status is guaranteed to be sent even if the last `EntryUpdated` was throttled

## Files to change

| File | Change |
|------|--------|
| `zed/crates/external_websocket_sync/src/thread_service.rs` lines 489-530 | Replace single-entry final flush with all-entry loop |

## How to verify

1. Start a spectask, send a message that triggers tool calls
2. After completion, query the interaction's `response_entries`:
   ```sql
   SELECT response_entries::text FROM interactions WHERE id = '<interaction_id>';
   ```
3. Check that all text entries have complete sentences (no trailing spaces or mid-word truncation)
4. Run the E2E test suite to ensure no regressions

## DB evidence

```sql
-- Find the truncated entries
SELECT response_entries::text FROM interactions
WHERE id = 'int_01kmz0bbdtgqzs5r1m9nrngxf8';

-- Entry 0:  ends with "So tapping moves the " (truncated, next is tool_call)
-- Entry 35: ends with "Now add `touchMode` to the `ha" (truncated)
-- Entry 38: ends with "Now commit and " (truncated)
-- Entry 44: complete (last text entry, gets corrected by Stopped handler)
```
