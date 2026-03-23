# Implementation Tasks

## Preparation

- [ ] Add upstream Zed remote: `git remote add upstream https://github.com/zed-industries/zed && git fetch upstream`
- [ ] Record current upstream HEAD SHA and compare with fork's base commit to understand the delta

## Merge

- [ ] Rebase (or merge) fork onto `upstream/main`: `git rebase upstream/main`
- [ ] Resolve conflicts file by file, keeping all `#[cfg(feature = "external_websocket_sync")]` blocks

## ACP Consolidation Rename Tracking

- [ ] Identify whether upstream renamed `acp_thread` crate or `crates/agent_ui/src/acp/` path — find the new locations before applying critical fixes
- [ ] Update all portingguide.md file path references to match new upstream names

## Critical Fix Verification (post-merge, before tests)

- [ ] Verify Critical Fix #1 is present: `load_session()` clones `Entity<NativeAgent>` before spawning async task (`crates/agent/src/agent.rs`)
- [ ] Verify Critical Fix #2 is present: `thread_view.rs` does NOT send `MessageAdded`/`MessageCompleted` WebSocket events (only `UserCreatedThread` and `ThreadTitleChanged`)
- [ ] Verify Critical Fix #3 is present: `AssistantMessage::content_only()` exists in `crates/acp_thread/src/acp_thread.rs`
- [ ] Verify Critical Fix #4 is present: follow-up path in `thread_service.rs` calls `notify_thread_display()` before sending message

## Structural Compatibility Check

- [ ] Check `ConnectedServerState` fields in `thread_view.rs` — if upstream changed `active_id`, `threads`, or `conversation` fields, update `from_existing_thread()` to match
- [ ] Check `AgentConnection` trait signature — if upstream changed the trait, update `HeadlessConnection` impl
- [ ] Check `agent_panel.rs` cfg-gated blocks are intact: callback setup, `from_existing_thread`, onboarding dismissal, `acp_history_store()`
- [ ] Verify all items 1–18 of the portingguide.md Rebase Checklist

## Test Validation

- [ ] `cargo check --package zed --features external_websocket_sync` — must compile with no errors
- [ ] `cargo test -p external_websocket_sync` — all unit and protocol-level tests pass
- [ ] Run E2E test: `ANTHROPIC_API_KEY=<key> crates/external_websocket_sync/e2e-test/run_docker_e2e.sh` — all 10 phases must pass

## Porting Guide Update

- [ ] Fix stale E2E phase count: update "Four-phase test" description to reflect the current 10-phase suite
- [ ] Add any newly conflicted upstream files to the "Modified Upstream Files" table
- [ ] Document any structural upstream changes and how they were resolved
- [ ] Add any new critical fixes or gotchas discovered during this merge
- [ ] Update the Commit History table with new Helix-specific commits

## Finalize

- [ ] Push updated fork to remote: `git push origin main`
