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

## Verify Critical Fixes (the 10 active fixes — #10 stays retired)

- [ ] Fix #1: `load_session` keeps `Entity<NativeAgent>` alive (survives compaction cluster + `620ceaaaca` flush-on-quit)
- [ ] Fix #2: `thread_view.rs` has no `MessageAdded` / `MessageCompleted` / streaming `EntryUpdated` sends
- [ ] Fix #3: `content_only` present in `acp_thread.rs`
- [ ] Fix #4: `notify_thread_display` called in `thread_service.rs`
- [ ] Fix #5: `flush_stale_pending_for_thread` present in `thread_service.rs`
- [ ] Fix #6: `stopped_emitted_for_task` invariant — exactly one Stopped per `send()`, all paths (survives `d7ac5e6cf4`'s ToolCall-status rewrite + `5c90b0664f`'s compaction-cancel race fix)
- [ ] Fix #7: `unregister_thread` called from `conversation_view.rs`
- [ ] Fix #8: `drop(turn.send_task)` not `cx.background_spawn(turn.send_task)`
- [ ] Fix #9: `stopped_emitted_for_task` guards normal-completion Stopped emission
- [ ] Fix #11: entity-identity guard `external_websocket_sync::get_thread(...)` at top of `load_agent_thread` in `agent_panel.rs` (`thread_id`-based form)

## Verify Helix Surface

- [ ] `crates/external_websocket_sync/` crate intact (all source files)
- [ ] **PR #60 `handle_follow_up_message` 4×750ms `ede_diagnostic` retry intact**
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] `from_existing_thread()` constructor on `ConversationView`, matching current field set + `ThreadView::new` arg list
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` in `feature_flags/src/flags.rs`
- [ ] Feature propagation chain intact: `zed/Cargo.toml` declares `external_websocket_sync = ["agent_ui/external_websocket_sync", ...]`; `title_bar` dep `optional = true`

## Verify PRs #50, #55, #56, #57, #60 + `fd26c1a113`

- [ ] **PR #50** `session_creation_chain` field on `AcpConnection` present; coexists with `_settings_subscription`
- [ ] **PR #50** `test_concurrent_session_creation_is_serialized` compiles and (locally) passes
- [ ] **PR #55** `EntryUpdated` emit after streaming-reveal drain present in `acp_thread.rs` — re-anchored against `d7ac5e6cf4`'s tool-call-status rewrite; document the post-merge location
- [ ] **PR #56 Fix 1a** `defer_user_created_thread_until_first_user_message` plumbing in `external_websocket_sync`
- [ ] **PR #56 Fix 1b** `#[cfg(feature = "external_websocket_sync")] { return; }` is the FIRST statement of `BaseView::Uninitialized` branch
- [ ] **PR #56** the unit test asserting deferred `UserCreatedThread` emit compiles and passes
- [ ] **PR #57** Phase 16 counter excludes Phase 10's synthetic `UserCreatedThread` ID in `helix-ws-test-server/main.go`
- [ ] **PR #60** retry loop in `handle_follow_up_message` intact (no upstream churn this window — guard against careless cleanup)
- [ ] **`fd26c1a113`** `Dockerfile.ci` still pulls `helix-org`

## Walk Rebase Checklist

- [ ] All numbered items in `portingguide.md` §"Rebase Checklist" walked
- [ ] Pay special attention to items 9 (cfg-gated `agent_panel.rs` blocks — Fix 1b position), 11 (`ConversationView` field set), 12 (`AgentConnection` trait impls), 12a (`Stopped` patterns), 31/31a/37 (`acp_thread.rs` cancel/Stopped — `d7ac5e6cf4` + compaction-cancel race risk), 39 (`--allow-multiple-instances`), 39a (`--headless`), 40 (`debug-embed`), 41 (`smol::Timer`), 41a (`Stopped(_)` test pattern), plus 002029 additions on Fix 1b first-statement and `supports_delete(&self, &App)` signature
- [ ] **New checklist item (002077)**: "All Helix `Workspace::show_error` call sites use the new `<E: WorkspaceError>` generic signature (`215ca2fb0b`+`83aa943705`)."
- [ ] **New checklist item (002077)**: "PR #60 retry block in `external_websocket_sync/src/thread_service.rs::handle_follow_up_message` retains the 4×750ms `ede_diagnostic` backoff. Phase 9 of the E2E is the regression gate."
- [ ] **New checklist item (002077, conditional)**: "If `620ceaaaca` flush-on-quit was gated behind `not(feature = "external_websocket_sync")`, document the rationale; otherwise document why the WS-authoritative store tolerates the upstream flush."
- [ ] **New checklist item (002077, conditional)**: "If the compaction cluster introduced new WS payload fields, the schema bump is documented and the Helix API server tolerates them."

## Build & Test (hard gate)

- [x] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds with zero errors (3 unused-import warnings, all upstream code; no Helix repairs needed). Build time: 8m 14s
- [x] No new `BaseView` / `ContextServerStatus` variant or trait-signature changes surfaced — Helix surface compatible with upstream as-is
- [ ] Pre-flight: `cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server && go mod tidy`
- [ ] Copy fresh binary into `e2e-test/zed-binary`: `cp /home/retro/work/helix/zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] Run E2E `zed-agent`: `cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test && ./run_docker_e2e.sh`
- [ ] All 17 phases pass for `zed-agent`, with:
  - **Phase 9** as the explicit gate that PR #60's `ede_diagnostic` retry-loop survived
  - **Phase 15** as the explicit gate that PR #55's `EntryUpdated` emit survived `d7ac5e6cf4`'s rewrite
  - **Phase 17** as the explicit gate that PR #56 Fix 1b draft suppression survived
- [ ] Run E2E for both agents: `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` (one retry permitted for Claude Code npm-install bootstrap flake — see 001996 lesson)
- [ ] All 17 phases pass for `claude` as well
- [ ] **If Phase 9 fails**: re-verify PR #60 retry block is intact and that no upstream commit added a new send path that bypasses it
- [ ] **If Phase 15 fails**: re-verify PR #55's `EntryUpdated` emit position post-`d7ac5e6cf4`; the WS sync layer must still receive an event on streaming-reveal completion
- [ ] **If Phase 17 fails**: stop, re-read `agent_panel.rs::ensure_thread_initialized`, restore the cfg-gated early return as the FIRST statement of the `BaseView::Uninitialized` branch, rebuild, re-run E2E. Do not mark the task complete with Phase 17 failing
- [ ] If any other phase fails: diagnose root cause, fix, document in `portingguide.md`, re-run

## Update `portingguide.md` (incremental, not at the end)

- [ ] Each conflict resolution appended live (upstream change / resolution / why / risk)
- [ ] New top-level `## Merge 002077 (2026-06-12)` section created at the top of the merge-history list, mirroring 002029-extension round 2 structure
- [ ] **Mandatory subsection**: "`d7ac5e6cf4` Preserve waiting tool call status — PR #55 emit + Critical Fix #6 invariant" — document post-merge emit location, confirm exactly-once `Stopped`
- [ ] **Mandatory subsection**: "Compaction cluster (`e5052961af` et al.) — WS payload schema check" — record whether the cluster added new payload fields
- [ ] **Mandatory subsection**: "`620ceaaaca` Flush-on-quit — Helix WS-authoritative store interaction" — record the reachability analysis and any `not(external_websocket_sync)` gate
- [ ] **Mandatory subsection**: "`215ca2fb0b` Typed workspace errors — Helix `show_error` call-site migration" — list each call site and chosen migration approach
- [ ] **Mandatory subsection**: "`116e4bc184` Inherit source agent without draft content vs Helix PR #56 Fix 1b" — confirm first-statement position
- [ ] **Mandatory subsection**: "`27191913e9` + `0bc6c76fcf` Token usage changes — WS schema check"
- [ ] **Mandatory subsection**: "PR #60 (`27e8867c9e`/`e4c36d837c`) `ede_diagnostic` retry-loop — survival check" — confirm retry block intact, document any new event path that bypasses it
- [ ] Subsection (conditional): "`89cac4944d` Sandbox write-path + `9baefe701e` auto_compact — settings field coexistence with Helix"
- [ ] Any "Pre-existing Breakage Repaired" subsections written for build fixes
- [ ] Commit-history table at bottom of `portingguide.md` extended with this merge's commits and any follow-up fix commits
- [ ] Rebase checklist extended only with **net-new** fragilities discovered in this window (do not invent updates)
- [ ] Stale guide entries discovered along the way are corrected or deleted

## Re-merge Fork Main (only if needed)

- [ ] Check whether `origin/main` advanced during merge work: `git fetch origin && git log feature/002077-merge-latest-zed..origin/main`
- [ ] If yes: `git merge origin/main` into the feature branch, re-build, re-run E2E. (PR #60 demonstrated active WS-sync-layer development during the planning window; not unlikely.)

## Finalise

- [ ] Push feature branch to Zed remote: `git push -u origin feature/002077-merge-latest-zed`
- [ ] Write `pull_request_zed.md` in this task directory with summary of upstream changes (highlight: compaction cluster, `d7ac5e6cf4` tool-call-status, `215ca2fb0b` typed errors, `116e4bc184` source-agent inheritance), conflict resolutions, and validation results (Phase 9, 15, 17 all green)
- [ ] In `/home/retro/work/helix/`, create branch `feature/002077-merge-latest-zed`, bump `ZED_COMMIT` in `sandbox-versions.txt` from `79b9bfb1d60cbef5b14ba7e3992ba5e8f6eb335c` to the new Zed merge HEAD, commit. **Note**: this bump also ships PR #60's retry loop, which was never bumped into the sandbox after #60 merged
- [ ] Push the Helix branch: `git push -u origin feature/002077-merge-latest-zed`
- [ ] Write `pull_request_helix.md` in this task directory — call out that PR #60 is included in this bump (was not in the previous `ZED_COMMIT`)
- [ ] Do NOT force-push `main` (Zed or Helix) without explicit user approval
- [ ] Do NOT open PRs from the agent — the Helix UI handles PR creation per task convention
