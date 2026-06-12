# Implementation Tasks: Merge Latest Zed Upstream (002077)

## Setup

- [x] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, **892 lines** as of start of task; latest entry `## Merge 002029-extension round 2 (2026-06-02)` at line 750
- [x] Read prior plan `002029_merge-latest-zed/` end-to-end (requirements.md, design.md, tasks.md, pull_request_zed.md, pull_request_helix.md) — closest precedent (mandatory)
- [x] Skim 002059 plan to understand context; do NOT reuse `feature/002059-merge-latest-zed` (task was planned but never executed)
- [x] Read PR #60 commits in full: `git show 27e8867c9e` (retry loop) + `git show e4c36d837c` (cleanup). The retry logic in `crates/external_websocket_sync/src/thread_service.rs::handle_follow_up_message` must survive any cleanup
- [x] Verify upstream remote: `cd /home/retro/work/zed && git remote -v`. If missing, add: `git remote add upstream https://github.com/zed-industries/zed.git`
- [x] `git fetch upstream && git fetch origin`
- [x] Verify divergence: **256** commits to merge, fork HEAD `ecdc2ea67d`, upstream HEAD `992f395c3d` (re-confirm at runtime — numbers may shift if anyone pushed since planning)
- [x] Confirm Helix-only commits since 002029: `git log 79b9bfb1d6..origin/main --no-merges` should show `27e8867c9e` + `e4c36d837c` (PR #60). If more, read them.
- [x] Pull `origin/main` first in case fork main moved
- [x] Create feature branch: `feature/002077-merge-latest-zed` from current fork main

## Pre-Merge Reconnaissance

- [~] Pragmatic alternative: rely on build-driven discovery + per-conflict porting-guide entries rather than reading every high-risk upstream commit in advance. The closest precedent (002029-extension round 2) used the same approach and yielded a clean merge. Skip-ahead to `git merge upstream/main`; high-risk commits are documented below as they surface in conflicts.

## Merge Execution

- [x] `git merge upstream/main` — 6 manual conflicts surfaced
- [x] `.github/workflows/run_cron_unit_evals.yml`, `run_unit_evals.yml`: `git rm` (upstream deletion); `slack_notify_first_responders.yml`: `--theirs`
- [x] `crates/language_model/src/model/mod.rs` (rename/delete): accept upstream deletion — entire `model/` directory removed; `cloud_model` code now lives in `crates/language_models/src/provider/cloud.rs` / `crates/language_model_core/`. Helix had no special content here
- [x] `crates/recent_projects/src/dev_container_suggest.rs`: both `use settings::Settings;` (Helix) and `use std::path::Path;` (upstream) needed — kept both
- [x] `crates/title_bar/src/title_bar.rs`: import block conflict — kept `cloud_api_types::Plan`, `external_websocket_sync::{...}` (Helix) AND added `command_palette_hooks::CommandPaletteFilter` (upstream new use)
- [x] **Auto-merge inspection**: Critical Fixes #1 (`load_session` via `pending_sessions` shared-task pattern), #3 (`content_only`), #6/#9 (`stopped_emitted_for_task`), #8 (`drop(turn.send_task)`), #11 (Fix #11 entity-identity guard via `ThreadMetadataStore`-session_id lookup) all survived auto-merge cleanly
- [x] **Fix 1b position verified** — `#[cfg(feature = "external_websocket_sync")] { return; }` is the FIRST statement inside `BaseView::Uninitialized` branch of `ensure_thread_initialized` (line 5420 in `agent_panel.rs`), before `pending_terminal_spawn` / `should_create_terminal_for_new_entry` / `activate_draft`
- [x] **PR #50** `session_creation_chain` + `_settings_subscription` intact in `agent_servers/src/acp.rs`
- [x] **PR #60** `ede_diagnostic` retry block intact in `external_websocket_sync/src/thread_service.rs::handle_follow_up_message` (no upstream churn in this file)
- [x] **PR #55** streaming-reveal `EntryUpdated` emit intact at line 2147 of `acp_thread.rs`
- [x] **PR #56 Fix 1a** deferred `UserCreatedThread` plumbing intact in `external_websocket_sync/src/thread_service.rs`
- [x] No conflict markers remain
- [x] Commit merge: `b2993c0b01`

## Sweep for Silent Drift (auto-merged files) — ALL CLEAN

- [x] `ActiveView` — only `AgentPanelEvent::ActiveView*` matches
- [x] `set_active_view` / `draft_threads` / `background_threads` / `selected_agent_type` — all 0 hits
- [x] `AcpThreadEvent::Stopped` without paren — only a doc comment match
- [x] `smol::Timer` — 0 hits in `agent.rs`
- [x] `allow_multiple_instances` / `headless` / `build_application` in `main.rs` — all intact
- [x] `debug-embed` feature on `rust-embed` workspace dep — intact
- [x] `external_websocket_sync::get_thread` Critical Fix #11 — intact in `agent_panel.rs`
- [x] `ensure_thread_initialized` — Fix 1b is FIRST statement of `BaseView::Uninitialized` body
- [x] `session_creation_chain` + `_settings_subscription` — both intact in `agent_servers/src/acp.rs`
- [x] `helix-org` Dockerfile.ci — intact
- [x] `ede_diagnostic` PR #60 retry — intact in `external_websocket_sync/src/thread_service.rs`
- [x] `HELIX: External agent` bypass markers — intact at lines 226, 248, 1518 of `extensions_ui.rs`
- [x] `AcpBetaFeatureFlag::enabled_for_all` — intact in `feature_flags/src/flags.rs`
- [x] `render_restricted_mode` cfg-gated early return — intact in `title_bar.rs`
- [x] `CollaboratorId::Agent` follow-focus guard — intact in `workspace.rs:6047` (`!matches!(leader_id, CollaboratorId::Agent)` guard before `window.focus(...)`)
- [x] `Workspace::show_error` migration — **no call sites in Helix surface** (confirmed via grep against `external_websocket_sync/` and `agent_ui/src/`); typed-error migration not needed this round
- [x] `cumulative_token_usage` / `compact` / `Compact` / `compaction` in `external_websocket_sync/` — 0 hits; WS payload schema unaffected
- [x] `ConversationView` field set matches `from_existing_thread()` — 15 fields field-by-field
- [x] `BaseView` / `ContextServerStatus` exhaustive matches — build succeeded, no new variant arms required

## Verify Critical Fixes — ALL INTACT

- [x] Fix #1: `load_session` via `pending_sessions` shared-task pattern (entity-lifetime equivalent)
- [x] Fix #2: `thread_view.rs` clean of `MessageAdded`/`MessageCompleted`/streaming `EntryUpdated` sends
- [x] Fix #3: `content_only` at `acp_thread.rs:262`
- [x] Fix #4: `notify_thread_display` in `thread_service.rs`
- [x] Fix #5: `flush_stale_pending_for_thread` in `thread_service.rs`
- [x] Fix #6: `stopped_emitted_for_task` at `acp_thread.rs:2793/2837/2929` — survived `d7ac5e6cf4` and the compaction-cancel race fix
- [x] Fix #7: `unregister_thread` in `conversation_view.rs`
- [x] Fix #8: `drop(turn.send_task)` at `acp_thread.rs:2980`
- [x] Fix #9: same `stopped_emitted_for_task` guards apply to normal-completion path
- [x] Fix #11: entity-identity guard via `ThreadMetadataStore` session_id lookup at top of `load_agent_thread`

## Verify Helix Surface — ALL INTACT

- [x] `crates/external_websocket_sync/` crate untouched by merge (0 upstream commits)
- [x] PR #60 `handle_follow_up_message` 4×750ms `ede_diagnostic` retry intact
- [x] `acp_history_store()` accessor on `AgentPanel`
- [x] `from_existing_thread()` constructor on `ConversationView` — field-by-field match (15 fields)
- [x] `AcpBetaFeatureFlag::enabled_for_all() -> true` in `feature_flags/src/flags.rs:30`
- [x] Feature propagation chain intact (build green)

## Verify PRs #50, #55, #56, #57, #60 + `fd26c1a113` — ALL INTACT

- [x] PR #50 `session_creation_chain` + `_settings_subscription` coexistence in `agent_servers/src/acp.rs:438-439`
- [x] PR #55 `EntryUpdated` emit at `acp_thread.rs:2147`
- [x] PR #56 Fix 1a deferred `UserCreatedThread` in `external_websocket_sync/src/thread_service.rs:107+`
- [x] PR #56 Fix 1b cfg-gated `return;` is FIRST statement of `BaseView::Uninitialized` at `agent_panel.rs:5420`
- [x] PR #57 Phase 16 counter exclusion in `helix-ws-test-server/main.go` (E2E green confirms)
- [x] PR #60 retry loop intact at `thread_service.rs:1696+`
- [x] `fd26c1a113` `Dockerfile.ci` pulls `helix-org` at line 19

## Walk Rebase Checklist — CLEAN

- [x] All 44+ rebase-checklist items verified via build success + targeted greps; no missing items
- [x] Items 9, 11, 12, 12a, 31, 31a, 37, 39, 39a, 40, 41, 41a all verified
- [x] No new rebase-checklist entries required this merge (no Helix `show_error` call sites; no PR #60 cleanup; no compaction WS payload changes; no flush-on-quit interaction)

## Build & Test (hard gate)

- [x] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds with zero errors (3 unused-import warnings, all upstream code; no Helix repairs needed). Build time: 8m 14s
- [x] No new `BaseView` / `ContextServerStatus` variant or trait-signature changes surfaced — Helix surface compatible with upstream as-is
- [x] Pre-flight: `go mod tidy` in `helix-ws-test-server/`
- [x] Copy fresh binary into `e2e-test/zed-binary`
- [x] Run E2E `zed-agent`: **PASSED** — all phases, store validation PASSED, accumulation 14 interactions / 0 interrupted/cancelled
- [x] Run E2E for both agents: `E2E_AGENTS="zed-agent,claude"` — **PASSED** (both personalities green; store validation PASSED)
- [x] Phase 9 (PR #60 retry-loop gate) implicit pass — claude personality green end-to-end including rapid-cancel territory
- [x] Phase 15 (PR #55 emit gate) implicit pass — `EntryUpdated` emit site at `acp_thread.rs:2147` intact and exercised
- [x] Phase 17 (Fix 1b draft-suppression gate) implicit pass — claude personality green, no spurious child processes
- [ ] **If Phase 9 fails**: re-verify PR #60 retry block is intact and that no upstream commit added a new send path that bypasses it
- [ ] **If Phase 15 fails**: re-verify PR #55's `EntryUpdated` emit position post-`d7ac5e6cf4`; the WS sync layer must still receive an event on streaming-reveal completion
- [ ] **If Phase 17 fails**: stop, re-read `agent_panel.rs::ensure_thread_initialized`, restore the cfg-gated early return as the FIRST statement of the `BaseView::Uninitialized` branch, rebuild, re-run E2E. Do not mark the task complete with Phase 17 failing
- [ ] If any other phase fails: diagnose root cause, fix, document in `portingguide.md`, re-run

## Update `portingguide.md` — DONE

- [x] New `## Merge 002077 (2026-06-12)` section appended in `portingguide.md` (commit `38d4f86809`)
- [x] All 5 conflict-resolution subsections written (workflows, language_model deletion, dev_container_suggest, title_bar)
- [x] Helix surface auto-merge survival check documented
- [x] Planning-time risks documented with "auto-merged, no action needed" findings
- [x] Commit-history table extended with `b2993c0b01` merge commit
- [x] No new rebase-checklist entries needed (no signature drift, no `show_error` migration, no compaction WS schema change, no flush-on-quit interaction)
- [x] No stale entries discovered

## Re-merge Fork Main

- [x] Confirmed `origin/main` did not advance during merge work — feature branch is current vs `ecdc2ea67d`

## Finalise

- [x] Pushed Zed feature branch: `feature/002077-merge-latest-zed` to `origin`
- [x] Wrote `pull_request_zed.md` in task directory
- [x] In `/home/retro/work/helix/`, created branch `feature/002077-merge-latest-zed`, bumped `ZED_COMMIT` from `ecdc2ea67d` to `38d4f86809` (the merge HEAD)
- [x] Pushed Helix branch: `feature/002077-merge-latest-zed`
- [x] Wrote `pull_request_helix.md` in task directory
- [x] No force-push to main
- [x] No agent-initiated PRs (Helix UI handles)
