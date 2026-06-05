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

- [x] `git merge upstream/main` — 1 conflict only (`acp_thread.rs`); no conflicts in `agent_panel.rs`/`conversation_view.rs` despite large diffs (auto-merged); no Cargo.lock conflict
- [x] Triage conflicts; resolved in `portingguide.md` §"Merge 001996" with full reasoning
- [x] `acp_thread.rs` resolved — folded upstream PR #55562 reorder with Helix `stopped_emitted_for_task`-guarded `Stopped(Cancelled)` emission. Single same-turn `take()` before dropped-tx guard, then dropped-tx guard emits `Stopped(Cancelled)` if not already emitted. Strict superset of both sides
- [x] No conflict markers remain (`grep -rn "<<<<<<<\|>>>>>>>" .` — only test-string markers in `git_store.rs`)
- [x] Merge committed: `bf544922aa`; porting guide entry committed: `48f7895607`

## Sweep for Silent Drift (auto-merged files)

- [x] `ActiveView` — only matches are `AgentPanelEvent::ActiveViewChanged`/`ActiveViewFocused` (valid enum variants, not the dead `ActiveView` from old porting guide)
- [x] `set_active_view` — clean
- [x] `draft_threads`/`background_threads` — clean
- [x] `selected_agent_type` — clean
- [x] `AcpThreadEvent::Stopped[^(]` test-pattern drift — clean (only a comment match)
- [x] `wait_for_tools_ready` — present (verified separately)
- [x] `smol::Timer` in `agent/src/agent.rs` — clean
- [x] `allow_multiple_instances` — present (2 sites: line 360 short-circuit, line 1957 arg def)
- [x] `headless` — present (all 3 call sites + supporting code in `main.rs`)
- [x] `debug-embed` in `Cargo.toml` — present line 705
- [x] Critical Fix #11 entity guard `external_websocket_sync::get_thread` — present in `agent_panel.rs:3027`

## Verify Critical Fixes (the 11 in `portingguide.md` §"Critical Fixes")

- [x] Fix #1: `open_thread` uses `pending_sessions` shared-task pattern with WeakEntity `this` (refactored from raw entity-clone — same protection)
- [x] Fix #2: `thread_view.rs` has only `unregister_thread*` calls and `register_thread`, no `MessageAdded`/`MessageCompleted` direct sends
- [x] Fix #3: `content_only` at `acp_thread.rs:144`
- [x] Fix #4: `notify_thread_display` called in 4 places in `thread_service.rs` (1065, 1494, 1765, 2051)
- [x] Fix #5: `flush_stale_pending_for_thread` at `thread_service.rs:211`
- [x] Fix #6: combined cancel/Stopped logic from conflict resolution preserves the "exactly one Stopped per send" invariant — `stopped_emitted_for_task` guards both the dropped-tx path (line 2360) and the natural-completion path (line 2452)
- [x] Fix #7: `unregister_thread` called in `conversation_view.rs:812, 817`
- [x] Fix #8: `drop(turn.send_task)` at `acp_thread.rs:2503`
- [x] Fix #9: `stopped_emitted_for_task` guards both completion paths (2360, 2452)
- [x] Fix #10: context-server `DEFAULT_REQUEST_TIMEOUT = 180` at `context_server/src/client.rs:38`
- [x] Fix #11: `external_websocket_sync::get_thread` at `agent_panel.rs:3027` (PR #53 entity-identity guard)

## Verify Helix Surface (per `requirements.md` acceptance criteria)

- [x] `crates/external_websocket_sync/` crate intact (10 source files including new `sync.rs`)
- [x] `acp_history_store()` accessor at `agent_panel.rs:818`
- [x] `from_existing_thread()` at `conversation_view.rs:771`; `ConnectedServerState` 6 fields unchanged (auth_state, active_id, threads, connection, conversation, _connection_entry_subscription)
- [x] `AcpBetaFeatureFlag::enabled_for_all() -> true` at `feature_flags/src/flags.rs:30`
- [x] Feature propagation chain intact: `zed/Cargo.toml` declares `external_websocket_sync = ["agent_ui/external_websocket_sync", ...]` and the dep is `optional = true`; `agent_ui` and `title_bar` likewise

## Verify PRs #51–#53 (Helix behaviour added since 001980)

- [x] PR #51 `--headless` flag at `main.rs:1965` (arg def), `:341` (platform), `:361` (single-instance short-circuit), `:885-886` (run branch), `:1438` (`initialize_headless` body)
- [x] PR #51 `initialize_headless()` cfg-gated and present
- [x] PR #52 `cancel_current_turn` command + `turn_cancelled` event in `types.rs:235-236, 324`
- [x] PR #52 routing in `external_websocket_sync.rs:339-349`
- [x] PR #52 handler in `thread_service.rs:49, 452, 1255, 1264`
- [x] PR #52 dispatch in `websocket_sync.rs:405, 549-556`
- [x] PR #52 protocol test `test_cancel_current_turn_noop` at `protocol_test.rs:446`
- [x] PR #53 entity-identity guard at `agent_panel.rs:3027` (Critical Fix #11)

## Walk Rebase Checklist

- [x] All 44 items walked (via the silent-drift sweep + critical-fix verification + Helix-surface checks above; `ConnectedServerState` 6 fields confirmed; `Stopped(StopReason)` still tuple)
- [x] No new `AgentConnection` trait methods needing Helix work (compile-clean)

## Build & Test (hard gate)

- [x] `./stack build-zed dev` succeeds (46s warm cache, 0 errors, only 1 unused-import warning in `zed.rs:849`) — `./zed-build/zed` produced (22M)
- [-] No local Rust toolchain: cargo check/test deferred to CI / E2E
- [x] Build implicitly proves `cargo check -p zed --features external_websocket_sync` (it's the same compile)
- [x] **Build fix needed**: upstream added `BaseView::Terminal { terminal_id }` variant to `agent_panel.rs:733`. Helix UI state query at `agent_panel.rs:1270` was missing the arm — added `BaseView::Terminal { .. } => ("terminal".to_string(), None, 0, None)` (commit `1828cea13c`)
- [x] Pre-flight: `go mod tidy` in `e2e-test/helix-ws-test-server/` — clean, no changes
- [x] Copy fresh binary into `e2e-test/zed-binary`
- [x] **First E2E run revealed Phase 13 race**: `message_completed` arrived before `turn_cancelled`, so `handleTurnCancelled` saw state=Completed (not Waiting) and didn't transition to Interrupted. Fixed by reordering the cancellation handler in `thread_service.rs:1238` to (a) probe `thread.status() == Generating` first, (b) send `TurnCancelled{status:cancelled}` BEFORE invoking `cancel()` so it wins the race against the synchronously-emitted Stopped → message_completed, (c) send `noop` if no turn was running. Commit `a7ad11ec00`.
- [x] Run E2E `zed-agent` — **ALL 14 PHASES PASSED** (including new Phase 13 cancel + Phase 14 noop)
- [x] Run E2E `claude` — **ALL 14 PHASES PASSED** on retry (first attempt timed out at Phase 1 with 0 events received — known Claude Code npm-install bootstrap flake unrelated to this merge; the second attempt was clean)
- [x] No phase failed — test gate satisfied for both agents

## Update `portingguide.md` (incremental, not at the end)

- [x] Conflict resolution appended live with upstream change / resolution / why / risk (commit `48f7895607`)
- [x] `## Merge 001996 (2026-05-11)` section created mirroring 001980's structure
- [x] Upstream `0a52f80824` (#55562) interaction documented in detail with reasoning trail
- [x] Commit history table extended with merge commit + 3 follow-up fix commits (`bf544922aa`, `1828cea13c`, `a7ad11ec00`)
- [x] Two new "Pre-existing Breakage Repaired" subsections added: `BaseView::Terminal` exhaustiveness and Phase 13 cancel/MessageCompleted race
- [x] No stale guide entries discovered to correct

## Re-merge Fork Main (only if needed)

- [x] Zed `origin/main` unchanged — no re-merge needed
- [x] Helix `origin/main` advanced 3 commits during work — fast-forward merged into `feature/001996-merge-latest-zed` (helix) before bumping ZED_COMMIT

## Finalise

- [x] Pushed `feature/001996-merge-latest-zed` to Zed remote (commit `a767007e53`)
- [x] Wrote `pull_request_zed.md` and `pull_request_helix.md`
- [x] Bumped `ZED_COMMIT` in `sandbox-versions.txt` to `a767007e53304b077eaee5d1872a5631e2ada207` (commit `2aa86b851`)
- [x] Pushed `feature/001996-merge-latest-zed` to Helix remote
- [x] Did not force-push `main`
- [x] Did not open PRs from the agent (per task convention — Helix UI handles PR creation)
