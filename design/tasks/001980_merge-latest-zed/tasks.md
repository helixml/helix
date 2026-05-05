# Implementation Tasks

## Setup

- [x] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, more detailed than this spec
- [x] Read prior plans for context: `001947_merge-latest-zed/` and `001946_merge-latest-zed/` (planned but never executed; same fork state, useful precedent), and `001909_merge-latest-zed/` (the last merge that actually shipped)
- [x] Verify upstream remote: `cd /home/retro/work/zed && git remote -v`. The `upstream` remote is currently **missing** — add it: `git remote add upstream https://github.com/zed-industries/zed.git`
- [x] `git fetch upstream`
- [x] Record divergence: 172 commits to merge, fork ahead 203, upstream HEAD `1da60a8518` — written into `portingguide.md`
- [x] Create feature branch: `feature/001980-merge-latest-zed` from `f5fab97857`

## Merge Execution

- [x] `git merge upstream/main` — 4 conflicts: `deploy_cloudflare.yml`, `Cargo.lock`, `agent_settings.rs`, `wgpu_renderer.rs`
- [x] Conflict triage done — see `portingguide.md` §"Merge 001980" for per-file resolutions
- [x] `.github/workflows/deploy_cloudflare.yml` — accept upstream deletion (`git rm`)
- [x] `Cargo.lock` — `git checkout --theirs`
- [x] Manual three-way merges:
  - `crates/agent_settings/src/agent_settings.rs` — kept Helix `show_onboarding`/`auto_open_panel`, dropped `new_thread_location` (upstream removed in #55575)
  - `crates/gpui_wgpu/src/wgpu_renderer.rs` — accept upstream comment addition (no Helix concern)
- [x] Porting guide updated live with all 4 resolutions
- [x] No conflict markers remain (`grep -rn "<<<<<<<\|>>>>>>>"` clean)
- [x] Merge committed: `c3e312b056`

## Sweep for Silent Drift (auto-merged files)

- [x] `grep -rn "ActiveView" crates/agent_ui/src/` — clean
- [x] `grep -rn "set_active_view" crates/agent_ui/src/` — clean
- [x] `grep -rn "draft_threads\|background_threads" crates/agent_ui/src/` — clean
- [x] `grep -n "selected_agent_type" crates/agent_ui/src/` — clean
- [x] `grep -n "wait_for_tools_ready" crates/agent/src/agent.rs` — present at line 1722
- [x] `grep -n "smol::Timer" crates/agent/src/agent.rs` — clean
- [x] `grep -n "allow_multiple_instances" crates/zed/src/main.rs` — present at lines 350, 1778
- [x] `grep -n "debug-embed" Cargo.toml` — present at line 704

## Verify Critical Fixes (the 9 in `portingguide.md` §"Critical Fixes")

- [ ] Fix #1: `load_session` clones `NativeAgent` entity before async task (`crates/agent/src/agent.rs`)
- [ ] Fix #2: no `MessageAdded`/`MessageCompleted` sends from `crates/agent_ui/src/acp/thread_view.rs`
- [ ] Fix #3: `content_only` present in `crates/acp_thread/src/acp_thread.rs`
- [ ] Fix #4: `notify_thread_display` called in `crates/external_websocket_sync/src/thread_service.rs` before follow-ups
- [ ] Fix #5: `flush_stale_pending_for_thread` present in `thread_service.rs`
- [ ] Fix #6: `cargo test -p acp_thread test_second_send` passes
- [ ] Fix #7: `unregister_thread` called in `crates/agent_ui/src/conversation_view.rs`
- [ ] Fix #8: `drop(turn.send_task)` present in `acp_thread.rs`
- [ ] Fix #9: `stopped_emitted_for_task` guards both completion paths in `acp_thread.rs`

## Verify Helix Surface (per `requirements.md` acceptance criteria)

- [ ] `crates/external_websocket_sync/` crate intact and unmodified by the merge
- [ ] WebSocket thread display callback present in `agent_panel.rs::new()`
- [ ] UI state query callback present in `agent_panel.rs::new()`
- [ ] `acp_history_store()` accessor still on `AgentPanel`
- [ ] `from_existing_thread()` constructor still on `ConversationView`, with all current `ConnectedServerState` fields populated
- [ ] Channel-based UI event forwarding (`tokio::sync::mpsc`) still in place
- [ ] `OnboardingUpsell::set_dismissed` Helix-mode cleanup path still wired
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` override still applied
- [ ] Built-in agent hiding (Claude Code, Codex, Gemini) still gated on `external_websocket_sync`
- [ ] Enterprise TLS skip still in `sync_settings`
- [ ] Feature propagation chain `zed → agent_ui → title_bar` still intact (`title_bar` dep `optional = true` + matching `[features]` entry)

## Verify PRs #44–#47 (recently added Helix behaviour)

- [ ] PR #44 trailing-edge flush timer for streaming throttle still in `acp_thread.rs`
- [ ] PR #45 `turn_request_id` refresh on `UserMessage NewEntry` still in `external_websocket_sync/`
- [ ] PR #46 `AgentConnectionStore` → `AgentConnectionCache` wiring intact
- [ ] PR #47 context-server request timeout still 180s

## Walk Rebase Checklist

- [ ] Step through every numbered item in `portingguide.md` §"Rebase Checklist"
- [ ] Re-confirm `ConnectedServerState` field count (was 6 fields at 001909) — update `from_existing_thread()` if upstream added/renamed any
- [ ] Re-confirm `AgentConnection` trait: any new methods? If so, every impl Helix touches must add them (or rely on default)
- [ ] Re-confirm `AcpThreadEvent::Stopped(StopReason)` is still a tuple variant
- [ ] Re-confirm Anthropic model list — order matches upstream to minimise future conflict
- [ ] Re-confirm default settings (`show_onboarding`, `trust_all_worktrees`, `show_sign_in`)

## Build & Test (hard gate)

- [ ] `cargo check -p zed` (no features) passes with zero errors
- [ ] `cargo check -p zed --features external_websocket_sync` passes with zero errors
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — produces `./zed-build/zed`
- [ ] `cargo test -p external_websocket_sync` — 37 pass (≤2 ignored env-dependent acceptable)
- [ ] `cargo test -p acp_thread test_second_send` — passes
- [ ] Copy fresh binary: `cp ./zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] E2E `zed-agent`: all in-tree phases pass; explicitly verify Phases 1, 2, 3, 4, 8, 9 named in requirements.md
- [ ] E2E `claude`: all in-tree phases pass (`E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh`)
- [ ] If any phase fails: diagnose, fix, re-run — do **not** mark the task complete

## Update `portingguide.md` (incremental, not at the end)

- [ ] Each conflict resolution appended live with upstream change / resolution / why
- [ ] Append commit history table with this merge's commits (merge commit + any follow-up fixes)
- [ ] Append any new rebase-checklist items uncovered during this merge
- [ ] Note any stale guide entries discovered (e.g. dead-code `HeadlessConnection` references) and either delete or correct them
- [ ] Note any Helix patches absorbed by upstream that can now be retired (with explicit justification)

## Re-merge Fork Main (only if needed)

- [ ] If anyone pushed to fork main during this work, `git merge origin/main` into the feature branch (Cargo.lock conflicts → `--theirs`)
- [ ] Rebuild + re-run E2E

## Finalise

- [ ] Push `feature/001980-merge-latest-zed` to `helixml/zed`
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` in this task directory (PR title + body for each)
- [ ] Open Helix repo PR **first** (bump `ZED_COMMIT` in `sandbox-versions.txt`) — per `CLAUDE.md` ordering rule
- [ ] Open Zed PR against fork main with the merge commit
- [ ] Do **not** force-push `main` without explicit user approval
