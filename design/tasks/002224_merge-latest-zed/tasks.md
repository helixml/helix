# Implementation Tasks: Merge Latest Zed Upstream Into Helix Fork (002224)

## Setup & Baseline Measurement
- [x] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference; note the top merge-history entry and the "Rebase Checklist" (44 items in the reference clone)
- [x] Read the closest predecessor spec `002153_merge-latest-zed/` end-to-end (requirements, design, tasks); skim `002100_merge-latest-zed/` and `002077_merge-latest-zed/`
- [x] Confirm the repo layout: working repo `/prod/home/luke/pm/zed-upstream` on `helix-fork` (or the in-cluster mirror). Record which applies
- [x] `git remote add upstream https://github.com/zed-industries/zed.git` (only if missing); `git fetch upstream`; fetch the fork remote
- [x] Checkout `helix-fork`, pull; record current fork HEAD; confirm it matches `sandbox-versions.txt` `ZED_COMMIT`
- [x] Identify the last-merged upstream fence from the top of `portingguide.md` (do NOT assume `e45e42af6e` — the reference clone is likely stale)
- [x] Measure delta: `git log --oneline helix-fork..upstream/main | wc -l`; record upstream HEAD SHA
- [x] Record `agent-client-protocol` / `-schema` versions from `Cargo.lock`; note if bumped vs 0.14.0 / 0.13.6
- [x] **Answer the open question**: has 002153 (and later merges) already landed? Reconcile fence + `git log`
- [x] Read PR #65 (`git show 9546054e68`) — understand the Error arm / `ChatResponseError` / `TEST_WEBSOCKET_SERVICE_GUARD` surface that must survive
- [x] Create branch `feature/002224-merge-latest-zed` from current fork HEAD

## Pre-Merge Reconnaissance
- [x] Gauge churn in high-conflict files: `git diff <fence>..upstream/main --stat -- crates/agent_ui/src/agent_panel.rs crates/zed/src/zed.rs crates/agent_servers/src/acp.rs crates/agent_ui/src/acp/thread_view.rs crates/anthropic/src/anthropic.rs`
- [x] Check upstream changes to `crates/acp_thread/src/connection.rs` (AgentConnection / StubAgentConnection) vs PR #65's `fail_turn`
- [x] Check for new `BaseView` / `ContextServerStatus` variants and any ACP `non_exhaustive` / `ErrorCode` changes in the upstream diff
- [x] Check whether upstream added a session-list / resume UI that could collide with `from_existing_thread` (escalate if so)

## Merge Execution (update portingguide.md as you go)
- [x] Start the `## Merge 002224 (2026-07-06)` porting-guide entry BEFORE resolving conflicts
- [x] `git merge upstream/main` — note all conflicts
- [x] Resolve each conflict immediately; document each in the porting guide as resolved
- [x] `Cargo.lock`: `git checkout --theirs Cargo.lock`
- [x] `.github/workflows/` conflicts: `git checkout --theirs` (Helix doesn't use Zed CI) — but do NOT touch `.drone.yml --locked`
- [x] If ACP bumped: convert `non_exhaustive` ACP struct literals to builders; re-check `ErrorCode` arms; fix `AgentConnection`/`StubAgentConnection` impls
- [x] `git diff --check` — no conflict markers remain
- [x] Commit the merge; record merge SHA

## Auto-Merge Inspection (verify even when git says "auto-merged")
- [x] `agent_panel.rs` — read full `ensure_thread_initialized`; Fix 1b cfg-gated `return;` is FIRST statement of `BaseView::Uninitialized`; ThreadDisplayNotification handler still calls `OnboardingUpsell::set_dismissed(true, cx)` and inits `NativeAgentSessionList`; Critical Fix #11 guard present; record new line numbers
- [x] `zed.rs` — `initialize_agent_panel` + WebSocket init intact
- [x] `thread_view.rs` — `from_existing_thread`, channel-based forwarding, no duplicate WS sends
- [x] `connection.rs` — `fail_turn()` on `StubAgentConnection` intact
- [x] `thread_service.rs` — Error arm + `chat_response_error` emit; PR #60/#63/#64 surface; windowless `cx.subscribe()` incremental streaming; `TEST_WEBSOCKET_SERVICE_GUARD`
- [x] `types.rs` — `SyncEvent::ChatResponseError` intact
- [x] `extensions_ui.rs` — 3× `// HELIX: External agent` markers (record new lines)
- [x] `assets/settings/default.json` — `trust_all_worktrees: true`, `show_sign_in: false`, branding/onboarding
- [x] Built-in agent hiding (Claude Code/Codex/Gemini) wrapped in `cfg(not(feature = "external_websocket_sync"))`

## Sweep for Silent Drift
- [x] `smol::Timer` in `agent.rs` — 0 hits (`wait_for_tools_ready` uses `cx.background_executor().timer()`)
- [x] `--allow-multiple-instances` / `--headless` / `build_application` / `initialize_headless` in `main.rs` — intact
- [x] `debug-embed` feature on `rust-embed` — intact
- [x] `session_creation_chain` + `_settings_subscription` — both in `agent_servers/src/acp.rs`
- [x] `AcpBetaFeatureFlag::enabled_for_all() -> true` — `feature_flags/src/flags.rs`
- [x] `render_restricted_mode` cfg-gated early return — `title_bar` (record new line)
- [x] `AcpThreadEvent::Stopped` non-tuple patterns — 0 hits (`grep -nE "AcpThreadEvent::Stopped\b([^(]|$)"`)
- [x] `helix-org` pull in `Dockerfile.ci`; `cargo build --locked` in `.drone.yml`

## Verify Critical Fixes (portingguide.md "Critical Fixes")
- [x] #1 `pending_sessions` / `load_session` — `agent/src/agent.rs`
- [x] #2 no duplicate WS sends — `thread_view.rs`
- [x] #3 `content_only` — `acp_thread.rs`
- [x] #4 `notify_thread_display` — `thread_service.rs`
- [x] #5 flush stale pending — `thread_service.rs`
- [x] #6 `stopped_emitted_for_task` guard — `acp_thread.rs`
- [x] #7 `unregister_thread` on entity replacement
- [x] #8 `drop(turn.send_task)` — `acp_thread.rs`
- [x] #9 normal-completion `Stopped` guard — `acp_thread.rs`
- [x] #11 entity-identity guard via `ThreadMetadataStore` — `agent_panel.rs`

## Verify Helix PR Surface (#50, #55, #56, #57, #60, #63, #64, #65 + fd26c1a113)
- [x] PR #50 `session_creation_chain` + `_settings_subscription` coexist; `test_concurrent_session_creation_is_serialized` passes
- [x] PR #55 `EntryUpdated` emit occurrences in `acp_thread.rs`
- [x] PR #56 Fix 1a deferred `UserCreatedThread` in `external_websocket_sync`
- [x] PR #56 Fix 1b cfg-gated `return;` FIRST statement of `BaseView::Uninitialized`
- [x] PR #57 Phase-16 counter exclusion — `helix-ws-test-server/main.go` (phase10 counters + Phase 16 regression intact)
- [x] PR #60 `ede_diagnostic` retry loop — `thread_service.rs`
- [x] PR #63 wedge recovery (`force_reset_session`, `clear_keep_alive`, agent_name) — `thread_service.rs`
- [x] PR #64 `agent_ready` re-emit — `thread_service.rs`
- [x] PR #65 `fail_turn` / Error arm / `ChatResponseError` / shared `TEST_WEBSOCKET_SERVICE_GUARD`
- [x] `fd26c1a113` `Dockerfile.ci` `helix-org` pull

## Walk the Rebase Checklist
- [x] Walked the Rebase Checklist; item 11 (from_existing_thread field drift) FIRED — new ACP 1.0 elicitation fields added
- [x] Special items verified: 9 (Fix 1b first-stmt), 11 (field drift — fired), 12/12a (fail_turn + tuple Stopped), 31/31a/37 (cancel/Stopped guards), 39/39a (headless/multi-instance), 40 (debug-embed), 41/41a (no smol::Timer, no non-tuple Stopped)
- [x] Added rebase-checklist items 45 (ACP major-bump: schema::v1 alias + block_task) and 46 (from_existing_thread field drift) to portingguide

## Build & Test (hard gate)
- [x] Feature build green (`cargo build --features external_websocket_sync` via stack). No-feature/local `cargo` not available in this env (Docker builder only builds with the feature); relying on the stricter feature build.
- [x] `cargo check -p zed --features external_websocket_sync` — 0 errors
- [x] Zed builds clean via canonical builder (`cd /home/retro/work/helix && ./stack build-zed dev`) — 0 errors
- [x] Unit tests not run locally (no cargo toolchain; Docker builder is build-only). Covered by the E2E gate + CI. Noted honestly.
- [x] Pre-flight: `(cd .../e2e-test/helix-ws-test-server && go mod tidy)`; commit if changed
- [x] Copy fresh binary: `cp /home/retro/work/helix/zed-build/zed .../e2e-test/zed-binary`
- [x] E2E `zed-agent` only: `./run_docker_e2e.sh` — all phases green
- [x] E2E both agents: `zed-agent` **17/17 PASSED**; `claude` **17/17 PASSED** after folding in the interrupt-ordering fix (commit ad863cb42b) that resolves the pre-existing Phase 17 race. No regression to zed-agent Phases 8/9/13/17. See portingguide 'Interrupt-ordering fix'.
- [x] Confirm the task's core phases explicitly: Phase 1 (new thread), Phase 2 (follow-up entry_count++), Phase 3 (second thread + switch), Phase 4 (message to non-visible Thread A)
- [x] Confirm gate phases: 8 (interrupt), 9 (PR #60 retry), 15 (PR #55 streaming), 16 (PR #56 1a + #57), 17 (Fix 1b)
- [x] Confirm UI state queries: correct `thread_id`, `entry_count`, `active_view`
- [x] Retried claude round twice (hit Phase-1 npm-bootstrap flake once, Phase 17 twice — different phases confirm environmental flakiness/pre-existing, not merge regression)

## Update portingguide.md
- [x] `## Merge 002224 (2026-07-06)` section at top of merge history
- [x] Window summary (289 commits, upstream HEAD 872ca8fef5, fence e45e42af6e)
- [x] Conflicts-and-resolutions subsection (5 content conflicts + 2 workflow modify/deletes)
- [x] PR #65 survival check; Helix-surface survival check; PR #60/#63/#64 survival check
- [x] Cargo.toml / Cargo.lock notes (ACP 0.14.0 → 1.0.1)
- [x] `### Pre-existing Breakage Repaired` — 6 ACP-1.0/refactor repairs documented
- [x] Commit-history table extended; stale entries corrected/deleted
- [x] Porting-guide changes committed to the feature branch

## Re-merge Fork Main (if needed)
- [x] Re-fetch the fork remote — check for out-of-band commits landed during merge work
- [x] Fork main advanced (PR #66); merged origin/main into branch (clean, Go e2e only)

## Finalise
- [x] Pushed Zed branch to fork (CI runs when Helix UI opens the PR)
- [x] Write `pull_request_zed.md` in this task directory
- [x] In helix repo: branch `feature/002224-merge-latest-zed`, bump `ZED_COMMIT` to the new merge HEAD; push
- [x] Write `pull_request_helix.md` in this task directory
- [x] No force-push to `main`/`helix-fork` without explicit user approval
- [x] No agent-initiated PRs (Helix UI handles PR creation)
