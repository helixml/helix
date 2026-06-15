# Implementation Tasks: Merge Latest Zed Upstream (002100)

## Setup

- [ ] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, **966 lines** as of start of task; latest entry `## Merge 002077 (2026-06-12)` at line 667
- [ ] Read prior plan `002077_merge-latest-zed/` end-to-end (requirements.md, design.md, tasks.md, pull_request_zed.md, pull_request_helix.md) — closest precedent (mandatory)
- [ ] Skim `002029_merge-latest-zed/`, `001996_merge-latest-zed/`, `001980_merge-latest-zed/` for additional context on critical-fix preservation patterns
- [ ] Verify upstream remote: `cd /home/retro/work/zed && git remote -v`. If missing, add: `git remote add upstream https://github.com/zed-industries/zed.git`
- [ ] `git fetch upstream && git fetch origin`
- [ ] Verify divergence: at planning time, **21** commits to merge, fork HEAD `f82e1c6760`, upstream HEAD `cccc7b2d44` — re-confirm at runtime (numbers will shift if upstream advances)
- [ ] Confirm Helix-only commits since 002077: `git log f82e1c6760..origin/main --no-merges` — expected empty. If non-empty, read each commit before merging.
- [ ] Pull `origin/main` first in case fork main moved
- [ ] Create feature branch: `feature/002100-merge-latest-zed` from current fork main

## Pre-Merge Reconnaissance

- [ ] Pragmatic alternative: rely on build-driven discovery + per-conflict porting-guide entries rather than reading every upstream commit in advance. With only 21 commits and zero churn in `acp_thread/`, `agent/src/`, `workspace.rs`, `zed/src/main.rs`, `title_bar/`, `feature_flags/`, `agent_servers/`, `external_websocket_sync/`, `agent_settings/`, `settings_content/`, the merge is expected to be near-trivial. Skip-ahead to `git merge upstream/main`; the two upstream commits that touch Helix-adjacent files (`1e017d04b9` agent_panel menu link removal, `f39cf25c0b` extensions_ui chip filter) are documented in `design.md` and `requirements.md`.

## Merge Execution

- [ ] `git merge upstream/main` — expected 0–2 manual conflicts (predicted: only `Cargo.lock`; possibly a trivial `Cargo.toml` block if anyone added a Helix-side `[patch.crates-io]` since last check)
- [ ] For any `Cargo.lock` conflict: `git checkout --theirs Cargo.lock && git add Cargo.lock` — regenerates on next build
- [ ] For any `.github/workflows/` conflict: accept upstream (`--theirs` or `git rm` as appropriate)
- [ ] For any `Cargo.toml` conflict (likely from upstream's new `[patch.crates-io] async-process = …` entry): keep both sides — Helix's workspace members + `objc2`/`objc2-app-kit` upstream bumps + the new `async-process` patch
- [ ] **Auto-merge inspection (mandatory regardless of conflict count)**:
  - [ ] `crates/agent_ui/src/agent_panel.rs` — confirm `1e017d04b9` deleted the `Rules Library` menu entry near line 5690 and that Fix 1b (cfg-gated `return;`) is still the FIRST statement of `BaseView::Uninitialized` in `ensure_thread_initialized`
  - [ ] `crates/extensions_ui/src/extensions_ui.rs` — confirm `f39cf25c0b` restructured the chip filter (`.filter_map` → `.filter().map()`) and the three Helix `// HELIX: External agent …` bypass markers survived (line numbers will shift; `grep -n "HELIX: External agent"` should report 3 hits)
  - [ ] `crates/agent_ui/src/threads_archive_view.rs` / `completion_provider.rs` / `config_options.rs` — Helix has no patches here; confirm clean apply
  - [ ] `Cargo.toml` — confirm Helix workspace members (`crates/cloud_api_types`, `crates/external_websocket_sync`) and `rust-embed` `debug-embed` feature intact alongside the new `objc2`/`objc2-app-kit` versions and `async-process` patch
- [ ] **Critical-fix sanity check** (auto-merge survival):
  - [ ] Fix #1: `load_session` / `pending_sessions` shared-task pattern in `crates/agent/src/agent.rs` — file unchanged upstream this window
  - [ ] Fix #3: `content_only` in `crates/acp_thread/src/acp_thread.rs` — file unchanged upstream
  - [ ] Fix #6/#9: `stopped_emitted_for_task` guard sites in `acp_thread.rs` — unchanged
  - [ ] Fix #8: `drop(turn.send_task)` — unchanged
  - [ ] Fix #11: entity-identity guard at top of `load_agent_thread` in `agent_panel.rs` — file changed only in menu region
- [ ] **PR #50** `session_creation_chain` + `_settings_subscription` intact in `agent_servers/src/acp.rs` (file unchanged upstream)
- [ ] **PR #55** streaming-reveal `EntryUpdated` emit intact in `acp_thread.rs` (file unchanged upstream)
- [ ] **PR #56 Fix 1a** deferred `UserCreatedThread` plumbing intact in `external_websocket_sync/src/thread_service.rs` (file unchanged either way)
- [ ] **PR #56 Fix 1b** cfg-gated `return;` is FIRST statement of `BaseView::Uninitialized` in `agent_panel.rs::ensure_thread_initialized`
- [ ] **PR #60** `ede_diagnostic` retry block intact in `external_websocket_sync/src/thread_service.rs::handle_follow_up_message` (file unchanged either way)
- [ ] No conflict markers remain (`grep -rn "<<<<<<<\|=======\|>>>>>>>" .` returns 0)
- [ ] Commit the merge

## Sweep for Silent Drift (auto-merged files)

- [ ] `ActiveView` — only `AgentPanelEvent::ActiveView*` matches
- [ ] `set_active_view` / `draft_threads` / `background_threads` / `selected_agent_type` — all 0 hits
- [ ] `AcpThreadEvent::Stopped` without paren — only doc-comment matches
- [ ] `smol::Timer` in `agent.rs` — 0 hits
- [ ] `allow_multiple_instances` / `headless` / `build_application` in `main.rs` — intact
- [ ] `debug-embed` feature on `rust-embed` workspace dep — intact
- [ ] `ensure_thread_initialized` — Fix 1b is FIRST statement of `BaseView::Uninitialized` body
- [ ] `session_creation_chain` + `_settings_subscription` — both intact in `agent_servers/src/acp.rs`
- [ ] `helix-org` Dockerfile.ci — intact
- [ ] `ede_diagnostic` PR #60 retry — intact in `thread_service.rs`
- [ ] `// HELIX: External agent` bypass markers — 3 hits in `extensions_ui.rs` (line numbers shifted from current 226/248/1518; confirm semantically equivalent)
- [ ] `AcpBetaFeatureFlag::enabled_for_all` — intact in `feature_flags/src/flags.rs`
- [ ] `render_restricted_mode` cfg-gated early return — intact in `title_bar.rs`
- [ ] `CollaboratorId::Agent` follow-focus guard — intact in `workspace.rs`
- [ ] `Workspace::show_error` migration — no new call sites in Helix surface (no upstream churn in `workspace.rs` this window)
- [ ] `cumulative_token_usage` / `compact` / `compaction` in `external_websocket_sync/` — 0 hits (unchanged from 002077)
- [ ] `ConversationView` field set matches `from_existing_thread()` (15 fields)
- [ ] `BaseView` / `ContextServerStatus` exhaustive matches — build succeeds without new arms

## Verify Critical Fixes

- [ ] Fix #1 (`load_session` / `pending_sessions`)
- [ ] Fix #2 (`thread_view.rs` clean of duplicate WS sends)
- [ ] Fix #3 (`content_only` at the original location)
- [ ] Fix #4 (`notify_thread_display` in `thread_service.rs`)
- [ ] Fix #5 (`flush_stale_pending_for_thread` in `thread_service.rs`)
- [ ] Fix #6 (`stopped_emitted_for_task` guard sites)
- [ ] Fix #7 (`unregister_thread` in `conversation_view.rs`)
- [ ] Fix #8 (`drop(turn.send_task)`)
- [ ] Fix #9 (same `stopped_emitted_for_task` guards on normal-completion path)
- [ ] Fix #11 (entity-identity guard via `ThreadMetadataStore` session_id lookup)

## Verify Helix Surface

- [ ] `crates/external_websocket_sync/` crate untouched (0 upstream commits)
- [ ] PR #60 `handle_follow_up_message` 4×750ms `ede_diagnostic` retry intact
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] `from_existing_thread()` constructor on `ConversationView` — field-by-field match
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` in `feature_flags/src/flags.rs`
- [ ] Feature propagation chain intact (build green)

## Verify PRs #50, #55, #56, #57, #60 + `fd26c1a113`

- [ ] PR #50 `session_creation_chain` + `_settings_subscription` coexistence in `agent_servers/src/acp.rs`
- [ ] PR #55 `EntryUpdated` emit in `acp_thread.rs`
- [ ] PR #56 Fix 1a deferred `UserCreatedThread` in `external_websocket_sync/src/thread_service.rs`
- [ ] PR #56 Fix 1b cfg-gated `return;` is FIRST statement of `BaseView::Uninitialized` in `agent_panel.rs`
- [ ] PR #57 Phase 16 counter exclusion in `helix-ws-test-server/main.go` (E2E green confirms)
- [ ] PR #60 retry loop intact at `thread_service.rs::handle_follow_up_message`
- [ ] `fd26c1a113` `Dockerfile.ci` pulls `helix-org`

## Walk Rebase Checklist

- [ ] All 44+ rebase-checklist items verified via build success + targeted greps; no missing items
- [ ] Items 9, 11, 12, 12a, 31, 31a, 37, 39, 39a, 40, 41, 41a all verified
- [ ] No new rebase-checklist entries required this merge (predict: none — confirm at end)

## Build & Test (hard gate)

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds with zero errors (warnings tolerable if all in upstream code)
- [ ] No new `BaseView` / `ContextServerStatus` variant or trait-signature changes surface
- [ ] Pre-flight: `go mod tidy` in `helix-ws-test-server/`
- [ ] Copy fresh binary into `e2e-test/zed-binary`
- [ ] Run E2E `zed-agent`: all phases pass, store validation PASSED
- [ ] Run E2E for both agents: `E2E_AGENTS="zed-agent,claude"` — both personalities green
- [ ] Phase 9 (PR #60 retry-loop gate) — pass (implicit via claude-personality green)
- [ ] Phase 15 (PR #55 emit gate) — pass
- [ ] Phase 16 (PR #56 Fix 1a + PR #57) — pass
- [ ] Phase 17 (Fix 1b draft-suppression gate) — pass
- [ ] **If Phase 9 fails**: re-verify PR #60 retry block intact; check no upstream commit added a new send path bypassing it
- [ ] **If Phase 15 fails**: re-verify PR #55's `EntryUpdated` emit position; the WS sync layer must still receive an event on streaming-reveal completion
- [ ] **If Phase 17 fails**: stop, re-read `agent_panel.rs::ensure_thread_initialized`, restore Fix 1b's first-statement position, rebuild, re-run E2E. Do not mark the task complete with Phase 17 failing
- [ ] If any other phase fails: diagnose root cause, fix, document in `portingguide.md`, re-run

## Update `portingguide.md`

- [ ] New `## Merge 002100 (2026-06-15)` section appended at top of merge-history list
- [ ] Window summary subsection: "21 upstream commits over 3 days; smallest catch-up window in this series."
- [ ] Conflict-resolution subsections written (or explicit "0 conflicts, auto-merge clean" note)
- [ ] Helix-surface auto-merge survival check subsection (per-area confirmation)
- [ ] `1e017d04b9` agent menu link removal — Fix 1b position re-verification subsection
- [ ] `f39cf25c0b` extensions_ui chip filter — three `// HELIX:` bypass marker survival subsection
- [ ] PR #60 retry-loop survival check subsection
- [ ] Cargo.toml / Cargo.lock notes (objc2 bumps + async-process patch)
- [ ] Commit-history table extended with this merge's commits
- [ ] No new rebase-checklist entries unless something actually broke (predict: none)
- [ ] No stale entries discovered (or correct/delete them if so)

## Re-merge Fork Main

- [ ] Confirm `origin/main` did not advance during merge work; if it did, `git pull --rebase origin main` or `git merge origin/main` into the feature branch and re-run the build + critical-fix sweep

## Finalise

- [ ] Push Zed feature branch: `feature/002100-merge-latest-zed` to `origin`
- [ ] Write `pull_request_zed.md` in this task directory
- [ ] In `/home/retro/work/helix/`, create branch `feature/002100-merge-latest-zed`, bump `ZED_COMMIT` from `f82e1c676099470ecd17590878a00bd25b342f82` to the new merge HEAD
- [ ] Push Helix branch: `feature/002100-merge-latest-zed`
- [ ] Write `pull_request_helix.md` in this task directory
- [ ] No force-push to main
- [ ] No agent-initiated PRs (Helix UI handles)
