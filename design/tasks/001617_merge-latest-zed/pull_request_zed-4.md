Merge latest upstream Zed into Helix fork

## Summary

Merges 736 upstream commits from `zed-industries/zed` main into the Helix fork, resolving all conflicts and preserving Helix-specific `external_websocket_sync` changes. Adapts Helix code to upstream's ACP consolidation (retirement of legacy non-ACP native agent).

## Changes

- Resolved all merge conflicts across ~15 upstream-modified files, keeping all `#[cfg(feature = "external_websocket_sync")]` blocks intact
- Updated `from_existing_thread()` in `conversation_view.rs` (previously `acp/thread_view.rs`) to match new upstream API: `EntryViewState::new`, `ThreadView::new`, and `ConnectedServerState` struct changes
- Fixed `agent_panel.rs` cfg-gated block: `ExternalAgent::NativeAgent` → `Agent::NativeAgent`, `AcpServerView::from_existing_thread` → `ConversationView::from_existing_thread`, history now fetched from `connection_store` instead of removed `acp_history` field
- Added `feature = "external_websocket_sync"` to `set_session_list` cfg in `thread_history.rs` so it's callable outside test builds
- Reverted all `.github/workflows/` changes back to pre-merge state — the internal git server rejects pushes that modify this directory (no required GitHub scope)
- Updated `portingguide.md`: ACP consolidation section, `.github/workflows/` revert rule, updated file paths, E2E phase count (4→10), full commit history

## All 4 critical fixes verified present

1. `load_session()` clones `Entity<NativeAgent>` before async spawn (`agent.rs`)
2. `conversation_view.rs` only sends `UserCreatedThread` and `ThreadTitleChanged` (no duplicates)
3. `AssistantMessage::content_only()` exists (`acp_thread.rs`)
4. Follow-up path calls `notify_thread_display()` before sending (`thread_service.rs`)

## Streaming content accumulation fix

Fixed a race condition where short AI responses (e.g. a single token) were
sent to the Go server with empty content, causing `validateStore()` to fail.

**Root cause**: `AcpThreadEvent::EntryUpdated` fires BEFORE
`buffer_streaming_text` appends the new chunk to `StreamingTextBuffer`.
For the first (and possibly only) chunk, `throttled_send_message_added`
captures empty content. `flush_streaming_throttle` on `Stopped` only
re-sends stored pending messages — it does not re-read the now-complete
content. So the Go server's `MessageAccumulator` ends up with empty
`ResponseMessage`/`ResponseEntries`.

**Fix** (`crates/external_websocket_sync/src/thread_service.rs`): in the
`AcpThreadEvent::Stopped` handler, after `flush_streaming_throttle`, iterate
all assistant/tool_call entries and send a fresh `message_added` with
`content_only(cx)` / `to_markdown(cx)`. At `Stopped` time,
`flush_streaming_text` has already been called, so all content is available.
The Go server accumulator uses per-messageID overwrite semantics, so these
final events correctly replace any truncated content sent during streaming.

E2E test (`run_docker_e2e.sh`) passes with both `zed-agent` and `claude`
agent types — all 10 phases pass including store validation.

Release Notes:

- N/A
