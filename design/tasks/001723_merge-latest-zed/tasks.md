# Implementation Tasks

## Setup

- [~] Create feature branch `feature/001723-merge-latest-zed` from current `main`
- [~] Ensure upstream remote exists: `git remote add upstream https://github.com/zed-industries/zed.git` and `git fetch upstream`

## Merge

- [ ] Run `git merge upstream/main` and begin conflict resolution
- [ ] Resolve conflicts in `Cargo.toml` (workspace root) — preserve `external_websocket_sync` member and dependency
- [ ] Resolve conflicts in `crates/zed/Cargo.toml` — preserve `external_websocket_sync` feature flag and optional dep
- [ ] Resolve conflicts in `crates/zed/src/zed.rs` — preserve cfg-gated WebSocket sync init block
- [ ] Resolve conflicts in `crates/agent_ui/src/agent_panel.rs` — preserve all 6 cfg-gated blocks: thread display callback (with correct `ConnectedServerState` fields), UI state query callback (match on `conversation_view` not `server_view`), thread creation callback, thread open callback, onboarding dismissal, `acp_history_store()` accessor, entity-level split-brain detection, auto-follow activation, history from `connection_store`
- [ ] Resolve conflicts in `crates/agent_ui/src/conversation_view.rs` — preserve `HeadlessConnection` (must implement any new `AgentConnection` methods), `from_existing_thread()` constructor, `THREAD_REGISTRY` integration, history refresh via `self.history()` method, `is_resume = load_session_id.is_some()`, `unregister_thread()` on reset/drop
- [ ] Resolve conflicts in `crates/acp_thread/src/acp_thread.rs` — preserve `content_only()` on `AssistantMessage`, verify `Stopped` is still a tuple variant `Stopped(StopReason)`, preserve `cancel()` using `drop(turn.send_task)` not `cx.background_spawn`
- [ ] Resolve conflicts in `crates/agent/src/agent.rs` — preserve `load_session()` entity lifetime fix (clone `Entity<NativeAgent>`), verify multi-project `projects.values().next()` pattern
- [ ] Resolve conflicts in `crates/acp_thread/src/connection.rs` — verify `AgentConnection` trait; if new methods added, update `HeadlessConnection` in `conversation_view.rs`
- [ ] Resolve conflicts in `crates/workspace/src/workspace.rs` — re-apply `CollaboratorId::Agent` focus-stealing guard in `follow()` and `update_follower_items()`
- [ ] Resolve any other conflicting files (feature_flags, extensions_ui, title_bar, agent_settings, etc.)

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
