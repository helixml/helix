# Implementation Tasks: Merge Latest Zed Upstream Into Helix Fork (002223)

## Setup & Baseline Re-confirmation

- [ ] Read `/home/retro/work/zed/portingguide.md` in full (canonical, 1109 lines; newest entry `## Merge 002100-extension` at line 670; Rebase Checklist at line 488)
- [ ] Read `002153_merge-latest-zed/` (requirements.md, design.md, tasks.md) end-to-end — it is the **unexecuted playbook for this exact baseline**
- [ ] Skim `002100_`, `002077_`, `002029_` merge specs for conflict-pattern context
- [ ] Read PR #65 in full: `git show 9546054e68` (Error arm + `ChatResponseError` + `TEST_WEBSOCKET_SERVICE_GUARD` surface)
- [ ] **Re-verify 002153 never landed**: no `feature/002153-...` on `origin`, porting guide newest is `002100-extension`, fork HEAD still `9546054e68`. If any of these changed, re-measure the merge base before continuing
- [ ] Add upstream remote (if missing): `git remote add upstream https://github.com/zed-industries/zed.git`
- [ ] `git fetch upstream && git fetch origin`
- [ ] Measure divergence: `git log --oneline e45e42af6e..upstream/main | wc -l`; record upstream HEAD SHA
- [ ] Confirm fork HEAD (`git log --oneline origin/main -1`) and `ZED_COMMIT` both at `9546054e68...`
- [ ] `git checkout main && git pull origin main`; create branch `feature/002223-merge-latest-zed`
- [ ] Decide whether to split into rounds if the commit count is large (002100-style)

## Pre-Merge Reconnaissance

- [ ] Inspect `git diff e45e42af6e..upstream/main -- crates/acp_thread/src/connection.rs` — any `AgentConnection`/`StubAgentConnection` change that could conflict with PR #65's `fail_turn()`?
- [ ] Gauge churn: `git diff e45e42af6e..upstream/main -- crates/agent_ui/src/agent_panel.rs | wc -l` (Fix 1b position risk)
- [ ] Scan the upstream diff for new `BaseView` / `ContextServerStatus` variants and any Rust edition / workspace-wide changes

## Merge Execution

- [ ] `git merge upstream/main` — note conflicts
- [ ] Start the `## Merge 002223 (2026-07-06)` entry in `portingguide.md` now; update it as each conflict is resolved
- [ ] Resolve each conflict one at a time; `Cargo.lock` → `--theirs`; `.github/workflows/` → `--theirs`
- [ ] Keep all Helix code behind `#[cfg(feature = "external_websocket_sync")]`; prefer upstream ordering for shared lists/match arms
- [ ] `git diff --check` clean (no conflict markers); commit the merge; record merge SHA

## Auto-Merge Survival Sweep (verify even when git says "auto-merged")

- [ ] PR #65: `fail_turn` in `connection.rs`; Error arm + `chat_response_error` in `thread_service.rs`; `SyncEvent::ChatResponseError` in `types.rs`; `TEST_WEBSOCKET_SERVICE_GUARD` present
- [ ] Fix 1b cfg-gated `return;` is the FIRST statement of `BaseView::Uninitialized` in `ensure_thread_initialized` (read the full function; record new line)
- [ ] Fix #11 entity-identity guard via `ThreadMetadataStore` at top of `load_agent_thread` — `agent_panel.rs`
- [ ] Fixes #3/#6/#8/#9: `content_only`, `stopped_emitted_for_task` (all sites), `drop(turn.send_task)` — `acp_thread.rs`
- [ ] Fix #1 `pending_sessions` shared-task; `wait_for_tools_ready` uses `background_executor().timer()` (no `smol::Timer`) — `agent/src/agent.rs`
- [ ] Fix #7 `unregister_thread` — `conversation_view.rs`; `from_existing_thread()` field-set (build-gated)
- [ ] PR #50 `session_creation_chain` + `_settings_subscription` coexist — `agent_servers/src/acp.rs`
- [ ] PR #55 `EntryUpdated` emit (~16 occurrences) — `acp_thread.rs`
- [ ] PR #56 Fix 1a deferred `UserCreatedThread`; PR #60 `ede_diagnostic` retry; PR #63 `force_reset_session`/`clear_keep_alive`; PR #64 `agent_ready` re-emit — `thread_service.rs`
- [ ] PR #57 Phase 16 counter exclusion — `helix-ws-test-server/main.go`
- [ ] 3× `// HELIX: External agent` markers — `extensions_ui.rs` (record new lines)
- [ ] `AcpBetaFeatureFlag::enabled_for_all` — `flags.rs`; `render_restricted_mode` early return + `optional = true` dep — `title_bar`
- [ ] `--allow-multiple-instances`/`--headless`/`build_application` — `main.rs`; `debug-embed` + workspace members — `Cargo.toml`; `CollaboratorId::Agent` — `workspace.rs`
- [ ] Helix `settings_content.rs` fields intact; `fd26c1a113` `helix-org` pull — `Dockerfile.ci`
- [ ] `BaseView` / `ContextServerStatus` matches updated for any new upstream variants
- [ ] `AcpThreadEvent::Stopped\b([^(]|$)` — 0 hits across `crates/acp_thread/src/`

## Walk Rebase Checklist

- [ ] Step through every numbered item in `portingguide.md` §"Rebase Checklist"; record any that fired
- [ ] Assess whether new rebase-checklist entries are warranted by this merge

## Build & Test (hard gate)

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` — zero errors (upstream-only warnings acceptable)
- [ ] (best-effort, if local Rust) `cargo test -p external_websocket_sync`; `cargo test -p acp_thread test_second_send`; `cargo test -p agent_servers test_concurrent_session_creation_is_serialized`
- [ ] Copy fresh binary: `cp /home/retro/work/helix/zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] `(cd crates/external_websocket_sync/e2e-test/helix-ws-test-server && go mod tidy)`; commit tidy if changed
- [ ] Run E2E zed-agent: `./run_docker_e2e.sh` — all 17 phases green
- [ ] Run E2E both agents: `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` (full rebuild, never `--no-build`) — both green
- [ ] Confirm Phase 8, **Phase 9**, Phase 13, **Phase 15**, **Phase 16**, **Phase 17** explicit PASS
- [ ] **Do not proceed to finalise if the external WebSocket sync E2E tests are failing**

## Update `portingguide.md`

- [ ] `## Merge 002223 (2026-07-06)` section at top of merge-history list
- [ ] Window summary (fill commit count + upstream HEAD SHA; note the 002153 delta folded in)
- [ ] Conflict-resolution subsection (or explicit "0 conflicts, auto-merge clean")
- [ ] PR #65 survival check; Helix-surface auto-merge survival check; PR #60/#63/#64 survival check
- [ ] Cargo.toml / Cargo.lock notes
- [ ] `### Pre-existing Breakage Repaired` subsection only if a signature/typed-error/new-variant fix fired
- [ ] Commit-history table extended; stale entries corrected/deleted; commit the guide to the feature branch

## Re-merge Fork Main (if needed)

- [ ] Re-fetch `origin/main`; if it advanced during merge work, merge into the feature branch and re-run critical-fix check + E2E

## Finalise

- [ ] Push Zed branch `feature/002223-merge-latest-zed` to `origin`
- [ ] Write `pull_request_zed.md` in this task directory (summary of significant upstream changes + porting decisions)
- [ ] In `/home/retro/work/helix/`, branch `feature/002223-merge-latest-zed`; bump `ZED_COMMIT` from `9546054e68e2b771ac63e55821a70654684ac651` to the new merge HEAD; push
- [ ] Write `pull_request_helix.md` in this task directory
- [ ] No force-push to main; no agent-initiated PRs (Helix UI handles PR creation)
