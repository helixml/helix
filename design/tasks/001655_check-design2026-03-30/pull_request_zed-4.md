Fix truncated text entries in Stopped event handler

## Summary

Intermediate assistant text entries in `response_entries` were truncated mid-sentence when a turn contained multiple text blocks interleaved with tool calls. Only the final text entry received corrected content; all earlier ones were cut off.

The `Stopped` handler used `.rev().find_map()` to locate only the last `AssistantMessage` and re-send its content. By the time `Stopped` fires, `flush_streaming_text` has been called so all entries have complete content — but only the last one was being sent.

## Changes

- `crates/external_websocket_sync/src/thread_service.rs`: Replace the single-entry `find_map` loop with a `for` loop over all entries that re-sends every `AssistantMessage` and `ToolCall` with their now-complete content
- Use `.log_err()` instead of `let _ =` per CLAUDE.md error handling guidelines

Re-sending is safe because the Go accumulator uses overwrite semantics for known `message_id`s.

Release Notes:

- Fixed truncated intermediate text entries in Helix interaction history
