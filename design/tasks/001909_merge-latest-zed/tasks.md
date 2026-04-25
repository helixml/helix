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

- [x] `grep -rn "ActiveView" crates/agent_ui/src/` — clean (no matches)
- [x] `grep -rn "set_active_view" crates/agent_ui/src/` — clean (no matches)
- [x] `grep -rn "draft_threads\|background_threads" crates/agent_ui/src/` — clean (no matches)
- [x] No stale references found; `wait_for_tools_ready` (Helix addition) preserved at agent.rs:1723

## Verify Critical Fixes

- [x] Fix #1: `load_session` shape preserved (relies on PendingSession ref-counting from upstream — same as fork main pre-merge, working since 001864)
- [x] Fix #2: `MessageAdded`/`MessageCompleted` not in `conversation_view/thread_view.rs` (only a comment about MessageCompleted from subscription)
- [x] Fix #3: `content_only` present at `acp_thread.rs:144`
- [x] Fix #4: `notify_thread_display` called in 4 places in `thread_service.rs` (incl. before follow-ups)
- [x] Fix #5: `flush_stale_pending_for_thread` present at `thread_service.rs:202`
- [ ] Fix #6: deferred — runs as part of `cargo test -p acp_thread test_second_send` in build & test phase
- [x] Fix #7: `unregister_thread` called in `conversation_view.rs` (line ~801)
- [x] Fix #8: `drop(turn.send_task)` present at `acp_thread.rs:2474`
- [x] Fix #9: `stopped_emitted_for_task` guards both completion paths (`acp_thread.rs:2319`, `2423`)

## Walk Rebase Checklist

- [x] Walk through key checklist items — high-risk files all auto-merged cleanly, all 9 critical fixes present, no silent renames
- [x] `ConnectedServerState` has 6 fields (`connection`, `auth_state`, `active_id`, `threads`, `conversation`, `_connection_entry_subscription`) — `from_existing_thread()` at line 999 sets all of them. Note: portingguide mentions `history` field but it's not in the struct — porting guide is stale on this point
- [x] `AgentConnection` trait — no new methods added in this delta vs Helix's existing impls
- [x] `Stopped(StopReason)` tuple variant unchanged
- [x] **Discovery**: `HeadlessConnection` referenced in portingguide only exists in dead code (`crates/agent_ui/src/acp/thread_view.rs`, not in `mod` tree). Current `from_existing_thread()` reuses `thread.read(cx).connection().clone()` — no longer needs HeadlessConnection. Will update portingguide.

## Build & Test

- [x] `./stack build-zed dev` — succeeded (2m 12s, 171M binary). Required follow-up commit `6ccf3010a6` to fix `wait_for_tools_ready` (smol → `cx.background_executor().timer()`) since upstream PR #53603 removed smol from agent crate deps.
- [x] Compile-error cause documented in design.md (smol removal, not the predicted HeadlessConnection issue)
- [ ] `cargo test -p external_websocket_sync` — deferred (can run via stack/docker if needed; not in canonical local toolchain)
- [ ] `cargo test -p acp_thread test_second_send` — deferred (will run as part of E2E)
- [x] Copy fresh binary: `cp /home/retro/work/helix/zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [x] E2E zed-agent: **PASSED all 12 phases** (test now has 12 phases, not 10 as portingguide says — added Phase 11 Spectask routing and Phase 12 Reconnect)
- [x] E2E claude: **PASSED all 12 phases**
- [x] Phase 8 (mid-stream interrupt) + Phase 9 (rapid 3-turn cancel) both PASSED for both agents

## Update `portingguide.md`

- [x] Update porting guide — added 3 new rebase checklist items (allow_multiple_instances flag, rust-embed debug-embed, smol→executor.timer pattern), appended commit history with all 6 new commits, noted 12-phase E2E (not 10)

## Re-merge Fork Main (out-of-band fix)

- [x] User pushed `d7be64fad1` ("fix: stop empty message_completed loop after Zed restart + Helix-mode UI cleanup") to fork main while implementation was in progress; merged it in (1 trivial Cargo.lock conflict, took theirs)
- [~] Rebuild after merging `d7be64fad1`

## Finalize

- [ ] `git push origin feature/001909-merge-latest-zed`
- [ ] Open PR against `helixml/zed` `main` with title "Merge upstream Zed into fork (001909)" and a body summarizing: upstream HEAD merged, conflict count, any new portingguide entries, E2E test results
- [ ] After fork PR merges, update `/home/retro/work/helix/sandbox-versions.txt` — set `ZED_COMMIT=` to the new merge commit SHA
- [ ] Open Helix repo PR to bump `ZED_COMMIT`
- [ ] After Helix PR merges, the build pipeline rebuilds Zed binary + desktop image
