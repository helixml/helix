# Implementation Tasks

## Setup

- [x] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, more detailed than this spec
- [x] Read prior plans for context: `001947_merge-latest-zed/` and `001946_merge-latest-zed/` (planned but never executed; same fork state, useful precedent), and `001909_merge-latest-zed/` (the last merge that actually shipped)
- [x] Verify upstream remote: `cd /home/retro/work/zed && git remote -v`. The `upstream` remote is currently **missing** — add it: `git remote add upstream https://github.com/zed-industries/zed.git`
- [x] `git fetch upstream`
- [x] Record divergence: 172 commits to merge, fork ahead 203, upstream HEAD `1da60a8518` — written into `portingguide.md`
- [x] Create feature branch: `feature/001980-merge-latest-zed` from `f5fab97857`

## Merge Execution

- [x] `git merge upstream/main` — 4 conflicts: `deploy_cloudflare.yml`, `Cargo.lock`, `agent_settings.rs`, `wgpu_renderer.rs`
- [x] Conflict triage done — see `portingguide.md` §"Merge 001980" for per-file resolutions
- [x] `.github/workflows/deploy_cloudflare.yml` — accept upstream deletion (`git rm`)
- [x] `Cargo.lock` — `git checkout --theirs`
- [x] Manual three-way merges:
  - `crates/agent_settings/src/agent_settings.rs` — kept Helix `show_onboarding`/`auto_open_panel`, dropped `new_thread_location` (upstream removed in #55575)
  - `crates/gpui_wgpu/src/wgpu_renderer.rs` — accept upstream comment addition (no Helix concern)
- [x] Porting guide updated live with all 4 resolutions
- [x] No conflict markers remain (`grep -rn "<<<<<<<\|>>>>>>>"` clean)
- [x] Merge committed: `c3e312b056`

## Sweep for Silent Drift (auto-merged files)

- [x] `grep -rn "ActiveView" crates/agent_ui/src/` — clean
- [x] `grep -rn "set_active_view" crates/agent_ui/src/` — clean
- [x] `grep -rn "draft_threads\|background_threads" crates/agent_ui/src/` — clean
- [x] `grep -n "selected_agent_type" crates/agent_ui/src/` — clean
- [x] `grep -n "wait_for_tools_ready" crates/agent/src/agent.rs` — present at line 1722
- [x] `grep -n "smol::Timer" crates/agent/src/agent.rs` — clean
- [x] `grep -n "allow_multiple_instances" crates/zed/src/main.rs` — present at lines 350, 1778
- [x] `grep -n "debug-embed" Cargo.toml` — present at line 704

## Verify Critical Fixes (the 9 in `portingguide.md` §"Critical Fixes")

- [x] Fix #1: `load_session` clones `NativeAgent` entity before async task (`crates/agent/src/agent.rs`)
- [x] Fix #2: no `MessageAdded`/`MessageCompleted` sends from `crates/agent_ui/src/acp/thread_view.rs`
- [x] Fix #3: `content_only` present at `acp_thread.rs:144`
- [x] Fix #4: `notify_thread_display` callable in `external_websocket_sync/src/thread_service.rs`
- [x] Fix #5: `flush_stale_pending_for_thread` present at `thread_service.rs:203`
- [x] Fix #6: test pattern repaired (`AcpThreadEvent::Stopped(_)` — was broken since Stopped became a tuple variant); `cargo test` deferred to E2E gate (no local Rust toolchain)
- [x] Fix #7: `unregister_thread` called in `conversation_view.rs:811, 816`
- [x] Fix #8: `drop(turn.send_task)` present at `acp_thread.rs:2480`
- [x] Fix #9: `stopped_emitted_for_task` guards both completion paths (`acp_thread.rs:2325, 2429`)

## Verify Helix Surface (per `requirements.md` acceptance criteria)

- [x] `crates/external_websocket_sync/` crate intact (no merge edits)
- [x] WebSocket thread display callback present in `agent_panel.rs::new()`
- [x] UI state query callback present in `agent_panel.rs::new()`
- [x] `acp_history_store()` accessor still on `AgentPanel`
- [x] `from_existing_thread()` constructor still on `ConversationView`; `ConnectedServerState` fields stable (6 fields, no upstream additions)
- [x] Channel-based UI event forwarding (`tokio::sync::mpsc`) still in place
- [x] `OnboardingUpsell::set_dismissed` Helix-mode cleanup path still wired
- [x] `AcpBetaFeatureFlag::enabled_for_all() -> true` override still applied (`feature_flags/src/flags.rs:30`)
- [x] Built-in agent hiding still gated on `external_websocket_sync`
- [x] Enterprise TLS skip still in `sync_settings`
- [x] Feature propagation chain intact (`zed → agent_ui → title_bar`, all `optional = true`)

## Verify PRs #44–#47 (recently added Helix behaviour)

- [x] PR #44–#47 commits all carried through merge (verified via `git log --oneline f5fab97857..HEAD`)

## Walk Rebase Checklist

- [x] Walked all 41 items in `portingguide.md` §"Rebase Checklist"
- [x] `ConnectedServerState`: 6 fields, unchanged
- [x] `AgentConnection` trait: no new methods needing Helix work
- [x] `AcpThreadEvent::Stopped(StopReason)` still a tuple variant — added new checklist item 41a covering test-code drift
- [x] Anthropic model list order: matches upstream (no conflict in this merge)
- [x] Default settings (`show_onboarding`, `trust_all_worktrees`, `show_sign_in`) intact

## Build & Test (hard gate)

- [x] `./stack build-zed dev` succeeds (6m 35s, 0 errors, only warnings) — `./zed-build/zed` produced (512 MB)
- [x] Build implicitly covers `cargo check -p zed --features external_websocket_sync` (it's the same compile)
- [-] `cargo check -p zed` (no features): no local Rust toolchain — covered by CI gate
- [-] `cargo test -p external_websocket_sync`: no local Rust toolchain — defer to CI / E2E gate
- [x] `cargo test -p acp_thread test_second_send`: source test pattern repaired (`Stopped(_)`); execution deferred to CI
- [x] Copy fresh binary + run E2E `zed-agent` — **ALL 12 PHASES PASSED** (incl. Phase 1, 2, 3, 4, 8, 9 named in requirements.md). Build duration ~2.5 min.
- [~] E2E `claude` (running)
- [ ] If any phase fails: diagnose, fix, re-run

### E2E side notes (zed-agent run)

- Pre-flight required `go mod tidy` in `e2e-test/helix-ws-test-server/` (helix Go deps had drifted: `kodit v1.3.6 → v1.3.7`, dropped `go-tika`). The runner script doesn't tidy itself. Committed the tidied go.mod/go.sum in same branch.
- Phase 12 logs a benign `WebSocket protocol error: Connection reset without closing handshake` from the deliberate Zed restart; it reconnects cleanly and Phase 12 PASSED.
- Phase 1 took 15.1s for `wait_for_tools_ready` (`slow-mcp-test` MCP server) — confirms the `cx.background_executor().timer()` fix from 001909 still works end-to-end.

## Update `portingguide.md` (incremental, not at the end)

- [ ] Each conflict resolution appended live with upstream change / resolution / why
- [ ] Append commit history table with this merge's commits (merge commit + any follow-up fixes)
- [ ] Append any new rebase-checklist items uncovered during this merge
- [ ] Note any stale guide entries discovered (e.g. dead-code `HeadlessConnection` references) and either delete or correct them
- [ ] Note any Helix patches absorbed by upstream that can now be retired (with explicit justification)

## Re-merge Fork Main (only if needed)

- [ ] If anyone pushed to fork main during this work, `git merge origin/main` into the feature branch (Cargo.lock conflicts → `--theirs`)
- [ ] Rebuild + re-run E2E

## Finalise

- [ ] Push `feature/001980-merge-latest-zed` to `helixml/zed`
- [ ] Write `pull_request_zed.md` and `pull_request_helix.md` in this task directory (PR title + body for each)
- [ ] Open Helix repo PR **first** (bump `ZED_COMMIT` in `sandbox-versions.txt`) — per `CLAUDE.md` ordering rule
- [ ] Open Zed PR against fork main with the merge commit
- [ ] Do **not** force-push `main` without explicit user approval
