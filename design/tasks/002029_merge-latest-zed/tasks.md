# Implementation Tasks: Merge Latest Zed Upstream (002029)

## Setup

- [ ] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, 762 lines as of start of task
- [ ] Read prior plan `001996_merge-latest-zed/` end-to-end — closest precedent (mandatory, not optional)
- [ ] Skim 001980 plan for the `BaseView::Terminal` exhaustiveness lesson and the GPUI-event-flush race pattern
- [ ] Read commit messages of Helix PRs since 001996: `git log 8841edb2b1..origin/main` — PR #50 (`b16e4a948a`), PR #55 (`3778eb04a3`), PR #56 (`32a1e3ba30`, `455c095fcc`, `056fe07180`, `769a463a2f`), PR #57 (`b35224530f`)
- [ ] Verify upstream remote: `cd /home/retro/work/zed && git remote -v`. If missing, add: `git remote add upstream https://github.com/zed-industries/zed.git`
- [ ] `git fetch upstream && git fetch origin`
- [ ] Verify divergence: 158 commits to merge, fork HEAD `b2f2ebefb6`, upstream HEAD `f2df3f9e18` (re-confirm at runtime — numbers may shift if upstream pushed since)
- [ ] Pull `origin/main` first in case fork main moved
- [ ] Create feature branch: `feature/002029-merge-latest-zed` from current fork main

## Pre-Merge Reconnaissance

- [ ] Read upstream commit `bbe23cc40b` "Bring back draft threads" (#54292) in full — this is the dominant risk; identify whether `ensure_thread_initialized()` is renamed/restructured, and if so where the equivalent entry point lives. Helix PR #56 Fix 1b's `#[cfg(feature = "external_websocket_sync")] { return; }` guard MUST land at the equivalent location.
- [ ] Read upstream commit `23231879cd` "ACP session deletion" (#57004) in full — overlaps Helix PR #50's `session_creation_chain` in `crates/agent_servers/src/acp.rs`. Identify the merge-three-way shape needed.
- [ ] Read upstream commit `f2df3f9e18` "ACP logout" (#56959) — confirm the new `supports_logout`/`logout` defaults won't surface a logout UI in Helix mode.
- [ ] Skim upstream commits touching `acp_thread.rs`: `git log 8bdd78e023..upstream/main -- crates/acp_thread/` — check whether anything overlaps PR #55's new `EntryUpdated` emit site.

## Merge Execution

- [ ] `git merge upstream/main`
- [ ] Triage conflicts; for each, append to `portingguide.md` §"Merge 002029" with `(upstream change / resolution / why / risk)` BEFORE moving to the next one
- [ ] `Cargo.lock` (if conflicting): `git checkout --theirs Cargo.lock`
- [ ] Any `.github/workflows/` conflicts: accept upstream
- [ ] Resolve `crates/agent_ui/src/agent_panel.rs` conflicts — Critical Fix #11 (entity-identity guard at top of `load_agent_thread`) must survive verbatim; **Helix PR #56 Fix 1b draft suppression** must remain in `ensure_thread_initialized` (or its equivalent if renamed); thread display callback, UI state query, `acp_history_store()`, onboarding bypass, ACP auto-approve preserved
- [ ] Resolve `crates/agent_ui/src/conversation_view.rs` conflicts — `from_existing_thread()`, `THREAD_REGISTRY` registration, `is_resume`, history refresh, unregister-on-reset preserved
- [ ] Resolve `crates/acp_thread/src/acp_thread.rs` conflicts — Critical Fixes #6, #8, #9 (cancel/Stopped invariants) preserved; PR #55 streaming-reveal `EntryUpdated` emit preserved
- [ ] Resolve `crates/acp_thread/src/connection.rs` if conflict — accept upstream trait additions (`supports_logout`, `logout` defaults; `supports_delete` signature change)
- [ ] Resolve `crates/agent_servers/src/acp.rs` conflicts — fold PR #50 `session_creation_chain` with upstream `23231879cd` session-deletion plumbing
- [ ] Resolve `crates/agent/src/agent.rs` conflicts — Critical Fix #1 (entity-lifetime clone in `load_session`) preserved; `wait_for_tools_ready` uses `cx.background_executor().timer()`; update `supports_delete(&self)` → `supports_delete(&self, &App)` signature
- [ ] Resolve any other Helix-touched file conflicts (workspace.rs, main.rs, title_bar, Cargo.toml workspace deps)
- [ ] Compile-driven trait-signature migration: walk every `supports_delete()` call site (`crates/agent_ui/src/acp/thread_history.rs` and any other) and add `cx` parameter; every `fn supports_delete(&self)` impl gets `&App`
- [ ] No conflict markers remain: `grep -rn "<<<<<<<\|>>>>>>>" .` (excluding test-string markers in `git_store.rs`)
- [ ] Commit merge: `git commit` (let git auto-generate the merge commit; do not amend)

## Sweep for Silent Drift (auto-merged files)

- [ ] `grep -rn "ActiveView" crates/agent_ui/src/` — only `AgentPanelEvent::ActiveView*` valid
- [ ] `grep -rn "set_active_view" crates/agent_ui/src/` — clean
- [ ] `grep -rn "draft_threads\|background_threads" crates/agent_ui/src/` — clean (likely now `retained_threads` + `draft_prompt_store`)
- [ ] `grep -rn "selected_agent_type" crates/agent_ui/src/` — clean
- [ ] `grep -nE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/` — clean (only doc comments)
- [ ] `grep -n "wait_for_tools_ready\|smol::Timer" crates/agent/src/agent.rs` — `wait_for_tools_ready` present, no `smol::Timer`
- [ ] `grep -n "allow_multiple_instances\|headless" crates/zed/src/main.rs` — both present, all 3 `--headless` sites
- [ ] `grep -n "debug-embed" Cargo.toml` — present
- [ ] `grep -n "external_websocket_sync::get_thread" crates/agent_ui/src/agent_panel.rs` — Critical Fix #11 entity guard present
- [ ] `grep -n "ensure_thread_initialized\|activate_draft" crates/agent_ui/src/agent_panel.rs` — Fix 1b early-return guard present at the right entry point
- [ ] `grep -n "session_creation_chain" crates/agent_servers/src/acp.rs` — PR #50 chain present
- [ ] Confirm `ConnectedServerState` field set matches what `from_existing_thread()` constructs (re-grep after merge — upstream may have grown it)
- [ ] Confirm `BaseView` enum: if upstream added new variants past `AgentThread`, `Uninitialized`, `Terminal`, add arms to the Helix UI state query loop in `agent_panel.rs::new()`

## Verify Critical Fixes (the 11 in `portingguide.md` §"Critical Fixes")

- [ ] Fix #1: `load_session` keeps `Entity<NativeAgent>` alive (entity-clone or `pending_sessions` shared-task pattern)
- [ ] Fix #2: `thread_view.rs` has no `MessageAdded`/`MessageCompleted`/streaming `EntryUpdated` sends
- [ ] Fix #3: `content_only` present in `acp_thread.rs`
- [ ] Fix #4: `notify_thread_display` called in `thread_service.rs`
- [ ] Fix #5: `flush_stale_pending_for_thread` present in `thread_service.rs`
- [ ] Fix #6: `stopped_emitted_for_task` invariant — exactly one Stopped per `send()`, all paths
- [ ] Fix #7: `unregister_thread` called from `conversation_view.rs`
- [ ] Fix #8: `drop(turn.send_task)` not `cx.background_spawn(turn.send_task)`
- [ ] Fix #9: `stopped_emitted_for_task` guards normal-completion Stopped emission
- [ ] Fix #10: `DEFAULT_REQUEST_TIMEOUT = Duration::from_secs(180)` in `context_server/src/client.rs`
- [ ] Fix #11: entity-identity guard `external_websocket_sync::get_thread(session_id)` at top of `load_agent_thread` in `agent_panel.rs`

## Verify Helix Surface (per `requirements.md` acceptance criteria)

- [ ] `crates/external_websocket_sync/` crate intact (all source files)
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] `from_existing_thread()` constructor on `ConversationView`, matching current `ConnectedServerState` field set
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` in `feature_flags/src/flags.rs`
- [ ] Feature propagation chain intact: `zed/Cargo.toml` declares `external_websocket_sync = ["agent_ui/external_websocket_sync", ...]`; `title_bar` dep `optional = true`

## Verify PRs #50, #55, #56, #57 (Helix behaviour added since 001996)

- [ ] **PR #50** `session_creation_chain: Rc<RefCell<Option<Shared<Task<()>>>>>` field on `AcpConnection` present; `new_session` / `open_or_create_session` acquire the next slot with drop guard
- [ ] **PR #50** `test_concurrent_session_creation_is_serialized` still compiles and (locally) passes
- [ ] **PR #55** `EntryUpdated` emit after streaming-reveal drain present in `acp_thread.rs`
- [ ] **PR #56 Fix 1a** `defer_user_created_thread_until_first_user_message` (or equivalently named) plumbing in `external_websocket_sync`
- [ ] **PR #56 Fix 1b** the `#[cfg(feature = "external_websocket_sync")] { return; }` early-return guard at the BaseView::Uninitialized branch of `ensure_thread_initialized` (or its post-upstream equivalent)
- [ ] **PR #56** the unit test asserting deferred `UserCreatedThread` emit still compiles and passes
- [ ] **PR #57** Phase 16 counter excludes Phase 10's synthetic `UserCreatedThread` ID in `helix-ws-test-server/main.go`

## Walk Rebase Checklist

- [ ] All 44 items in `portingguide.md` §"Rebase Checklist" walked (the silent-drift sweep + critical-fix verification + Helix-surface checks above cover most; treat any unchecked item as a real gap)
- [ ] Pay special attention to items 9 (cfg-gated `agent_panel.rs` blocks), 11 (`ConnectedServerState`), 12 (`AgentConnection` trait impls), 12a (`Stopped` patterns), 31/31a/37 (`acp_thread.rs` cancel/Stopped), 39 (`--allow-multiple-instances`), 39a (`--headless`), 40 (`debug-embed`), 41 (`smol::Timer`), 41a (`Stopped(_)` test pattern)
- [ ] **New checklist item (002029)**: "Check `agent_panel.rs::ensure_thread_initialized` (or its post-upstream equivalent) for `#[cfg(feature = "external_websocket_sync")] { return; }` early-return guard — Helix PR #56 Fix 1b. Phase 17 of E2E is the regression gate."
- [ ] **New checklist item (002029, if signature changes broadly)**: "All `AgentSessionList::supports_delete` impls take `(&self, &App)`; all call sites pass `cx`."

## Build & Test (hard gate)

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds with zero errors
- [ ] If any new `BaseView` variant or trait-signature change surfaces a build failure, fix it and append a "Pre-existing Breakage Repaired" subsection to `portingguide.md` §"Merge 002029"
- [ ] Pre-flight: `cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server && go mod tidy`
- [ ] Copy fresh binary into `e2e-test/zed-binary`: `cp /home/retro/work/helix/zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] Run E2E `zed-agent`: `cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test && ./run_docker_e2e.sh`
- [ ] All 17 phases pass for `zed-agent`, with **Phase 17 as the explicit gate that PR #56 Fix 1b draft suppression survived**
- [ ] Run E2E for both agents: `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` (one retry permitted for Claude Code npm-install bootstrap flake — see 001996 lesson)
- [ ] All 17 phases pass for `claude` as well
- [ ] **If Phase 17 fails**: stop, re-read `agent_panel.rs`, restore the cfg-gated early return at the correct entry point, rebuild, re-run E2E. Do not mark the task complete with Phase 17 failing.
- [ ] If any other phase fails: diagnose root cause, fix, document in `portingguide.md`, re-run

## Update `portingguide.md` (incremental, not at the end)

- [ ] Each conflict resolution appended live (upstream change / resolution / why / risk)
- [ ] New top-level `## Merge 002029 (2026-05-18)` section created, mirroring 001996's structure
- [ ] Mandatory subsection: "PR #56 Fix 1b draft suppression vs upstream `bbe23cc40b` Bring back draft threads" — even if the merge auto-resolved cleanly, document where the Helix guard sits after the merge and how Phase 17 was verified
- [ ] Mandatory subsection: "PR #50 `session_creation_chain` vs upstream `23231879cd` ACP session deletion" — document the side-by-side fold in `crates/agent_servers/src/acp.rs`
- [ ] Subsection (if applicable): "`supports_delete` signature change `(&self)` → `(&self, &App)`" — document scope of compile-driven updates
- [ ] Subsection (if applicable): "ACP logout default impls" — confirm Helix-mode UI does not expose a logout button
- [ ] Any "Pre-existing Breakage Repaired" subsections written for build fixes (`BaseView::*` exhaustiveness, etc.)
- [ ] Commit-history table at bottom of `portingguide.md` extended with this merge's commits and any follow-up fix commits
- [ ] Rebase checklist extended with new items called out above
- [ ] Stale guide entries discovered along the way are corrected or deleted (do not invent updates)

## Re-merge Fork Main (only if needed)

- [ ] Check whether `origin/main` advanced during merge work: `git fetch origin && git log feature/002029-merge-latest-zed..origin/main`
- [ ] If yes: `git merge origin/main` into the feature branch, re-build, re-run E2E

## Finalise

- [ ] Push feature branch to Zed remote: `git push -u origin feature/002029-merge-latest-zed`
- [ ] Write `pull_request_zed.md` in this task directory with summary of upstream changes, conflict resolutions, and validation results
- [ ] In `/home/retro/work/helix/`, create branch `feature/002029-merge-latest-zed`, bump `ZED_COMMIT` in `sandbox-versions.txt` to the new Zed merge HEAD, commit
- [ ] Push the Helix branch: `git push -u origin feature/002029-merge-latest-zed`
- [ ] Write `pull_request_helix.md` in this task directory
- [ ] Do NOT force-push `main` (Zed or Helix) without explicit user approval
- [ ] Do NOT open PRs from the agent — the Helix UI handles PR creation per task convention
