# Implementation Tasks: Merge Latest Zed Upstream (002029)

## Setup

- [x] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, 762 lines as of start of task
- [x] Read prior plan `001996_merge-latest-zed/` end-to-end — closest precedent (mandatory, not optional)
- [x] Skim 001980 plan for the `BaseView::Terminal` exhaustiveness lesson and the GPUI-event-flush race pattern
- [x] Read commit messages of Helix PRs since 001996: `git log 8841edb2b1..origin/main` — PR #50 (`b16e4a948a`), PR #55 (`3778eb04a3`), PR #56 (`32a1e3ba30`, `455c095fcc`, `056fe07180`, `769a463a2f`), PR #57 (`b35224530f`), direct fix (`fd26c1a113`)
- [x] Verify upstream remote: `cd /home/retro/work/zed && git remote -v`. If missing, add: `git remote add upstream https://github.com/zed-industries/zed.git`
- [x] `git fetch upstream && git fetch origin`
- [x] Verify divergence: **261** commits to merge, fork HEAD `fd26c1a113`, upstream HEAD `1399540715` (re-confirm at runtime — numbers may shift if upstream pushed since)
- [x] Pull `origin/main` first in case fork main moved
- [x] Create feature branch: `feature/002029-merge-latest-zed` from current fork main

## Pre-Merge Reconnaissance

- [ ] Read upstream commit `589dc95c87` "Restore last active agent panel entry" (#57150) in full — **highest-risk single commit; rewrites `ensure_thread_initialized` itself with new `pending_terminal_spawn`/`should_create_terminal_for_new_entry`/ACP-restoration branches**. Helix PR #56 Fix 1b's early-return guard must land at the very TOP of the new function body.
- [ ] Read upstream commit `bbe23cc40b` "Bring back draft threads" (#54292) in full — second-highest risk; restructures the draft-thread lifecycle around `draft_prompt_store.rs` and retained drafts.
- [ ] Read upstream commit `c3951af24f` "Support additional session directories" (#57051) in full — adds 489 lines to `agent_servers/src/acp.rs`, expanding `new_session`/`load_session`/`open_or_create_session` signatures. Identify what new parameters PR #50's `session_creation_chain` wrapper needs to pass through.
- [ ] Read upstream commit `23231879cd` "ACP session deletion" (#57004) in full — adds 161 lines to `agent_servers/src/acp.rs` AND cascades the `supports_delete(&self)` → `supports_delete(&self, &App)` signature change through Helix call sites.
- [ ] Read upstream commit `f2df3f9e18` "ACP logout" (#56959) — confirm the new `supports_logout`/`logout` defaults won't surface a logout UI in Helix mode.
- [ ] Read upstream commit `c84c22dab5` "Deprecate ACP extensions" (#57133) — removes 1 459 lines including 80 from `extensions_ui.rs` around Helix's `// HELIX: External agent ...` bypass markers. Decide whether the Helix patch is now redundant.
- [ ] Skim upstream commits touching `acp_thread.rs`: `git log 8bdd78e023..upstream/main -- crates/acp_thread/` — check whether anything overlaps PR #55's 12-line `EntryUpdated` emit site.

## Merge Execution

- [ ] `git merge upstream/main`
- [ ] Triage conflicts; for each, append to `portingguide.md` §"Merge 002029" with `(upstream change / resolution / why / risk)` BEFORE moving to the next one
- [ ] `Cargo.lock` (if conflicting): `git checkout --theirs Cargo.lock`
- [ ] Any `.github/workflows/` conflicts: accept upstream
- [ ] Resolve `crates/agent_ui/src/agent_panel.rs` conflicts — Critical Fix #11 (entity-identity guard at top of `load_agent_thread`) must survive verbatim; **Helix PR #56 Fix 1b draft suppression must be the FIRST statement of the new `ensure_thread_initialized` body** (after `589dc95c87`'s rewrite adds `pending_terminal_spawn` / `should_create_terminal_for_new_entry` / ACP-restoration branches); thread display callback, UI state query, `acp_history_store()`, onboarding bypass, ACP auto-approve preserved
- [ ] Resolve `crates/agent_ui/src/conversation_view.rs` conflicts — `from_existing_thread()`, `THREAD_REGISTRY` registration, `is_resume`, history refresh, unregister-on-reset preserved
- [ ] Resolve `crates/acp_thread/src/acp_thread.rs` conflicts — Critical Fixes #6, #8, #9 (cancel/Stopped invariants) preserved; PR #55 streaming-reveal `EntryUpdated` emit preserved
- [ ] Resolve `crates/acp_thread/src/connection.rs` if conflict — accept upstream trait additions (`supports_logout`, `logout` defaults; `supports_delete(&self, &App)` signature change)
- [ ] Resolve `crates/agent_servers/src/acp.rs` conflicts — **three-way fold** of PR #50 `session_creation_chain` with upstream `23231879cd` session-deletion plumbing AND `c3951af24f`'s 489-line expansion of `new_session`/`load_session`/`open_or_create_session` (PR #50's chain wrapper may need new parameters passed through)
- [ ] Resolve `crates/agent/src/agent.rs` conflicts — Critical Fix #1 (entity-lifetime clone in `load_session`) preserved; `wait_for_tools_ready` uses `cx.background_executor().timer()`; update `supports_delete(&self)` → `supports_delete(&self, &App)` signature on line 1838
- [ ] Decide on `crates/extensions_ui/src/extensions_ui.rs`: upstream `c84c22dab5` deletes 80 lines around Helix's `// HELIX: External agent ...` bypass markers. If upstream's deletion subsumes the bypass intent (no agent keywords / no agent upsells), drop the Helix patch deliberately and document. If not, re-apply.
- [ ] Decide on `crates/agent_servers/src/custom.rs`: upstream `c84c22dab5` guts it (116 → ~40 lines). Confirm Helix's `ExternalAgent::server()` path with `CustomAgentServer::new(AgentId(...))` still compiles.
- [ ] Resolve any other Helix-touched file conflicts (workspace.rs, main.rs, title_bar, Cargo.toml workspace deps)
- [ ] Compile-driven trait-signature migration: walk every `supports_delete()` call site — confirmed 10 references in `crates/agent_ui/src/acp/thread_history.rs` (lines 362, 365, 563, 574, 700, 797, 868, 879, 884–885) plus impl in `crates/agent/src/agent.rs:1838` — and add `cx` / `&App` everywhere upstream's signature change demands
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
- [ ] `grep -n "ensure_thread_initialized\|activate_draft" crates/agent_ui/src/agent_panel.rs` — Fix 1b early-return guard present; **read the full function body** and confirm the cfg-gated `return;` is the FIRST statement, before `pending_terminal_spawn` / `should_create_terminal_for_new_entry` / ACP-restoration branches
- [ ] `grep -n "session_creation_chain" crates/agent_servers/src/acp.rs` — PR #50 chain present; also `grep -n "fn delete_session\|fn logout"` to confirm upstream additions coexist
- [ ] `grep -n "helix-org" crates/external_websocket_sync/e2e-test/Dockerfile.ci` — fork's `fd26c1a113` Dockerfile.ci fix still present
- [ ] `grep -n "HELIX: External agent" crates/extensions_ui/src/extensions_ui.rs` — if absent (because upstream `c84c22dab5` deleted the surrounding surface), document the deliberate drop in `portingguide.md`
- [ ] `grep -n "AcpBetaFeatureFlag\|enabled_for_all" crates/feature_flags/src/flags.rs` — Helix's `enabled_for_all() -> true` override still present
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

## Verify PRs #50, #55, #56, #57 + `fd26c1a113` (Helix behaviour added since 001996)

- [ ] **PR #50** `session_creation_chain: Rc<RefCell<Option<Shared<Task<()>>>>>` field on `AcpConnection` present; `new_session` / `open_or_create_session` acquire the next slot with drop guard; **any new arguments demanded by upstream `c3951af24f` are correctly threaded through the chain wrapper**
- [ ] **PR #50** `test_concurrent_session_creation_is_serialized` still compiles and (locally) passes
- [ ] **PR #55** `EntryUpdated` emit after streaming-reveal drain present in `acp_thread.rs`
- [ ] **PR #56 Fix 1a** `defer_user_created_thread_until_first_user_message` (or equivalently named) plumbing in `external_websocket_sync`
- [ ] **PR #56 Fix 1b** the `#[cfg(feature = "external_websocket_sync")] { return; }` early-return guard is the FIRST statement of `ensure_thread_initialized`'s body — before `pending_terminal_spawn` / `should_create_terminal_for_new_entry` / ACP-restoration branches introduced by upstream `589dc95c87`
- [ ] **PR #56** the unit test asserting deferred `UserCreatedThread` emit still compiles and passes
- [ ] **PR #57** Phase 16 counter excludes Phase 10's synthetic `UserCreatedThread` ID in `helix-ws-test-server/main.go`
- [ ] **`fd26c1a113`** `Dockerfile.ci` still pulls `helix-org` (the 2-line CI fix)

## Walk Rebase Checklist

- [ ] All 44 items in `portingguide.md` §"Rebase Checklist" walked (the silent-drift sweep + critical-fix verification + Helix-surface checks above cover most; treat any unchecked item as a real gap)
- [ ] Pay special attention to items 9 (cfg-gated `agent_panel.rs` blocks), 11 (`ConnectedServerState`), 12 (`AgentConnection` trait impls), 12a (`Stopped` patterns), 31/31a/37 (`acp_thread.rs` cancel/Stopped), 39 (`--allow-multiple-instances`), 39a (`--headless`), 40 (`debug-embed`), 41 (`smol::Timer`), 41a (`Stopped(_)` test pattern)
- [ ] **New checklist item (002029)**: "Check `agent_panel.rs::ensure_thread_initialized` for `#[cfg(feature = "external_websocket_sync")] { return; }` early-return guard — Helix PR #56 Fix 1b. After upstream `589dc95c87` the function body has multiple branches before `activate_draft`; the Helix guard MUST be the FIRST statement of the body. Phase 17 of E2E is the regression gate."
- [ ] **New checklist item (002029)**: "All `AgentSessionList::supports_delete` impls take `(&self, &App)`; all call sites pass `cx`."
- [ ] **New checklist item (002029, conditional)**: If `c84c22dab5` made the Helix `// HELIX: External agent ...` bypass in `extensions_ui.rs` redundant, record this as a deliberate drop so future merges don't re-add the now-obsolete patch.

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
- [ ] New top-level `## Merge 002029 (2026-05-21)` section created, mirroring 001996's structure
- [ ] **Mandatory subsection**: "PR #56 Fix 1b draft suppression vs upstream `589dc95c87` Restore last active agent panel entry + `bbe23cc40b` Bring back draft threads" — document where the Helix early-return guard sits in the post-merge `ensure_thread_initialized` body and how Phase 17 was verified for both `zed-agent` and `claude`
- [ ] **Mandatory subsection**: "PR #50 `session_creation_chain` vs upstream `23231879cd` ACP session deletion + `c3951af24f` additional session directories" — document the three-way fold in `crates/agent_servers/src/acp.rs`; record any new parameters threaded through the chain wrapper
- [ ] Subsection: "`supports_delete` signature change `(&self)` → `(&self, &App)`" — document scope (10 call sites in `thread_history.rs`, 1 impl in `agent.rs:1838`)
- [ ] Subsection: "ACP logout default impls (`f2df3f9e18`)" — confirm Helix-mode UI does not expose a logout button
- [ ] Subsection (conditional): "`c84c22dab5` ACP-extension deprecation absorbed Helix `// HELIX: External agent ...` bypass; patch deliberately dropped" — only if confirmed
- [ ] Any "Pre-existing Breakage Repaired" subsections written for build fixes (`BaseView::*` exhaustiveness, new trait signatures missed, etc.)
- [ ] Commit-history table at bottom of `portingguide.md` extended with this merge's commits and any follow-up fix commits
- [ ] Rebase checklist extended with new items called out above
- [ ] Stale guide entries discovered along the way are corrected or deleted (do not invent updates)

## Re-merge Fork Main (only if needed)

- [ ] Check whether `origin/main` advanced during merge work: `git fetch origin && git log feature/002029-merge-latest-zed..origin/main`
- [ ] If yes: `git merge origin/main` into the feature branch, re-build, re-run E2E. This is especially likely given that 5 Helix-only PRs landed in the 10 days between 001996 and 002029.

## Finalise

- [ ] Push feature branch to Zed remote: `git push -u origin feature/002029-merge-latest-zed`
- [ ] Write `pull_request_zed.md` in this task directory with summary of upstream changes, conflict resolutions, and validation results
- [ ] In `/home/retro/work/helix/`, create branch `feature/002029-merge-latest-zed`, bump `ZED_COMMIT` in `sandbox-versions.txt` from `b35224530f7c2ff5ead8b9cfcea23b050583d70d` to the new Zed merge HEAD, commit
- [ ] Push the Helix branch: `git push -u origin feature/002029-merge-latest-zed`
- [ ] Write `pull_request_helix.md` in this task directory
- [ ] Do NOT force-push `main` (Zed or Helix) without explicit user approval
- [ ] Do NOT open PRs from the agent — the Helix UI handles PR creation per task convention
