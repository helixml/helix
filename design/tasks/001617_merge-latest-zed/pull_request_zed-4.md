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

Release Notes:

- N/A
