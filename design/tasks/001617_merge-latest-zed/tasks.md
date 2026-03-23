# Implementation Tasks

## Preparation

- [x] Create branch: `git checkout -b feature/001617-merge-latest-zed`
- [x] Update `portingguide.md` with the branch naming convention (the internal git server requires `feature/<task-id>-*` — do not use date-based names)
- [x] Add upstream Zed remote: `git remote add upstream https://github.com/zed-industries/zed && git fetch upstream`
- [x] Record current upstream HEAD SHA and compare with fork's base commit to understand the delta
  - Merge base: `7cca7bc6d6`, 736 commits behind upstream, 87 Helix-specific commits on branch
  - Using merge commit strategy (same as previous upstream merges in this fork's history)

## Merge

- [x] Rebase (or merge) fork onto `upstream/main`: `git merge upstream/main`
- [x] Resolve conflicts file by file, keeping all `#[cfg(feature = "external_websocket_sync")]` blocks

## ACP Consolidation Rename Tracking

- [x] Identify whether upstream renamed `acp_thread` crate or `crates/agent_ui/src/acp/` path — find the new locations before applying critical fixes
  - `acp_thread` crate still exists unchanged
  - `crates/agent_ui/src/acp/thread_view.rs` renamed to `crates/agent_ui/src/conversation_view.rs`
  - `AcpThreadHistory` → `ThreadHistory` (crates/agent_ui/src/thread_history.rs)
  - `ExternalAgent` → `Agent` (consolidated enum in agent_ui.rs)
- [x] Update all portingguide.md file path references to match new upstream names

## Critical Fix Verification (post-merge, before tests)

- [x] Verify Critical Fix #1 is present: `load_session()` clones `Entity<NativeAgent>` before spawning async task (`crates/agent/src/agent.rs`)
- [x] Verify Critical Fix #2 is present: `conversation_view.rs` does NOT send `MessageAdded`/`MessageCompleted` WebSocket events (only `UserCreatedThread` and `ThreadTitleChanged`)
- [x] Verify Critical Fix #3 is present: `AssistantMessage::content_only()` exists in `crates/acp_thread/src/acp_thread.rs`
- [x] Verify Critical Fix #4 is present: follow-up path in `thread_service.rs` calls `notify_thread_display()` before sending message

## Structural Compatibility Check

- [x] Check `ConnectedServerState` fields in `conversation_view.rs` — upstream added `history: Option<Entity<ThreadHistory>>` and `_connection_entry_subscription`; updated `from_existing_thread()` to match
- [x] Check `from_existing_thread()` parameter API — updated to new `EntryViewState::new` and `ThreadView::new` signatures
- [x] Check `agent_panel.rs` cfg-gated blocks are intact: callback setup, `from_existing_thread`, onboarding dismissal, `acp_history_store()`
- [x] Verify all items 1–18 of the portingguide.md Rebase Checklist

## Test Validation

- [~] `cargo check --package zed --features external_websocket_sync` — must compile with no errors (running in CI — cargo not available locally)
- [ ] `cargo test -p external_websocket_sync` — all unit and protocol-level tests pass
- [ ] Run E2E test: `ANTHROPIC_API_KEY=<key> crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` — all 10 phases must pass

## Porting Guide Update

- [x] Fix stale E2E phase count: updated to reflect 10-phase suite and `run_docker_e2e.sh` script name
- [x] Update file path references (thread_view.rs → conversation_view.rs)
- [x] Document ACP consolidation renames in new "ACP Consolidation" section
- [x] Document `from_existing_thread` API changes and `set_session_list` cfg fix
- [x] Update the Commit History table with new Helix-specific commits

## Finalize

- [x] Push branch: `git push origin feature/001617-merge-latest-zed`
- [ ] Open PR against zed-4 `main` (awaiting CI pass)
- [ ] After zed-4 PR is merged, update `ZED_COMMIT` in `/home/retro/work/helix-4/sandbox-versions.txt` to the new zed-4 `main` HEAD SHA
- [ ] Open PR against helix-4 `main` with the `sandbox-versions.txt` change
