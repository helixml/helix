# Implementation Tasks

## Setup

- [x] Read `/home/retro/work/zed/portingguide.md` in full — it is the canonical reference, more detailed than this spec
- [x] Skim previous merge spec `/home/retro/work/helix-specs/design/tasks/001864_merge-latest-zed/merge_resolution_log.md` for resolution patterns
- [x] Verify upstream remote URL: `cd /home/retro/work/zed && git remote -v` — if `upstream` URL is missing, `git remote set-url upstream https://github.com/zed-industries/zed.git`
- [x] `git fetch upstream`
- [x] Note divergence count: **86 upstream commits to merge**, 187 fork commits ahead of common ancestor. Upstream HEAD: `e3d1876c06` ("Revert terminal changes from #54728 (#54836)")
- [x] Create feature branch: `git checkout -b feature/001909-merge-latest-zed`

## Merge Execution

- [x] `git merge upstream/main` — **only 1 conflict**: `crates/agent/src/agent.rs` (visibility + smol→async_channel migration on `NativeAgentSessionList::new`); all other files auto-merged including `agent_panel.rs`, `conversation_view.rs`, `acp_thread.rs`, `connection.rs`, `workspace.rs`
- [x] N/A — no conflicts in `.github/workflows/*`
- [x] N/A — no conflicts in `Cargo.lock`
- [x] N/A — `crates/agent_ui/src/agent_panel.rs` auto-merged
- [x] N/A — `crates/agent_ui/src/conversation_view.rs` auto-merged
- [x] N/A — `crates/acp_thread/src/acp_thread.rs` auto-merged
- [x] N/A — `crates/acp_thread/src/connection.rs` auto-merged
- [x] Resolved `crates/agent/src/agent.rs`: took upstream's `fn new` (visibility) + `async_channel::unbounded()` (matches the auto-merged field types). Helix's `pub` on `new()` was unused externally
- [x] N/A — no conflicts in `Cargo.toml` files
- [x] N/A — no conflicts in `crates/workspace/src/workspace.rs`
- [x] No remaining conflicts after agent.rs resolution
- [x] `git add` resolved files and `git commit` the merge — commit `8428a4399d`

## Sweep for Silent Drift (auto-merged files)

- [ ] `grep -rn "ActiveView" crates/agent_ui/src/` — should return nothing if upstream has finished the `BaseView` rename (was already done in 001864)
- [ ] `grep -rn "set_active_view" crates/agent_ui/src/`
- [ ] `grep -rn "draft_threads\|background_threads" crates/agent_ui/src/` — both replaced with `retained_threads`
- [ ] Fix any stale references found in Helix cfg-gated code

## Verify Critical Fixes

- [ ] Fix #1: `grep -n "load_session" crates/agent/src/agent.rs | head` — entity clone present before async task
- [ ] Fix #2: `grep -n "MessageAdded\|MessageCompleted" crates/agent_ui/src/conversation_view/thread_view.rs` — should NOT find these (only `UserCreatedThread` + `ThreadTitleChanged` allowed)
- [ ] Fix #3: `grep -n "content_only" crates/acp_thread/src/acp_thread.rs` — method exists
- [ ] Fix #4: `grep -n "notify_thread_display" crates/external_websocket_sync/src/thread_service.rs` — called before follow-up
- [ ] Fix #5: Verify stale-pending flush in `thread_service.rs` when `message_id` changes
- [ ] Fix #6: `cargo test -p acp_thread test_second_send` — passes
- [ ] Fix #7: `grep -n "unregister_thread" crates/agent_ui/src/conversation_view.rs` — called on entity change
- [ ] Fix #8: `grep -n "drop(turn.send_task)" crates/acp_thread/src/acp_thread.rs` — present (NOT `cx.background_spawn(turn.send_task)`)
- [ ] Fix #9: `grep -n "stopped_emitted_for_task" crates/acp_thread/src/acp_thread.rs` — guard on normal completion path

## Walk Rebase Checklist

- [ ] Walk through every numbered item (1–41+) in `portingguide.md` §"Rebase Checklist" — tick each off mentally
- [ ] Pay extra attention to: items 11 (`ConnectedServerState` fields), 12 (`AgentConnection` trait methods), 12a (`Stopped(StopReason)` tuple), 13 (agent_servers signatures)
- [ ] Verify `from_existing_thread()` field list matches current `ConnectedServerState` exactly

## Build & Test

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` — builds zed binary in Docker (Helix's canonical build path)
- [ ] If compile fails: read errors carefully, most likely cause is a missing `AgentConnection` trait method on `HeadlessConnection` or a missing `ConnectedServerState` field in `from_existing_thread()`
- [ ] `cargo test -p external_websocket_sync` — unit tests pass
- [ ] `cargo test -p acp_thread test_second_send` — Stopped invariant test passes
- [ ] Copy fresh binary: `cp /home/retro/work/helix/zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] E2E zed-agent: `cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test && ./run_docker_e2e.sh`
- [ ] E2E claude: `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh`
- [ ] All 10 phases pass for both agents (Phase 8 + 9 most sensitive — verify they succeed)

## Update `portingguide.md`

- [ ] If any new upstream API changes were encountered, document them under "Modified Upstream Files"
- [ ] If any new conflict patterns were discovered, append a numbered item to "Rebase Checklist"
- [ ] Append the merge commit + any post-merge fix commits to "Commit History" table
- [ ] If the merge was uneventful (no API changes), only the commit-history append is needed — do NOT invent updates

## Finalize

- [ ] `git push origin feature/001909-merge-latest-zed`
- [ ] Open PR against `helixml/zed` `main` with title "Merge upstream Zed into fork (001909)" and a body summarizing: upstream HEAD merged, conflict count, any new portingguide entries, E2E test results
- [ ] After fork PR merges, update `/home/retro/work/helix/sandbox-versions.txt` — set `ZED_COMMIT=` to the new merge commit SHA
- [ ] Open Helix repo PR to bump `ZED_COMMIT`
- [ ] After Helix PR merges, the build pipeline rebuilds Zed binary + desktop image
