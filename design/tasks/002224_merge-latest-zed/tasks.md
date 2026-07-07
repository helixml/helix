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
- [ ] Start the `## Merge 002224 (2026-07-06)` porting-guide entry BEFORE resolving conflicts
- [x] `git merge upstream/main` — note all conflicts
- [x] Resolve each conflict immediately; document each in the porting guide as resolved
- [x] `Cargo.lock`: `git checkout --theirs Cargo.lock`
- [x] `.github/workflows/` conflicts: `git checkout --theirs` (Helix doesn't use Zed CI) — but do NOT touch `.drone.yml --locked`
- [~] If ACP bumped: convert `non_exhaustive` ACP struct literals to builders; re-check `ErrorCode` arms; fix `AgentConnection`/`StubAgentConnection` impls
- [x] `git diff --check` — no conflict markers remain
- [x] Commit the merge; record merge SHA

## Auto-Merge Inspection (verify even when git says "auto-merged")
- [ ] `agent_panel.rs` — read full `ensure_thread_initialized`; Fix 1b cfg-gated `return;` is FIRST statement of `BaseView::Uninitialized`; ThreadDisplayNotification handler still calls `OnboardingUpsell::set_dismissed(true, cx)` and inits `NativeAgentSessionList`; Critical Fix #11 guard present; record new line numbers
- [ ] `zed.rs` — `initialize_agent_panel` + WebSocket init intact
- [ ] `thread_view.rs` — `from_existing_thread`, channel-based forwarding, no duplicate WS sends
- [ ] `connection.rs` — `fail_turn()` on `StubAgentConnection` intact
- [ ] `thread_service.rs` — Error arm + `chat_response_error` emit; PR #60/#63/#64 surface; windowless `cx.subscribe()` incremental streaming; `TEST_WEBSOCKET_SERVICE_GUARD`
- [ ] `types.rs` — `SyncEvent::ChatResponseError` intact
- [ ] `extensions_ui.rs` — 3× `// HELIX: External agent` markers (record new lines)
- [ ] `assets/settings/default.json` — `trust_all_worktrees: true`, `show_sign_in: false`, branding/onboarding
- [ ] Built-in agent hiding (Claude Code/Codex/Gemini) wrapped in `cfg(not(feature = "external_websocket_sync"))`

## Sweep for Silent Drift
- [ ] `smol::Timer` in `agent.rs` — 0 hits (`wait_for_tools_ready` uses `cx.background_executor().timer()`)
- [ ] `--allow-multiple-instances` / `--headless` / `build_application` / `initialize_headless` in `main.rs` — intact
- [ ] `debug-embed` feature on `rust-embed` — intact
- [ ] `session_creation_chain` + `_settings_subscription` — both in `agent_servers/src/acp.rs`
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` — `feature_flags/src/flags.rs`
- [ ] `render_restricted_mode` cfg-gated early return — `title_bar` (record new line)
- [ ] `AcpThreadEvent::Stopped` non-tuple patterns — 0 hits (`grep -nE "AcpThreadEvent::Stopped\b([^(]|$)"`)
- [ ] `helix-org` pull in `Dockerfile.ci`; `cargo build --locked` in `.drone.yml`

## Verify Critical Fixes (portingguide.md "Critical Fixes")
- [ ] #1 `pending_sessions` / `load_session` — `agent/src/agent.rs`
- [ ] #2 no duplicate WS sends — `thread_view.rs`
- [ ] #3 `content_only` — `acp_thread.rs`
- [ ] #4 `notify_thread_display` — `thread_service.rs`
- [ ] #5 flush stale pending — `thread_service.rs`
- [ ] #6 `stopped_emitted_for_task` guard — `acp_thread.rs`
- [ ] #7 `unregister_thread` on entity replacement
- [ ] #8 `drop(turn.send_task)` — `acp_thread.rs`
- [ ] #9 normal-completion `Stopped` guard — `acp_thread.rs`
- [ ] #11 entity-identity guard via `ThreadMetadataStore` — `agent_panel.rs`

## Verify Helix PR Surface (#50, #55, #56, #57, #60, #63, #64, #65 + fd26c1a113)
- [ ] PR #50 `session_creation_chain` + `_settings_subscription` coexist; `test_concurrent_session_creation_is_serialized` passes
- [ ] PR #55 `EntryUpdated` emit occurrences in `acp_thread.rs`
- [ ] PR #56 Fix 1a deferred `UserCreatedThread` in `external_websocket_sync`
- [ ] PR #56 Fix 1b cfg-gated `return;` FIRST statement of `BaseView::Uninitialized`
- [ ] PR #57 Phase-16 counter exclusion — `helix-ws-test-server/main.go`
- [ ] PR #60 `ede_diagnostic` retry loop — `thread_service.rs`
- [ ] PR #63 wedge recovery (`force_reset_session`, `clear_keep_alive`, agent_name) — `thread_service.rs`
- [ ] PR #64 `agent_ready` re-emit — `thread_service.rs`
- [ ] PR #65 `fail_turn` / Error arm / `ChatResponseError` / shared `TEST_WEBSOCKET_SERVICE_GUARD`
- [ ] `fd26c1a113` `Dockerfile.ci` `helix-org` pull

## Walk the Rebase Checklist
- [ ] Step through every numbered item in `portingguide.md` "Rebase Checklist"; record any fired
- [ ] Special: items 9, 11, 12/12a, 31/31a/37, 39/39a, 40, 41/41a + PR #65 `fail_turn`
- [ ] Assess whether any new rebase-checklist entry is warranted by this merge

## Build & Test (hard gate)
- [ ] `cargo check -p zed` (no feature) — 0 errors
- [ ] `cargo check -p zed --features external_websocket_sync` — 0 errors
- [ ] Zed builds clean via canonical builder (`cd /home/retro/work/helix && ./stack build-zed dev`) — 0 errors
- [ ] Unit tests: `cargo test -p external_websocket_sync` (PR #65 crash + reconnect, no deadlock on shared guard); `-p acp_thread test_second_send`; `-p agent_servers test_concurrent_session_creation_is_serialized`
- [ ] Pre-flight: `(cd .../e2e-test/helix-ws-test-server && go mod tidy)`; commit if changed
- [ ] Copy fresh binary: `cp /home/retro/work/helix/zed-build/zed .../e2e-test/zed-binary`
- [ ] E2E `zed-agent` only: `./run_docker_e2e.sh` — all phases green
- [ ] E2E both agents: `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` (full rebuild; never `--no-build`) — green
- [ ] Confirm the task's core phases explicitly: Phase 1 (new thread), Phase 2 (follow-up entry_count++), Phase 3 (second thread + switch), Phase 4 (message to non-visible Thread A)
- [ ] Confirm gate phases: 8 (interrupt), 9 (PR #60 retry), 15 (PR #55 streaming), 16 (PR #56 1a + #57), 17 (Fix 1b)
- [ ] Confirm UI state queries: correct `thread_id`, `entry_count`, `active_view`
- [ ] One retry allowed for Claude Phase-1 npm flake and Phase-9 API-latency flake

## Update portingguide.md
- [ ] `## Merge 002224 (2026-07-06)` section at top of merge history
- [ ] Window summary (actual commit count + upstream HEAD SHA + fence)
- [ ] Conflicts-and-resolutions subsection (or explicit "0 conflicts, auto-merge clean")
- [ ] PR #65 survival check; Helix-surface survival check; PR #60/#63/#64 survival check
- [ ] Cargo.toml / Cargo.lock notes (incl. ACP bump if any)
- [ ] `### Pre-existing Breakage Repaired` — only if a fix actually fired
- [ ] Commit-history table extended; stale entries corrected/deleted
- [ ] Porting-guide changes committed to the feature branch

## Re-merge Fork Main (if needed)
- [ ] Re-fetch the fork remote — check for out-of-band commits landed during merge work
- [ ] If it advanced: merge into the feature branch; re-run critical-fix check + E2E

## Finalise
- [ ] Push Zed branch `feature/002224-merge-latest-zed` to the fork; confirm `helixml/zed` CI green
- [ ] Write `pull_request_zed.md` in this task directory
- [ ] In helix repo: branch `feature/002224-merge-latest-zed`, bump `ZED_COMMIT` to the new merge HEAD; push
- [ ] Write `pull_request_helix.md` in this task directory
- [ ] No force-push to `main`/`helix-fork` without explicit user approval
- [ ] No agent-initiated PRs (Helix UI handles PR creation)
