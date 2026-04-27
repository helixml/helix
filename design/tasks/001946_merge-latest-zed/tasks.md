# Implementation Tasks

## Setup

- [ ] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, more detailed than this spec
- [ ] Skim previous merge specs in `helix-specs` for resolution patterns:
  - [ ] `001909_merge-latest-zed/design.md` (most recent — same shape as this merge)
  - [ ] `001864_merge-latest-zed/merge_resolution_log.md` (for HIGH-risk file patterns)
- [ ] `cd /home/retro/work/zed && git remote -v` — verify `upstream` URL is `github.com/zed-industries/zed`; fix if missing
- [ ] `git fetch upstream`
- [ ] Note divergence count: how many upstream commits behind, how many fork commits ahead, upstream HEAD SHA
- [ ] Verify clean state: `git status` clean, `sandbox-versions.txt` `ZED_COMMIT` matches fork HEAD `f5fab97857`
- [ ] Create feature branch: `git checkout -b feature/001946-merge-latest-zed`

## Merge Execution

- [ ] `git merge upstream/main`
- [ ] Resolve `.github/workflows/*.yml` conflicts (if any) — accept upstream
- [ ] Resolve `Cargo.lock` conflict (if any) — accept upstream
- [ ] Manually three-way merge `crates/agent_ui/src/agent_panel.rs` (HIGH risk)
- [ ] Manually three-way merge `crates/agent_ui/src/conversation_view.rs` (HIGH risk)
- [ ] Manually three-way merge `crates/external_websocket_sync/src/thread_service.rs` (raised risk — PR #45 just touched it)
- [ ] Manually three-way merge other `crates/external_websocket_sync/src/*` conflicts (raised risk — PR #44 + #46)
- [ ] Manually three-way merge `crates/acp_thread/src/acp_thread.rs` (MEDIUM)
- [ ] Manually three-way merge `crates/acp_thread/src/connection.rs` (MEDIUM)
- [ ] Manually three-way merge `crates/agent/src/agent.rs` (MEDIUM)
- [ ] Resolve `Cargo.toml` (zed, agent_ui, title_bar, workspace root) — preserve `external_websocket_sync` feature chain + `rust-embed` `debug-embed`
- [ ] Resolve `crates/workspace/src/workspace.rs` (LOW)
- [ ] `git add` resolved files and `git commit` the merge

## Sweep for Silent Drift (auto-merged files)

- [ ] `grep -rn "ActiveView\|set_active_view\|draft_threads\|background_threads" crates/agent_ui/src/` — must be clean
- [ ] `grep -rn "smol::Timer" crates/agent/src/` — must be clean (use `cx.background_executor().timer()`)
- [ ] `grep -n "wait_for_tools_ready" crates/agent/src/agent.rs` — Helix addition must still be present
- [ ] `grep -n "allow_multiple_instances" crates/zed/src/main.rs` — both `Args` field and `failed_single_instance_check` use-site present
- [ ] `grep -n "debug-embed" Cargo.toml` — workspace `rust-embed` still has `debug-embed` feature

## Verify Critical Fixes (9 fixes — by grep)

- [ ] Fix #1: `load_session` shape preserved (`crates/agent/src/agent.rs`)
- [ ] Fix #2: No duplicate `MessageAdded`/`MessageCompleted` sends in `thread_view.rs` / `conversation_view.rs`
- [ ] Fix #3: `content_only` present in `crates/acp_thread/src/acp_thread.rs`
- [ ] Fix #4: `notify_thread_display` calls present in `external_websocket_sync/src/thread_service.rs`
- [ ] Fix #5: `flush_stale_pending_for_thread` present in `external_websocket_sync/src/thread_service.rs`
- [ ] Fix #6: every `send()` emits one `Stopped` (verified by `cargo test -p acp_thread test_second_send` later)
- [ ] Fix #7: `unregister_thread` called in `conversation_view.rs`
- [ ] Fix #8: `drop(turn.send_task)` present in `acp_thread.rs`
- [ ] Fix #9: `stopped_emitted_for_task` guards both completion paths in `acp_thread.rs`

## Verify Recently Added Helix PRs Survived

- [ ] PR #44: trailing-edge flush timer present in `crates/external_websocket_sync/src/`
- [ ] PR #45: `turn_request_id` refresh on `UserMessage NewEntry` present
- [ ] PR #46: `AgentConnectionCache` (not `Store`) routing in place
- [ ] PR #47: context-server request timeout still 180s

## Walk Rebase Checklist

- [ ] Walk through each numbered item (1–44) in `portingguide.md` §"Rebase Checklist"
- [ ] Confirm `ConnectedServerState` field list still matches `from_existing_thread()` in `conversation_view.rs`
- [ ] Confirm `AgentConnection` trait — no new methods upstream (or, if added, decide whether they need Helix impl)
- [ ] Confirm `Stopped(StopReason)` tuple variant unchanged

## Build & Test

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` — must produce fresh `./zed-build/zed`
- [ ] If build fails, document root cause and add to porting guide as new rebase checklist item
- [ ] (Optional, if local toolchain) `cargo test -p external_websocket_sync`
- [ ] (Optional, if local toolchain) `cargo test -p acp_thread test_second_send`
- [ ] Copy fresh binary: `cp /home/retro/work/helix/zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] **E2E zed-agent**: `./run_docker_e2e.sh` — all 12 phases must pass
- [ ] **E2E claude**: `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` — all 12 phases must pass
- [ ] Verify Phase 8 (mid-stream interrupt) and Phase 9 (rapid 3-turn cancel) pass for both agents
- [ ] **HARD GATE: do NOT proceed to PR if external_websocket_sync E2E fails**

## Update `portingguide.md` (incrementally during the merge, not retrospectively)

- [ ] Document any new conflict patterns discovered
- [ ] Add new rebase checklist items (next number is #45) for any new gotchas
- [ ] Append new merge commit and any post-merge fix commits to the commit history table
- [ ] If genuinely uneventful, only the commit-history append is needed — do not invent updates

## Re-merge Fork Main (if user pushes during the merge)

- [ ] If `origin/main` advances during the work, `git merge origin/main` into the feature branch
- [ ] Resolve any conflicts (Cargo.lock typically — take theirs)
- [ ] Rebuild and re-run E2E

## Finalize

- [ ] **Open Helix PR FIRST** (per `CLAUDE.md` ordering): bump `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt` to the merge branch HEAD, open PR
- [ ] Push `feature/001946-merge-latest-zed` to Zed fork origin
- [ ] Open Zed PR (will be #48) with summary of upstream changes + conflict resolutions
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` in this task directory documenting the actual outcome
- [ ] After both PRs merge, build pipeline rebuilds Zed binary + desktop image automatically
