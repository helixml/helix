# Implementation Tasks

## Setup

- [ ] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, more detailed than this spec
- [ ] Skim previous merge logs: `001909_merge-latest-zed/tasks.md` and `001864_merge-latest-zed/merge_resolution_log.md` for resolution patterns
- [ ] Verify upstream remote: `cd /home/retro/work/zed && git remote -v` — if `upstream` is missing, `git remote add upstream https://github.com/zed-industries/zed.git`
- [ ] `git fetch upstream`
- [ ] Record divergence: `git log --oneline upstream/main ^main | wc -l` (commits to merge) and `git log --oneline main ^upstream/main | wc -l` (fork commits ahead) — write the count and current upstream HEAD into the porting guide
- [ ] Create feature branch: `git checkout -b feature/001947-merge-latest-zed` from fork main (`f5fab97857` or newer if anyone pushed since)

## Merge Execution

- [ ] `git merge upstream/main`
- [ ] List conflicted files; for each, classify into Category 1 (accept upstream) or Category 2 (manual three-way merge) using the resolution principles in `design.md`
- [ ] `.github/workflows/*` → `git checkout --theirs` (always)
- [ ] `Cargo.lock` → `git checkout --theirs` (always)
- [ ] For each Category 2 file: resolve manually, **then immediately append a porting-guide entry** with upstream change / resolution / why
- [ ] After resolving: re-grep for renamed identifiers (`ActiveView`, `set_active_view`, `selected_agent_type`, `draft_threads`, `background_threads`, etc.) in cfg-gated regions — fix any silent drift now
- [ ] `git add` resolved files and `git commit` the merge

## Sweep for Silent Drift (auto-merged files)

- [ ] `grep -rn "ActiveView" crates/agent_ui/src/` — must be clean (renamed to `BaseView` in 001864)
- [ ] `grep -rn "set_active_view" crates/agent_ui/src/` — must be clean
- [ ] `grep -rn "draft_threads\|background_threads" crates/agent_ui/src/` — must be clean (now `retained_threads`)
- [ ] `grep -n "selected_agent_type" crates/agent_ui/src/` — must be clean (now `selected_agent`)
- [ ] `grep -n "wait_for_tools_ready" crates/agent/src/agent.rs` — Helix addition still present
- [ ] `grep -n "allow_multiple_instances" crates/zed/src/main.rs` — Helix CLI flag still present
- [ ] `grep -n "debug-embed" Cargo.toml` — `rust-embed` workspace feature still set

## Verify Critical Fixes

- [ ] Fix #1: `load_session` clones `NativeAgent` entity before async task (`crates/agent/src/agent.rs`)
- [ ] Fix #2: no `MessageAdded`/`MessageCompleted` sends from `crates/agent_ui/src/acp/thread_view.rs`
- [ ] Fix #3: `content_only` present in `crates/acp_thread/src/acp_thread.rs`
- [ ] Fix #4: `notify_thread_display` called in `crates/external_websocket_sync/src/thread_service.rs` before follow-ups
- [ ] Fix #5: `flush_stale_pending_for_thread` present in `thread_service.rs`
- [ ] Fix #6: `cargo test -p acp_thread test_second_send` passes
- [ ] Fix #7: `unregister_thread` called in `crates/agent_ui/src/conversation_view.rs`
- [ ] Fix #8: `drop(turn.send_task)` present in `acp_thread.rs`
- [ ] Fix #9: `stopped_emitted_for_task` guards both completion paths in `acp_thread.rs`

## Verify Helix Surface (per requirements.md acceptance criteria)

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

## Walk Rebase Checklist

- [ ] Step through every numbered item in `portingguide.md` §"Rebase Checklist"
- [ ] Re-confirm `ConnectedServerState` field count (was 6 fields at 001909) — update `from_existing_thread()` if upstream added/renamed any
- [ ] Re-confirm `AgentConnection` trait: any new methods? If so, every impl Helix touches must add them (or rely on default)
- [ ] Re-confirm `AcpThreadEvent::Stopped(StopReason)` is still a tuple variant
- [ ] Re-confirm Anthropic model list — order matches upstream to minimise future conflict
- [ ] Re-confirm default settings (`show_onboarding`, `trust_all_worktrees`, `show_sign_in`)

## Build & Test (hard gate)

- [ ] `cargo check -p zed` (no features) passes with zero errors
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — produces `./zed-build/zed`
- [ ] `cargo test -p external_websocket_sync` — 37 pass (≤2 ignored env-dependent acceptable)
- [ ] `cargo test -p acp_thread test_second_send` — passes
- [ ] Copy fresh binary: `cp ./zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] E2E zed-agent: all in-tree phases pass; explicitly verify the four named in requirements.md (Phase 1: thread A id + entries ≥ 2; Phase 2: same id, entries grow; Phase 3: thread B id, entries ≥ 2; Phase 4: non-visible-thread follow-up completes with no thread-load error)
- [ ] E2E claude (Claude Code agent): all in-tree phases pass
- [ ] If any phase fails: diagnose, fix, re-run — do **not** mark the task complete

## Update `portingguide.md` (incremental, not at the end)

- [ ] Each conflict resolution appended live with upstream change / resolution / why
- [ ] Append commit history table with this merge's commits (merge commit + any follow-up fixes)
- [ ] Append any new rebase-checklist items uncovered during this merge
- [ ] Note any stale guide entries discovered (e.g. dead-code `HeadlessConnection` references) and either delete or correct them

## Re-merge Fork Main (only if needed)

- [ ] If anyone pushed to fork main during this work, `git merge origin/main` into the feature branch (Cargo.lock conflicts → `--theirs`)
- [ ] Rebuild + re-run E2E

## Finalise

- [ ] Push `feature/001947-merge-latest-zed` to `helixml/zed`
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` in this task directory
- [ ] Open Helix repo PR first (bump `ZED_COMMIT` in `sandbox-versions.txt`) — per `CLAUDE.md` ordering rule
- [ ] Open Zed PR against fork main with merge commit
- [ ] Do **not** force-push `main` without explicit user approval
