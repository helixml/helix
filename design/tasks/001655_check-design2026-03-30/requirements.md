# Requirements: Fix Truncated Text Entries in Zed→Helix Streaming Sync

## Problem

Intermediate text entries in `response_entries` are truncated mid-sentence when an assistant turn contains multiple text blocks interleaved with tool calls. Only the final text entry receives its complete content; earlier ones are cut off.

## User Stories

**As a Helix user**, when I view a completed interaction, I expect all assistant text entries to contain complete sentences — not text cut off mid-word.

**As a developer debugging interactions**, I expect `response_entries` in the DB to faithfully represent what the assistant said, so I can trust query results.

## Acceptance Criteria

- [ ] After an assistant turn completes, all text entries in `response_entries` contain full content (no trailing spaces or mid-word truncation)
- [ ] Tool call entries in `response_entries` reflect their final status
- [ ] The fix does not cause duplicate/spurious entries — re-sending a known `message_id` with the same content is a no-op on the Go side
- [ ] No regression in the E2E test suite
- [ ] The fix applies only to `AssistantMessage` and `ToolCall` entries in the `Stopped` handler; user messages are unaffected
