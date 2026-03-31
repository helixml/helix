# Design: Fix Truncated Text Entries

## Root Cause (from bug report)

During streaming, `EntryUpdated` events fire with 100ms throttle. When a new entry starts (e.g., a tool_call after text), the throttle flushes the last buffered snapshot for the old entry — but that snapshot was taken before `flush_streaming_text` materialized the Markdown entity. So intermediate text entries end up with truncated content.

The old `Stopped` handler used `.rev().find_map()` to send corrected content for only the **last** `AssistantMessage`. Earlier truncated entries were never corrected.

## Fix Already Applied

`zed/crates/external_websocket_sync/src/thread_service.rs` lines 489–536 now loop over **all** entries and re-send `MessageAdded` for each `AssistantMessage` and `ToolCall`. By the time `Stopped` fires, `flush_streaming_text` has been called, so all Markdown entities have complete content.

The Go accumulator uses overwrite semantics for known `message_id`s, so re-sending is a no-op when content is already correct (the final entry). Cost: a few extra WebSocket messages per turn — negligible.

## Remaining Cleanup

Two issues in the `Stopped` handler violate CLAUDE.md coding conventions:

1. **Line 544** — `let _ = crate::send_websocket_event(SyncEvent::MessageCompleted { ... })` silently discards the error. Should be `.log_err()` for visibility, consistent with the `MessageAdded` sends in the loop above.

2. **Lines 540–543** — Debug `eprintln!` logging the "Stopped event" was left in production code. Should be removed.

## File to Change

`zed/crates/external_websocket_sync/src/thread_service.rs` lines 540–548 only.
