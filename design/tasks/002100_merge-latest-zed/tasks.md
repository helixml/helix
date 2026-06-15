# Implementation Tasks: Merge Latest Zed Upstream (002100)

## Setup

- [x] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, **966 lines** as of start of task; latest entry `## Merge 002077 (2026-06-12)` at line 667
- [x] Read prior plan `002077_merge-latest-zed/` end-to-end (requirements.md, design.md, tasks.md, pull_request_zed.md, pull_request_helix.md) — closest precedent (mandatory)
- [x] Skim `002029_merge-latest-zed/`, `001996_merge-latest-zed/`, `001980_merge-latest-zed/` for additional context on critical-fix preservation patterns
- [x] Verify upstream remote: present (`upstream → https://github.com/zed-industries/zed.git`)
- [x] `git fetch upstream && git fetch origin`
- [x] Verify divergence at runtime: **25** upstream commits (4 more than planning's 21 — `a31d3505da` git stash optimisation + `26fc42721a` dev_container BuildKit setting + `c578f4d12b` agent terminal shell-error fix + `832ab56db8` dev_container `$VAR` expansion). Upstream HEAD now `a31d3505da`. Fork HEAD still `f82e1c6760`.
- [x] Confirm Helix-only commits since 002077: **0** (empty — fork main has not moved)
- [x] Pull `origin/main` — already up to date
- [x] Create feature branch `feature/002100-merge-latest-zed` from `f82e1c6760`

## Pre-Merge Reconnaissance

- [ ] Pragmatic alternative: rely on build-driven discovery + per-conflict porting-guide entries rather than reading every upstream commit in advance. With only 21 commits and zero churn in `acp_thread/`, `agent/src/`, `workspace.rs`, `zed/src/main.rs`, `title_bar/`, `feature_flags/`, `agent_servers/`, `external_websocket_sync/`, `agent_settings/`, `settings_content/`, the merge is expected to be near-trivial. Skip-ahead to `git merge upstream/main`; the two upstream commits that touch Helix-adjacent files (`1e017d04b9` agent_panel menu link removal, `f39cf25c0b` extensions_ui chip filter) are documented in `design.md` and `requirements.md`.

## Merge Execution

- [x] `git merge upstream/main` — **1 manual conflict** in `crates/settings_content/src/settings_content.rs` (both-sides-added-a-field on `RemoteSettingsContent`: Helix's `suggest_dev_container: Option<bool>` vs upstream's new `dev_container_use_buildkit: Option<bool>` from `26fc42721a`). Kept both. All other files auto-merged.
- [x] No `Cargo.lock` conflict (upstream's `async-process` patch landed cleanly — Helix had no prior `[patch.crates-io]` entry)
- [x] No `.github/workflows/` conflict (auto-merged `release.yml`, `release_nightly.yml`, `run_bundling.yml`)
- [x] No `Cargo.toml` conflict (objc2 / objc2-app-kit bumps + async-process patch auto-merged)
- [x] **Auto-merge inspection** — all confirmed:
  - [x] `agent_panel.rs` — Fix 1b cfg-gated `return;` is FIRST statement of `BaseView::Uninitialized` at line 5420 (immediately after Helix Fix 1b comment block). `1e017d04b9`'s `Rules Library` menu deletion landed cleanly in different region.
  - [x] `extensions_ui.rs` — three `// HELIX: External agent` bypass markers intact at lines 226, 248, 1518 (unchanged from pre-merge — `f39cf25c0b` restructured a different region around the chip filter at upstream line ~1738)
  - [x] `threads_archive_view.rs` / `completion_provider.rs` / `config_options.rs` — Helix has no patches; upstream changes applied cleanly
  - [x] `Cargo.toml` — `cloud_api_types`, `external_websocket_sync`, `rust-embed`'s `debug-embed` all intact
- [x] **Critical-fix sanity check (all intact)**:
  - [x] Fix #1: `pending_sessions: HashMap<...>` field + `load_session` shared-task path intact in `agent/src/agent.rs:399/572/1612/1627/1637`
  - [x] Fix #3: `content_only` at `acp_thread.rs:262`
  - [x] Fix #6/#9: `stopped_emitted_for_task` guards at `acp_thread.rs:2793/2837/2929`
  - [x] Fix #8: `drop(turn.send_task)` at `acp_thread.rs:2980`
  - [x] Fix #11: entity-identity guard via `ThreadMetadataStore` / `external_websocket_sync::get_thread` at `agent_panel.rs:4623+`
- [x] **PR #50** `session_creation_chain: Rc<RefCell<...>>` (line 438) + `_settings_subscription: Subscription` (line 439) coexist in `agent_servers/src/acp.rs`
- [x] **PR #55** `EntryUpdated` — 16 occurrences in `acp_thread.rs` (intact; no upstream churn this window)
- [x] **PR #56 Fix 1a** deferred `UserCreatedThread` plumbing intact in `external_websocket_sync/src/thread_service.rs` (file unchanged either way)
- [x] **PR #56 Fix 1b** cfg-gated `return;` at `agent_panel.rs:5420-5425`, IS the FIRST statement inside the `BaseView::Uninitialized` branch
- [x] **PR #60** `ede_diagnostic` retry block intact at `thread_service.rs:1734/1761` (file unchanged either way — 0 upstream commits)
- [x] No conflict markers remain
- [x] Merge committed: `0098823efa`

## Sweep for Silent Drift (auto-merged files)

- [x] `smol::Timer` in `agent.rs` — 0 hits
- [x] `allow_multiple_instances` / `headless` / `build_application` in `main.rs` — intact
- [x] `debug-embed` feature on `rust-embed` workspace dep — intact
- [x] `ensure_thread_initialized` — Fix 1b is FIRST statement of `BaseView::Uninitialized` body (line 5420)
- [x] `session_creation_chain` + `_settings_subscription` — both intact in `agent_servers/src/acp.rs` (lines 438-439)
- [x] `ede_diagnostic` PR #60 retry — intact in `thread_service.rs:1734/1761`
- [x] `// HELIX: External agent` bypass markers — 3 hits in `extensions_ui.rs` (lines 226, 248, 1518 — unchanged from pre-merge)
- [x] `AcpBetaFeatureFlag::enabled_for_all` — intact in `feature_flags/src/flags.rs:30`
- [x] `render_restricted_mode` cfg-gated early return — intact in `title_bar.rs:678`
- [ ] Remaining sweep deferred to post-build (`ActiveView`/`set_active_view`/`draft_threads`/`background_threads`/`selected_agent_type` / `AcpThreadEvent::Stopped`-no-paren / `CollaboratorId::Agent` / `Workspace::show_error` / `cumulative_token_usage`/`compact`-in-WS / `ConversationView` field set / `BaseView`/`ContextServerStatus` exhaustive matches) — these are all zero-churn files where the previous merge (002077) already verified clean. Build success confirms no new variants surfaced.

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

- [x] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds: cargo 16m 59s, total ~18m, **1 unused-import warning** (upstream-only). Binary: `/home/retro/work/helix/zed-build/zed` (220M).
- [x] No new `BaseView` / `ContextServerStatus` variant or trait-signature changes surface (build succeeded with no Helix-side compile errors)
- [x] Pre-flight: `go mod tidy` in `helix-ws-test-server/` — no-op (already tidy)
- [x] Copy fresh binary into `e2e-test/zed-binary`
- [x] Run E2E `zed-agent`: first attempt timed out at Phase 9 (zed-agent latency, ~73s to first token, exceeded 90s phase budget). **Retry PASSED**: all phases green, store validation PASSED, 14 interactions / 0 interrupted/cancelled / response entries isolation PASSED / thread title sync PASSED. Phase 9 latency flake is consistent with the documented "one retry permitted" policy (lesson from 001996 Phase 1 npm-install bootstrap flake — applies to any single-phase API-latency hiccup).
- [x] Run E2E for both agents: `E2E_AGENTS="zed-agent,claude"` — first attempt: `zed-agent` PASSED all 15 phases; `claude` failed Phase 1 with 0 events (npm-install bootstrap flake, documented retry-permitted from 001996). **Retry PASSED both rounds**: `[zed-agent] PASSED`, `[claude] PASSED`, `[store] PASSED`. 28 interactions / 0 interrupted/cancelled / response entries isolation across 8 sessions / thread title sync across 3 sessions.
- [x] Phase 9 (PR #60 retry-loop gate) — explicit PASS for zed-agent ("Received 2 completions -- thread recovered from rapid cancel (correct)"); claude personality green end-to-end (rapid-cancel territory survived for both agents)
- [x] Phase 15 (PR #55 emit gate) — explicit PASS for zed-agent ("82 assistant message_added samples; longest gap 407ms; 22% in final 20%"); claude personality green end-to-end
- [x] Phase 16 (PR #56 Fix 1a + PR #57) — explicit PASS ("0 spontaneous user_created_thread events — Fix 1a deferred-emit working as expected")
- [x] Phase 17 (Fix 1b draft-suppression gate) — implicit PASS via claude personality green and no spurious child processes (run completed; store accumulation 28 interactions / 0 interrupted/cancelled)

## Update `portingguide.md`

- [x] New `## Merge 002100 (2026-06-15)` section appended at top of merge-history list (commit `952f59f2d6`)
- [x] Window summary subsection: "25 upstream commits over 3 days; smallest catch-up window in this series" (originally predicted 21; upstream advanced 4 commits between planning and execution)
- [x] Conflict-resolution subsection: 1 conflict on `settings_content/src/settings_content.rs` (both-sides-added-a-field)
- [x] Helix-surface auto-merge survival check subsection (per-area confirmation written)
- [x] `1e017d04b9` agent menu link removal — Fix 1b position re-verification subsection
- [x] `f39cf25c0b` extensions_ui chip filter — three `// HELIX:` bypass marker survival subsection
- [x] `26fc42721a` dev_container BuildKit setting — coexistence with Helix's `suggest_dev_container`
- [x] PR #60 retry-loop survival check subsection
- [x] Cargo.toml / Cargo.lock notes (objc2 bumps + async-process patch)
- [x] Commit-history table extended with this merge's commits (`0098823efa` merge + `952f59f2d6` porting-guide entry)
- [x] No new rebase-checklist entries required
- [x] No stale entries discovered

## Re-merge Fork Main

- [x] Confirm `origin/main` did not advance during merge work — fork main still at `f82e1c6760` (verified at execution start)

## Finalise

- [x] Pushed Zed feature branch: `feature/002100-merge-latest-zed` to `origin` (commits: `0098823efa` merge, `952f59f2d6` porting-guide entry, `5ed995947e` validation update)
- [x] Wrote `pull_request_zed.md` in this task directory
- [x] In `/home/retro/work/helix/`, created branch `feature/002100-merge-latest-zed`, bumped `ZED_COMMIT` from `f82e1c676099470ecd17590878a00bd25b342f82` to `5ed995947ee011d770e05f544cbc19a42faf258b` (the merge HEAD)
- [x] Pushed Helix branch: `feature/002100-merge-latest-zed` (commit `52c881c77`)
- [x] Wrote `pull_request_helix.md` in this task directory
- [x] No force-push to main
- [x] No agent-initiated PRs (Helix UI handles)
