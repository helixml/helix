# Implementation Tasks: Merge Latest Zed Upstream (002153)

## Setup

- [ ] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, **1109 lines** as of start of task; latest entry `## Merge 002100-extension (2026-06-18)` at line 670
- [ ] Read prior plan `002100_merge-latest-zed/` end-to-end (requirements.md, design.md, tasks.md, pull_request_zed.md, pull_request_helix.md) — closest precedent (mandatory)
- [ ] Skim `002077_merge-latest-zed/` and `002029_merge-latest-zed/` for additional conflict-pattern context
- [ ] Read PR #65 commit (`9546054e68`) in full: `git show 9546054e68` — understand the Error arm + `ChatResponseError` + `TEST_WEBSOCKET_SERVICE_GUARD` surface that must survive the merge
- [ ] Add upstream remote: `git remote add upstream https://github.com/zed-industries/zed.git` (only if missing)
- [ ] `git fetch upstream && git fetch origin`
- [ ] Measure divergence: `git log --oneline upstream/main ^e45e42af6e | wc -l` and record upstream HEAD SHA
- [ ] Confirm fork HEAD: still `9546054e68` (verify `git log --oneline origin/main -1`)
- [ ] Confirm `sandbox-versions.txt` ZED_COMMIT already at `9546054e68...` (no pre-merge catch-up needed)
- [ ] Pull `origin/main` — confirm already up to date
- [ ] Create feature branch `feature/002153-merge-latest-zed` from `9546054e68`

## Pre-Merge Reconnaissance

- [ ] Inspect upstream diff for changes to `crates/acp_thread/src/connection.rs` since `e45e42af6e` — any `AgentConnection` or `StubAgentConnection` changes that could conflict with PR #65's `fail_turn()`?
- [ ] Skim `git diff e45e42af6e..upstream/main -- crates/agent_ui/src/agent_panel.rs | wc -l` — gauge agent_panel.rs churn to estimate Fix 1b position-shift risk
- [ ] Skim for any new `BaseView` or `ContextServerStatus` variants in the upstream diff

## Merge Execution

- [ ] `git merge upstream/main` — note any conflicts
- [ ] Resolve each conflict immediately, updating `portingguide.md` as each is resolved
- [ ] `Cargo.lock` conflict: `git checkout --theirs Cargo.lock`
- [ ] `.github/workflows/` conflicts: `git checkout --theirs` (Helix doesn't use Zed's CI)
- [ ] **Auto-merge inspection** — verify each even if git reports "auto-merged":
  - [ ] `agent_panel.rs` — read full `ensure_thread_initialized` body; Fix 1b cfg-gated `return;` must be FIRST statement of `BaseView::Uninitialized`; record new line number
  - [ ] `extensions_ui.rs` — `grep -n "HELIX: External agent"` — expect 3 hits; record new line numbers
  - [ ] `crates/acp_thread/src/connection.rs` — `fail_turn()` on `StubAgentConnection` still present; no upstream addition overwriting it
  - [ ] `external_websocket_sync/src/thread_service.rs` — Error arm + ChatResponseError emit intact; `TEST_WEBSOCKET_SERVICE_GUARD` present; PR #60/#63/#64 surface intact
  - [ ] `external_websocket_sync/src/types.rs` — `SyncEvent::ChatResponseError` variant intact
- [ ] **Critical-fix sanity check**:
  - [ ] Fix #1: `pending_sessions` field + `load_session` shared-task path in `agent/src/agent.rs`
  - [ ] Fix #3: `content_only` in `acp_thread.rs`
  - [ ] Fix #6/#9: `stopped_emitted_for_task` guards in `acp_thread.rs`
  - [ ] Fix #8: `drop(turn.send_task)` in `acp_thread.rs`
  - [ ] Fix #11: entity-identity guard via `ThreadMetadataStore` at top of `load_agent_thread` in `agent_panel.rs`
- [ ] **PR #50**: `session_creation_chain` + `_settings_subscription` coexist in `agent_servers/src/acp.rs`
- [ ] **PR #55**: `EntryUpdated` occurrences in `acp_thread.rs` — count still ~16; no upstream rewrite of the emit site
- [ ] **PR #56 Fix 1a**: deferred `UserCreatedThread` plumbing intact in `external_websocket_sync/src/thread_service.rs`
- [ ] **PR #56 Fix 1b**: cfg-gated `return;` at new line in `agent_panel.rs` is FIRST statement of `BaseView::Uninitialized`
- [ ] No conflict markers remain (`git diff --check`)
- [ ] Merge committed; record merge SHA

## Sweep for Silent Drift (auto-merged files)

- [ ] `smol::Timer` in `agent.rs` — 0 hits
- [ ] `allow_multiple_instances` / `headless` / `build_application` in `main.rs` — intact
- [ ] `debug-embed` feature on `rust-embed` workspace dep — intact
- [ ] `ensure_thread_initialized` — Fix 1b is FIRST statement of `BaseView::Uninitialized` body (confirm by reading full function)
- [ ] `session_creation_chain` + `_settings_subscription` — both intact in `agent_servers/src/acp.rs`
- [ ] `ede_diagnostic` PR #60 retry — intact in `thread_service.rs`
- [ ] PR #63 `force_reset_session` + `clear_keep_alive` — intact in `thread_service.rs`
- [ ] PR #64 `agent_ready` re-emit — intact in `thread_service.rs`
- [ ] `// HELIX: External agent` bypass markers — 3 hits in `extensions_ui.rs` (record new line numbers)
- [ ] `AcpBetaFeatureFlag::enabled_for_all` — intact in `feature_flags/src/flags.rs`
- [ ] `render_restricted_mode` cfg-gated early return — intact in `title_bar.rs` (record new line number)
- [ ] `fail_turn` — intact in `crates/acp_thread/src/connection.rs`
- [ ] `SyncEvent::ChatResponseError` — intact in `crates/external_websocket_sync/src/types.rs`
- [ ] `TEST_WEBSOCKET_SERVICE_GUARD` — intact in `crates/external_websocket_sync/src/thread_service.rs`

## Verify Critical Fixes

- [ ] Fix #1 (`load_session` / `pending_sessions` shared-task) — `agent/src/agent.rs`
- [ ] Fix #2 (`thread_view.rs` clean of duplicate WS sends) — check file is not modified in unexpected ways
- [ ] Fix #3 (`content_only`) — `acp_thread.rs`
- [ ] Fix #4 (`notify_thread_display`) — `thread_service.rs`
- [ ] Fix #5 (`flush_stale_pending_for_thread`) — `thread_service.rs`
- [ ] Fix #6 (`stopped_emitted_for_task` guard sites) — `acp_thread.rs`
- [ ] Fix #7 (`unregister_thread`) — `conversation_view.rs`
- [ ] Fix #8 (`drop(turn.send_task)`) — `acp_thread.rs`
- [ ] Fix #9 (same `stopped_emitted_for_task` guards on normal-completion path) — same sites as #6
- [ ] Fix #11 (entity-identity guard via `ThreadMetadataStore` session_id lookup) — `agent_panel.rs`

## Verify Helix Surface (PRs #50, #55, #56, #57, #60, #63, #64, #65 + `fd26c1a113`)

- [ ] PR #50 `session_creation_chain` + `_settings_subscription` coexistence — `agent_servers/src/acp.rs`
- [ ] PR #55 `EntryUpdated` emit — occurrences in `acp_thread.rs`
- [ ] PR #56 Fix 1a deferred `UserCreatedThread` — `external_websocket_sync/src/thread_service.rs`
- [ ] PR #56 Fix 1b cfg-gated `return;` is FIRST statement of `BaseView::Uninitialized` — `agent_panel.rs` (record new line number)
- [ ] PR #57 Phase 16 counter exclusion — `helix-ws-test-server/main.go`
- [ ] PR #60 retry loop — `thread_service.rs::handle_follow_up_message`
- [ ] PR #63 wedge recovery surface — `thread_service.rs`
- [ ] PR #64 `agent_ready` re-emit — `thread_service.rs`
- [ ] PR #65 `fail_turn` — `crates/acp_thread/src/connection.rs`
- [ ] PR #65 Error arm + `chat_response_error` emit — `thread_service.rs`
- [ ] PR #65 `SyncEvent::ChatResponseError` — `types.rs`
- [ ] PR #65 `TEST_WEBSOCKET_SERVICE_GUARD` — shared by crash + reconnect tests in `thread_service.rs`
- [ ] `fd26c1a113` `Dockerfile.ci` pulls `helix-org` — `e2e-test/Dockerfile.ci`

## Walk Rebase Checklist

- [ ] Step through every numbered item in `portingguide.md` §"Rebase Checklist" — record any fired items
- [ ] Items 9, 11, 12, 12a, 31, 31a, 37, 39, 39a, 40, 41, 41a confirmed (Fix 1b first-statement, ConversationView field set, Stopped invariant, etc.)
- [ ] Assess whether any new rebase-checklist entries are warranted by this merge

## Build & Test (hard gate)

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — 0 errors (warnings acceptable if upstream-only)
- [ ] No new `BaseView` / `ContextServerStatus` variant or trait-signature changes surface (build failure is the gate)
- [ ] Pre-flight: `(cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server && go mod tidy)` — commit tidy if changed
- [ ] Copy fresh binary into `e2e-test/zed-binary`: `cp /home/retro/work/helix/zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] Run E2E `zed-agent` only: `./run_docker_e2e.sh` — all phases green
- [ ] Run E2E for both agents: `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` (full rebuild — never `--no-build`) — both rounds green
- [ ] Phase 9 (PR #60 retry-loop gate) — explicit PASS confirmed
- [ ] Phase 15 (PR #55 emit gate) — explicit PASS confirmed
- [ ] Phase 16 (PR #56 Fix 1a + PR #57) — explicit PASS confirmed (0 spontaneous user_created_thread events)
- [ ] Phase 17 (Fix 1b draft-suppression gate) — PASS confirmed
- [ ] Unit tests (if local Rust toolchain): `cargo test -p external_websocket_sync` — PR #65 crash-regression test and reconnect test both pass with no deadlock on `TEST_WEBSOCKET_SERVICE_GUARD`

## Update `portingguide.md`

- [ ] New `## Merge 002153 (2026-06-22)` section added at top of merge-history list
- [ ] Window summary subsection written (fill actual commit count and upstream HEAD SHA)
- [ ] Conflict-resolution subsection written (or explicit "0 conflicts, auto-merge clean" note)
- [ ] PR #65 survival check subsection written
- [ ] Helix-surface auto-merge survival check subsection written (per-area confirmation)
- [ ] PR #60/#63/#64 survival check subsection written
- [ ] Cargo.toml / Cargo.lock notes written
- [ ] `### Pre-existing Breakage Repaired` subsection — only if any signature-drift / typed-error / new-variant fix fired; omit otherwise
- [ ] Commit-history table extended with this merge's commits
- [ ] Stale entries encountered along the way corrected or deleted
- [ ] Portingguide entry committed to feature branch

## Re-merge Fork Main (if needed)

- [ ] Re-fetch `origin/main` — check if any out-of-band commits landed during merge work
- [ ] If fork main advanced: merge `origin/main` into feature branch, re-run critical-fix check + E2E

## Finalise

- [ ] Push Zed feature branch `feature/002153-merge-latest-zed` to `origin`
- [ ] Write `pull_request_zed.md` in this task directory
- [ ] In `/home/retro/work/helix/`, create branch `feature/002153-merge-latest-zed`, bump `ZED_COMMIT` from `9546054e68e2b771ac63e55821a70654684ac651` to the new merge HEAD
- [ ] Push Helix branch `feature/002153-merge-latest-zed`
- [ ] Write `pull_request_helix.md` in this task directory
- [ ] No force-push to main
- [ ] No agent-initiated PRs (Helix UI handles)
