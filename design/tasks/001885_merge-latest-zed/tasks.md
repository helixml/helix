# Implementation Tasks

## Setup

- [x] Read `portingguide.md` in full — it is the canonical reference, more detailed than this spec
- [x] Read the previous merge spec (001864) for recent context
- [x] Create feature branch `feature/001885-merge-latest-zed` from fork main
- [x] Add upstream remote: `git remote add upstream https://github.com/zed-industries/zed.git`
- [x] Fetch upstream: `git fetch upstream`
- [x] Check divergence: 177 fork commits ahead, 927 upstream commits to merge

## Merge Execution

- [x] Execute `git merge upstream/main` — 927 upstream commits merged
- [x] Resolve conflicts in `agent_panel.rs` — accepted BaseView/retained_threads refactor, kept Helix imports, onboarding guard, auto-approve
- [x] Resolve conflicts in `conversation_view.rs` — adapted THREAD_REGISTRY block to use root_session_id
- [x] Resolve conflicts in `agent.rs` — auto-merged cleanly, no conflicts
- [x] Resolve conflicts in `connection.rs` — auto-merged cleanly, no conflicts
- [x] Resolve conflicts in `acp_thread.rs` — merged token usage tracking with stopped_emitted guard
- [x] Resolve conflicts in `workspace.rs` — accepted upstream expanded imports
- [x] Resolve conflicts in `Cargo.toml` files — kept feature propagation chain (zed→agent_ui→title_bar)
- [x] Resolve remaining conflicts (title_bar, feature_flags, acp.rs, context_server_registry, etc.)
- [~] Fix compilation errors from upstream API changes (ActiveView→BaseView, field renames, new trait methods)
- [ ] Check if `from_existing_thread()` needs to interact with `retained_threads` (replaces old `draft_threads`)

## Verify Critical Fixes (grep + manual inspection)

- [ ] Fix #1: `grep -n "load_session" crates/agent/src/agent.rs | grep "clone()"` — entity cloned before async task
- [ ] Fix #2: Verify `thread_view.rs` only sends `UserCreatedThread` and `ThreadTitleChanged`, NOT `MessageAdded`/`MessageCompleted`
- [ ] Fix #3: `grep -n "content_only" crates/acp_thread/src/acp_thread.rs` — method exists
- [ ] Fix #4: `grep -n "notify_thread_display" crates/external_websocket_sync/src/thread_service.rs` — called before follow-up
- [ ] Fix #5: Verify stale pending entry flush in `thread_service.rs` when `message_id` changes
- [ ] Fix #6: `cargo test -p acp_thread test_second_send` — Stopped invariant holds
- [ ] Fix #7: `grep -n "unregister_thread" crates/agent_ui/src/conversation_view.rs` — called on entity change
- [ ] Fix #8: `grep -n "drop(turn.send_task)" crates/acp_thread/src/acp_thread.rs` — drop not await
- [ ] Fix #9: `grep -n "stopped_emitted_for_task" crates/acp_thread/src/acp_thread.rs` — guard on normal completion

## Walk Rebase Checklist (portingguide.md items 1–33)

- [ ] Walk through all 33+ rebase checklist items in `portingguide.md` sequentially
- [ ] Pay special attention to items 11 (ConnectedServerState fields), 12 (AgentConnection trait methods), and 13 (AgentServer/AgentConnection API signatures) — these are most likely to have changed
- [ ] Check item 12a: `AcpThreadEvent::Stopped` is still tuple variant `Stopped(StopReason)`
- [ ] Verify `from_existing_thread()` ConnectedServerState fields match current upstream struct

## Build & Test

- [ ] `cargo check --package zed --features external_websocket_sync` — must compile
- [ ] `cargo test -p external_websocket_sync` — unit tests pass
- [ ] `cargo test -p acp_thread test_second_send` — Stopped invariant test passes
- [ ] Build fresh Zed binary: `cargo build --features external_websocket_sync -p zed --release`
- [ ] Run E2E test (zed-agent): copy binary to `e2e-test/zed-binary`, run `./run_docker_e2e.sh`
- [ ] Run E2E test (claude): `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh`
- [ ] All 10 E2E phases pass for both agents

## Update Documentation

- [ ] Update `portingguide.md` — add new upstream API changes discovered during merge:
  - `AgentConnection::prompt` signature change (UserMessageId required)
  - Session ref-counting in NativeAgent (PendingSession, register_session)
  - Worktree picker removal from agent_panel
  - Draft→thread unification (DraftId removed, retained_threads)
  - Thread generation direction (top-down)
  - Any new ConnectedServerState fields
  - Any new AgentConnection trait methods
- [ ] Update portingguide rebase checklist with any new items found during this merge
- [ ] Update portingguide commit history table with new Helix-specific commits (PRs #29–#41)
- [ ] Update portingguide "Modified Upstream Files" section if any new files were modified

## Finalize

- [ ] Push feature branch to fork origin
- [ ] Open PR against fork main
- [ ] Update `sandbox-versions.txt` in `/home/retro/work/helix/` with new ZED_COMMIT hash
- [ ] Create PR in helix repo to bump ZED_COMMIT
- [ ] Rebuild Zed binary and desktop image with new commit
