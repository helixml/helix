# Implementation Tasks

## Setup

- [x] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, more detailed than this spec (724 lines as of 2026-05-11)
- [x] Read prior plan `001980_merge-latest-zed/` end-to-end — closest precedent (this is mandatory, not optional)
- [x] Skim 001909 plan for the carry-over fix list (`--allow-multiple-instances`, `debug-embed`, `smol → executor.timer`)
- [x] Verify upstream remote: `cd /home/retro/work/zed && git remote -v`. If missing, add: `git remote add upstream https://github.com/zed-industries/zed.git`
- [x] `git fetch upstream`
- [x] Verify divergence: 127 commits to merge, fork HEAD `fe8f4f4e3f`, upstream HEAD `8bdd78e023` (confirmed at runtime — numbers unchanged)
- [x] Pull `origin/main` first in case fork main moved since this spec was written (no movement)
- [x] Create feature branch: `feature/001996-merge-latest-zed` from current fork main

## Pre-Merge Reconnaissance

- [x] Read upstream commit `0a52f80824` in full — Helix's existing dropped-tx branch already does the cleanup upstream is fixing (lines 2308–2329) AND adds `Stopped(Cancelled)` emission with duplicate-guard. Helix's logic is a strict superset; resolution principle: keep Helix code, the conflict will likely auto-resolve or need a tiny three-way pick
- [x] Inspect `agent_panel.rs` diff start — upstream removes `external_websocket_sync_dep as external_websocket_sync` alias re-export (it's now imported directly per crate). Big diff, expect heavy conflicts in cfg-gated regions
- [x] Skipped detailed pre-read of `acp_thread.rs` / `conversation_view.rs` — better to let `git merge` surface the actual conflicts than try to predict from diffs

## Merge Execution

- [~] `git merge upstream/main`
- [ ] Triage conflicts; for each, write a `## Merge 001996` subsection in `portingguide.md` capturing upstream change, resolution, why, risk
- [ ] `Cargo.lock` (if conflicting): `git checkout --theirs Cargo.lock`
- [ ] `.github/workflows/*` (if conflicting): accept upstream
- [ ] Manual three-way merges for any cfg-gated Helix code in `agent_panel.rs`, `acp_thread.rs`, `conversation_view.rs`
- [ ] **If conflict in `acp_thread.rs` cancel/Stopped path**: stop and reason carefully about interaction with Critical Fixes #6/#8/#9 and PR #52 `cancel_current_turn` before resolving
- [ ] No conflict markers remain (`grep -rn "<<<<<<<\|>>>>>>>" .`)
- [ ] Commit the merge

## Sweep for Silent Drift (auto-merged files)

- [ ] `grep -rn "ActiveView" crates/agent_ui/src/` — should be clean
- [ ] `grep -rn "set_active_view" crates/agent_ui/src/` — should be clean
- [ ] `grep -rn "draft_threads\|background_threads" crates/agent_ui/src/` — should be clean
- [ ] `grep -n "selected_agent_type" crates/agent_ui/src/` — should be clean
- [ ] `grep -n "AcpThreadEvent::Stopped\b\([^(]\|$\)" crates/acp_thread/src/` — should be clean (test-pattern drift, lesson from 001980)
- [ ] `grep -n "wait_for_tools_ready" crates/agent/src/agent.rs` — should be present
- [ ] `grep -n "smol::Timer" crates/agent/src/agent.rs` — should be clean
- [ ] `grep -n "allow_multiple_instances" crates/zed/src/main.rs` — should be present (≥2 sites: arg def + single-instance short-circuit)
- [ ] `grep -n "headless" crates/zed/src/main.rs` — should be present (3 sites per checklist 39a)
- [ ] `grep -n "debug-embed" Cargo.toml` — should be present
- [ ] `grep -n "external_websocket_sync::get_thread" crates/agent_ui/src/agent_panel.rs` — should be present (Critical Fix #11 from PR #53)

## Verify Critical Fixes (the 11 in `portingguide.md` §"Critical Fixes")

- [ ] Fix #1: `load_session` clones `NativeAgent` entity before async task (`crates/agent/src/agent.rs`)
- [ ] Fix #2: no `MessageAdded`/`MessageCompleted` sends from `crates/agent_ui/src/acp/thread_view.rs`
- [ ] Fix #3: `content_only` present in `crates/acp_thread/src/acp_thread.rs`
- [ ] Fix #4: `notify_thread_display` callable in `external_websocket_sync/src/thread_service.rs`
- [ ] Fix #5: `flush_stale_pending_for_thread` present in `thread_service.rs`
- [ ] Fix #6: `Stopped(_)` test patterns intact (`cargo test -p acp_thread test_second_send` if local rust)
- [ ] Fix #7: `unregister_thread` called in `conversation_view.rs`
- [ ] Fix #8: `drop(turn.send_task)` present in `acp_thread.rs`
- [ ] Fix #9: `stopped_emitted_for_task` guards both completion paths
- [ ] Fix #10: context-server request timeout still 180s (`crates/context_server/src/client.rs`)
- [ ] Fix #11: `load_agent_thread` entity-identity guard at top, calls `external_websocket_sync::get_thread(session_id)` (PR #53)

## Verify Helix Surface (per `requirements.md` acceptance criteria)

- [ ] `crates/external_websocket_sync/` crate intact (no merge edits)
- [ ] WebSocket thread display callback present in `agent_panel.rs::new()`
- [ ] UI state query callback present in `agent_panel.rs::new()`
- [ ] `acp_history_store()` accessor still on `AgentPanel`
- [ ] `from_existing_thread()` constructor still on `ConversationView`; `ConnectedServerState` field count unchanged (6 fields)
- [ ] Channel-based UI event forwarding (`tokio::sync::mpsc`) still in place
- [ ] `OnboardingUpsell::set_dismissed` Helix-mode cleanup path still wired
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` override still applied (`feature_flags/src/flags.rs`)
- [ ] Built-in agent hiding still gated on `external_websocket_sync`
- [ ] Enterprise TLS skip still in `sync_settings`
- [ ] Feature propagation chain intact (`zed → agent_ui → title_bar`, all `optional = true`)

## Verify PRs #51–#53 (Helix behaviour added since 001980)

- [ ] PR #51 `--headless` flag intact across all 3 call sites in `crates/zed/src/main.rs` (per checklist 39a)
- [ ] PR #51 `initialize_headless()` function present and cfg-gated
- [ ] PR #51 e2e `E2E_HEADLESS=1` mode still wired
- [ ] PR #52 `cancel_current_turn` command type in `external_websocket_sync/src/types.rs`
- [ ] PR #52 `turn_cancelled` event type in `external_websocket_sync/src/types.rs`
- [ ] PR #52 `cancel_current_turn` routing in `external_websocket_sync/src/external_websocket_sync.rs`
- [ ] PR #52 `cancel_current_turn` handler in `thread_service.rs` (lookup active thread by request_id, call `cancel()`)
- [ ] PR #52 `cancel_current_turn` send path in `websocket_sync.rs`
- [ ] PR #53 entity-identity guard at top of `load_agent_thread` (Critical Fix #11)

## Walk Rebase Checklist

- [ ] Walk all 44 items in `portingguide.md` §"Rebase Checklist" (includes new 41a from 001980 and #11 from PR #53)
- [ ] `ConnectedServerState` field count: 6 (re-confirm)
- [ ] `AgentConnection` trait: any new methods needing Helix work?
- [ ] `AcpThreadEvent::Stopped(StopReason)` still tuple variant
- [ ] Anthropic model list order matches upstream (no conflict in this merge?)
- [ ] Default settings (`show_onboarding`, `trust_all_worktrees`, `show_sign_in`) intact

## Build & Test (hard gate)

- [ ] `./stack build-zed dev` succeeds with zero errors → `./zed-build/zed` produced
- [ ] (If local rust toolchain) `cargo check -p zed --features external_websocket_sync` clean
- [ ] (If local rust toolchain) `cargo test -p external_websocket_sync` full pass
- [ ] (If local rust toolchain) `cargo test -p acp_thread test_second_send` passes
- [ ] (If local rust toolchain) `cargo test -p external_websocket_sync cancel_current_turn` passes (PR #52)
- [ ] Pre-flight: `(cd crates/external_websocket_sync/e2e-test/helix-ws-test-server && go mod tidy)` — runner doesn't tidy itself
- [ ] Copy fresh binary into `e2e-test/zed-binary`
- [ ] Run E2E `zed-agent` — **all phases pass**, including 1, 2, 3, 4, 8, 9, **13**, **14**
- [ ] Run E2E `claude` — **all phases pass**, including 1, 2, 3, 4, 8, 9, **13**, **14**
- [ ] No phase failed — task complete on the test gate

## Update `portingguide.md` (incremental, not at the end)

- [ ] All conflict resolutions appended live with upstream change / resolution / why / risk
- [ ] `## Merge 001996 (2026-05-11)` section created mirroring 001980's structure
- [ ] If upstream `0a52f80824` (#55562) required resolution: dedicated subsection explaining interaction with Critical Fixes #6/#8/#9 and PR #52 `cancel_current_turn`
- [ ] Commit history table extended with merge commit + any follow-up fix commits
- [ ] Any new rebase-checklist items added if novel fragility uncovered
- [ ] Stale guide entries discovered along the way are corrected or deleted (do not invent updates)

## Re-merge Fork Main (only if needed)

- [ ] If anyone pushed to fork main during the merge: `git merge origin/main` into feature branch and re-run E2E

## Finalise

- [ ] Push `feature/001996-merge-latest-zed` to Zed remote (`origin`)
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` in this task directory (mirror 001980's structure)
- [ ] Bump `sandbox-versions.txt` `ZED_COMMIT=` in `/home/retro/work/helix/` to new merge HEAD
- [ ] Push `feature/001996-merge-latest-zed` branch to Helix remote
- [ ] Do not force-push `main`
- [ ] Do not open PRs from the agent (per task convention — Helix UI handles PR creation)
