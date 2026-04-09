# Implementation Tasks

## Setup

- [x] Create feature branch `feature/001723-merge-latest-zed` from current `main`
- [x] Ensure upstream remote exists: `git remote add upstream https://github.com/zed-industries/zed.git` and `git fetch upstream`

## Merge

- [x] Run `git merge upstream/main` and begin conflict resolution
- [x] Resolve conflicts in `Cargo.toml` (workspace root) — auto-merged, no conflict
- [x] Resolve conflicts in `crates/zed/Cargo.toml` — kept external_websocket_sync + upstream track-project-leak
- [x] Resolve conflicts in `crates/zed/src/zed.rs` — auto-merged, no conflict
- [x] Resolve conflicts in `crates/agent_ui/src/agent_panel.rs` — preserved all cfg-gated WebSocket blocks, migrated onboarding, added agent_layout_onboarding
- [x] Resolve conflicts in `crates/agent_ui/src/conversation_view.rs` — auto-merged, no conflict
- [x] Resolve conflicts in `crates/acp_thread/src/acp_thread.rs` — auto-merged, no conflict
- [x] Resolve conflicts in `crates/agent/src/agent.rs` — auto-merged, no conflict
- [x] Resolve conflicts in `crates/acp_thread/src/connection.rs` — auto-merged, no conflict
- [x] Resolve conflicts in `crates/workspace/src/workspace.rs` — added new upstream re-exports
- [x] Resolve any other conflicting files (context_server_registry.rs AuthRequired, title_bar connection indicator, dev_container_suggest, workflows, keymaps, editor)

## Porting Guide Updates (Incremental — During Merge)

- [ ] Document any new file renames discovered during conflict resolution
- [ ] Document any new `AgentConnection` trait methods that required `HeadlessConnection` updates
- [ ] Document any new `ConnectedServerState` fields required by `from_existing_thread()`
- [ ] Document any upstream API signature changes (agent_servers, connection, thread types)
- [ ] Add new rebase checklist items for any patterns not covered by existing items 1-33
- [ ] Update commit history table in portingguide.md with the merge commit

## Verification — Rebase Checklist (portingguide.md items 1-33)

- [ ] Item 1: `external_websocket_sync` crate intact
- [ ] Item 2: `agent.rs` `load_session()` entity lifetime fix present
- [ ] Item 3: No duplicate WebSocket sends in `conversation_view.rs`
- [ ] Item 4: `acp_thread.rs` `content_only()` exists
- [ ] Item 5: `thread_service.rs` calls `notify_thread_display()` before follow-up sends
- [ ] Item 6: `thread_service.rs` flushes stale pending entries on new entry start
- [ ] Item 7: `cargo test -p acp_thread test_second_send` passes (Stopped invariant)
- [ ] Item 8: `conversation_view.rs` calls `unregister_thread()` on entity change
- [ ] Items 9-11: `agent_panel.rs` and `conversation_view.rs` cfg-gated blocks verified, `ConnectedServerState` fields match
- [ ] Item 12: `HeadlessConnection` implements all `AgentConnection` methods
- [ ] Item 12a: `Stopped(StopReason)` tuple variant usage correct
- [ ] Items 13-14: `thread_service.rs` uses correct `AgentServer`/`AgentConnection` APIs, `CustomAgentServer::new(AgentId(...))`
- [ ] Item 15: `workspace.rs` agent follow focus guard present
- [ ] Items 16-27: All remaining checklist items verified (migrate.rs, grep_tool.rs, config_options.rs, extensions_ui.rs, etc.)
- [ ] Items 28-31: SyncEvent fields, UiStateResponse fields, NativeAgent multi-project, cancel() drop
- [ ] Items 32-33: Compilation and unit test commands documented

## Build & Test

- [ ] `cargo check --package zed --features external_websocket_sync` compiles
- [ ] `cargo test -p external_websocket_sync` — all unit tests pass
- [ ] `cargo test -p acp_thread test_second_send` — Stopped invariant test passes
- [ ] Docker E2E test: all 10 phases pass for `zed-agent`
- [ ] Docker E2E test: all 10 phases pass for `claude` agent

## Finalize

- [ ] Push feature branch to origin
- [ ] Create PR against `main` with summary of notable upstream changes (breaking changes, new features, deprecations)
- [ ] After PR merge: update `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt` to new fork HEAD SHA
- [ ] After PR merge: create PR in helix repo for the `ZED_COMMIT` bump
