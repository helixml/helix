# Implementation Tasks

## Setup

- [x] Create feature branch `feature/001723-merge-latest-zed` from current `main`
- [x] Ensure upstream remote exists

## Second Merge Pass (April 16 — origin/main +12 commits, upstream +183 more)

- [x] Reset feature branch to origin/main, merge upstream/main (692 total upstream commits)
- [x] Resolve all 29 conflicts (workflows, keymaps, settings, Cargo.tomls, agent_panel, conversation_view, workspace, title_bar, acp_thread, context_server_registry, dev_container_suggest, editor)
- [x] Post-merge fixes: ActiveView→BaseView rename, root_session_id, AuthRequired, selected_agent
- [x] Verify all 8 critical fixes survived (entity lifetime, no dups, content_only, notify_thread_display, stale flush, Stopped invariant, unregister, cancel drop)
- [x] Verify E2E test infrastructure intact (all Go code, Dockerfiles, scripts, slow-mcp-server)
- [x] Review and update portingguide.md (fix stale references, add 6 new checklist items 35-40, update commit history with 20 new Helix commits)
- [x] Push feature branch to origin

## Build & Test (requires CI)

- [ ] `cargo check --package zed --features external_websocket_sync` compiles
- [ ] `cargo test -p external_websocket_sync` passes
- [ ] `cargo test -p acp_thread test_second_send` passes (Stopped invariant)
- [ ] Docker E2E test: all phases pass for `zed-agent`
- [ ] Docker E2E test: all phases pass for `claude` agent

## Finalize

- [x] Push feature branch to origin
- [x] Create PR descriptions (pull_request_zed.md, pull_request_helix.md)
- [ ] After PR merge: update `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt`
- [ ] After PR merge: create PR in helix repo for the `ZED_COMMIT` bump
