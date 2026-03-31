# Requirements: Fix Truncated Text Entries in Zed→Helix Streaming Sync

## Background

During Zed→Helix streaming sync, intermediate text entries (those followed by tool calls) were saved truncated in the interaction's `response_entries`. The final text entry was correct because the old `Stopped` handler only corrected the last AssistantMessage. All earlier ones kept their streaming-snapshot content.

## Status

**The primary fix is already applied.** The `Stopped` handler in `thread_service.rs` now loops over all entries (not just the last). Two cleanup items remain.

## User Stories

**As a user reviewing a spectask interaction**, I expect all text entries in `response_entries` to contain complete sentences, not content truncated mid-word at a tool-call boundary.

**As a developer**, I expect the codebase to follow the error-handling conventions in CLAUDE.md (no silent `let _ =` discards, no debug `eprintln!` in production code).

## Acceptance Criteria

- [ ] All `AssistantMessage` entries in `response_entries` have complete content after a turn completes
- [ ] The `MessageCompleted` send uses `.log_err()` instead of `let _ =`
- [ ] The debug `eprintln!` in the `Stopped` handler is removed
- [ ] Existing unit tests and E2E test pass
