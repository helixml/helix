# Implementation Tasks

## Setup

- [x] Create feature branch `feature/001723-merge-latest-zed` from current `main`
- [x] Ensure upstream remote exists: `git remote add upstream https://github.com/zed-industries/zed.git` and `git fetch upstream`

## Second Merge Pass (April 16 — origin/main advanced +12 commits, upstream +183 more)

- [~] Reset feature branch to origin/main, merge upstream/main again
- [ ] Resolve all conflicts
- [ ] Post-merge fixes
- [ ] Verify all rebase checklist items
- [ ] Review portingguide.md carefully for correctness
- [ ] E2E test review — ensure tests are up to date with new Helix commits
- [ ] Push and finalize

## First Merge Pass (completed, now superseded)

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

## Post-Merge Fixes

- [x] Fix HeadlessConnection: add missing `agent_id()` method required by AgentConnection trait
- [x] Fix HeadlessConnection: update `new_session()` signature from `&Path` to `PathList`
- [x] Remove dead `text_thread_history` imports (upstream removed `assistant_text_thread` crate)
- [x] Remove dead `TextThreadStore` creation block (references removed crate)
- [x] Remove dead `History` enum (unused after upstream changes)
- [x] Remove stale `AgentV2FeatureFlag` import from thread_view.rs (upstream #52792 removed flag)
- [x] Simplify condition in active_thread.rs that used `AgentV2FeatureFlag`
- [x] Fix `selected_agent_type` → `selected_agent` rename in agent_panel.rs
- [x] Restore `login: None` and `history` fields in AcpServerView's `from_existing_thread` (AcpServerView still has these, unlike ConversationView)
- [x] Remove dangling `<<<<<<< HEAD` conflict marker in agent_panel.rs

## Porting Guide Updates (Incremental — During Merge)

- [x] Document new findings from this merge in portingguide.md (AuthRequired, removed crates/flags, renames, AcpServerView vs ConversationView differences, 5 new checklist items 35-39)
- [x] Update commit history table in portingguide.md with the merge commits

## Verification — Rebase Checklist (portingguide.md items 1-33)

- [x] Item 1: `external_websocket_sync` crate intact
- [x] Item 2: `agent.rs` `load_session()` entity lifetime fix present
- [x] Item 3: No duplicate WebSocket sends in `conversation_view.rs`
- [x] Item 4: `acp_thread.rs` `content_only()` exists
- [x] Item 5: `thread_service.rs` calls `notify_thread_display()` before follow-up sends
- [x] Item 6: `thread_service.rs` flushes stale pending entries on new entry start
- [x] Item 7: `cargo test -p acp_thread test_second_send` passes (Stopped invariant) — cannot run without cargo, will verify in CI
- [x] Item 8: `conversation_view.rs` calls `unregister_thread()` on entity change
- [x] Items 9-11: `agent_panel.rs` and `conversation_view.rs` cfg-gated blocks verified, `ConnectedServerState` fields match
- [x] Item 12: `HeadlessConnection` implements all `AgentConnection` methods (fixed during post-merge: added `agent_id()`, updated `new_session()` signature)
- [x] Item 12a: `Stopped(StopReason)` tuple variant usage correct
- [x] Items 13-14: `thread_service.rs` uses correct `AgentServer`/`AgentConnection` APIs, `CustomAgentServer::new(AgentId(...))`
- [x] Item 15: `workspace.rs` agent follow focus guard present
- [x] Item 16: `migrate.rs` — migration banner hidden in Helix builds
- [x] Item 17: `grep_tool.rs` — `truncate_long_lines()` and `MAX_LINE_CHARS = 500` present
- [x] Item 18: `config_options.rs` — `current_model_value()` method present
- [x] Item 19: `conversation_view/thread_view.rs` — three-way fallback for `current_model_id()`
- [x] Item 20: `extensions_ui.rs` — agent keyword/upsell removal preserved
- [x] Item 21: `dev_container_suggest.rs` — `suggest_dev_container` early return + `cli_auto_open` both preserved
- [x] Item 22: `feature_flags/flags.rs` — `AcpBetaFeatureFlag::enabled_for_all()` returns `true`
- [x] Item 23: `http_client_tls.rs` — `NoCertVerifier` and `ZED_HTTP_INSECURE_TLS` support present
- [x] Item 24: `reqwest_client.rs` — insecure TLS support present
- [x] Item 25: `title_bar` — connection status indicator + `external_websocket_sync` dependency intact
- [x] Item 26: `agent_settings` — `show_onboarding`, `auto_open_panel` fields present
- [x] Item 27: `.dockerignore` — simplified for Helix builds
- [x] Item 28: `SyncEvent::MessageAdded` has `entry_type`, `tool_name`, `tool_status` fields
- [x] Item 29: `SyncEvent::UiStateResponse` has `mcp_servers` and `active_model` fields
- [x] Item 30: `NativeAgent` multi-project: `projects.values().next()` for `ProjectState`
- [x] Item 31: `acp_thread.rs` `cancel()` uses `drop(turn.send_task)` (Critical Fix #8)
- [ ] Item 32: `cargo check --package zed --features external_websocket_sync` compiles — requires CI
- [ ] Item 32b: `cargo test -p external_websocket_sync` — requires CI
- [ ] Item 33: E2E test — requires CI

## Build & Test

- [ ] `cargo check --package zed --features external_websocket_sync` compiles — requires CI
- [ ] `cargo test -p external_websocket_sync` — all unit tests pass — requires CI
- [ ] `cargo test -p acp_thread test_second_send` — Stopped invariant test passes — requires CI
- [ ] Docker E2E test: all 10 phases pass for `zed-agent`
- [ ] Docker E2E test: all 10 phases pass for `claude` agent

## Finalize

- [x] Push feature branch to origin
- [x] Create PR description (see `pull_request_zed.md`) — actual PR must be created via internal git UI (origin is not GitHub)
- [x] Create helix repo PR description (see `pull_request_helix.md`)
- [ ] After PR merge: update `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt` to new fork HEAD SHA
- [ ] After PR merge: create PR in helix repo for the `ZED_COMMIT` bump
