# Implementation Tasks: Merge Latest Zed Upstream (002077)

## Setup

- [ ] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, **892 lines** as of start of task; latest entry `## Merge 002029-extension round 2 (2026-06-02)` at line 750
- [ ] Read prior plan `002029_merge-latest-zed/` end-to-end (requirements.md, design.md, tasks.md, pull_request_zed.md, pull_request_helix.md) — closest precedent (mandatory)
- [ ] Skim 002059 plan to understand why it was scoped (assumed 002029 still open; rendered moot when PR #58 landed 2026-06-02). 002077 is the direct next merge after 002029 landed; do NOT reuse the `feature/002059-merge-latest-zed` branch slot
- [ ] Verify upstream remote: `cd /home/retro/work/zed && git remote -v`. If missing, add: `git remote add upstream https://github.com/zed-industries/zed.git`
- [ ] `git fetch upstream && git fetch origin`
- [ ] Verify divergence: **139** commits to merge, fork HEAD `79b9bfb1d6`, upstream HEAD `3f5705b985` (re-confirm at runtime — numbers may shift if upstream pushed since planning)
- [ ] Confirm Helix-only commits since 002029 = 0: `git log 79b9bfb1d6..origin/main --no-merges` should be empty
- [ ] Pull `origin/main` first in case fork main moved
- [ ] Create feature branch: `feature/002077-merge-latest-zed` from current fork main

## Pre-Merge Reconnaissance

- [ ] Read upstream commit `215ca2fb0b` "Typed workspace errors (#57649)" in full — **highest-risk single commit; migrates `Workspace::show_error` to a generic `<E: WorkspaceError>` signature**. Identify every Helix call site that will break (likely `external_websocket_sync/src/thread_service.rs` and possibly `agent_panel.rs`)
- [ ] Read upstream commit `116e4bc184` "agent_ui: Inherit source agent without draft content (#58636)" — touches the activate-draft / draft-inheritance path. Determine whether Fix 1b's first-statement position in `BaseView::Uninitialized` is at risk
- [ ] Read upstream commit `27191913e9` "agent: Accumulate cumulative token usage (#58378)" — revives `Thread::cumulative_token_usage`. Determine whether the WS sync payload schema is affected
- [ ] Read upstream commit `56b71271c4` "acp: Enable ACP session usage and deletion features (#58680)" — confirm no default-flip the Helix `AcpConnection` impl needs to override
- [ ] Read upstream commit `89cac4944d` "Improve sandbox write-path handling (#58283)" — confirm coexistence with Helix's `show_onboarding` / `auto_open_panel` fields in `agent_settings` / `settings_content`
- [ ] Read upstream commit `a32999e00b` "workspace: Update window title when switching active workspace (#58401)" — confirm `CollaboratorId::Agent` follow-focus guard unaffected
- [ ] Skim upstream commits touching `acp_thread.rs` (4 commits, +184 lines): `git log 9d50bab893..upstream/main -- crates/acp_thread/` — check overlap with PR #55's `EntryUpdated` emit site and Critical Fixes #6/#8/#9 (cancel/Stopped)

## Merge Execution

- [ ] `git merge upstream/main`
- [ ] Triage conflicts; for each, append to `portingguide.md` §"Merge 002077" with `(upstream change / resolution / why / risk)` BEFORE moving to the next one
- [ ] `Cargo.lock` (if conflicting): `git checkout --theirs Cargo.lock`
- [ ] Any `.github/workflows/` conflicts: accept upstream
- [ ] Resolve `crates/agent_ui/src/agent_panel.rs` conflicts — Critical Fix #11 entity-identity guard (now `thread_id`-based after 002029) must survive verbatim; **Fix 1b draft suppression `#[cfg(feature = "external_websocket_sync")] { return; }` MUST be the FIRST statement of the `BaseView::Uninitialized` branch**, even if `116e4bc184` source-agent inheritance restructures the surrounding code; thread display callback, UI state query, `acp_history_store()`, onboarding bypass, ACP auto-approve preserved
- [ ] Resolve `crates/agent_ui/src/conversation_view.rs` conflicts — `from_existing_thread()` may need a fourth round of signature-drift repair (mirror upstream's `ConversationView::new()` field-by-field); `THREAD_REGISTRY` registration, `is_resume`, history refresh, unregister-on-reset preserved
- [ ] Resolve `crates/acp_thread/src/acp_thread.rs` conflicts — Critical Fixes #6/#8/#9 (cancel/Stopped invariants) preserved; PR #55 streaming-reveal `EntryUpdated` emit preserved; check `de744e744c` "Correctly handle file links" for conflict against PR #55's emit site
- [ ] Resolve `crates/agent_servers/src/acp.rs` conflicts (only 2 upstream commits, +4/-91 — should be cleanup) — PR #50 `session_creation_chain` + `_settings_subscription` (002029-round-2) coexist
- [ ] Resolve `crates/agent/src/agent.rs` conflicts — Critical Fix #1 (entity-lifetime clone in `load_session`) preserved; `wait_for_tools_ready` uses `cx.background_executor().timer()`; `supports_delete(&self, &App)` impl on line ~1838 preserved
- [ ] Resolve `crates/workspace/src/workspace.rs` conflicts — `215ca2fb0b` typed-errors and `a32999e00b` window-title-tracking touch this file; the `CollaboratorId::Agent` follow-focus guard must survive
- [ ] Resolve `crates/zed/src/main.rs` conflicts — `--allow-multiple-instances`, `--headless`, `initialize_headless()`, `build_application(headless: bool)` (002029-round-2 pattern) preserved
- [ ] Resolve `crates/agent_settings/src/agent_settings.rs` + `crates/settings_content/src/agent.rs` — `89cac4944d` extends `sandbox_permissions`; "both sides added a field" three-way coexistence with Helix's `show_onboarding` / `auto_open_panel`
- [ ] Resolve `crates/title_bar/` conflicts — `external_websocket_sync = { workspace = true, optional = true }` dep + cfg-gated `render_restricted_mode` early return preserved
- [ ] Resolve `crates/extensions_ui/src/extensions_ui.rs` if touched by `215ca2fb0b` typed-errors — `// HELIX: External agent ...` bypass markers retained (lines ~221, ~243, ~1513)
- [ ] Resolve `crates/Cargo.toml` workspace deps if conflicting — `rust-embed` `debug-embed` feature preserved
- [ ] Compile-driven `Workspace::show_error` migration: walk every Helix call site surfaced by `./stack build-zed dev` and migrate to the new `<E: WorkspaceError>` signature (impl `WorkspaceError` for a Helix error type, or ad-hoc wrap, or use upstream convenience constructor — document chosen approach in porting guide)
- [ ] No conflict markers remain: `grep -rn "<<<<<<<\|>>>>>>>" .` (excluding test-string markers in `git_store.rs`)
- [ ] Commit merge: `git commit` (let git auto-generate the merge commit; do not amend)

## Sweep for Silent Drift (auto-merged files)

- [ ] `grep -rn "ActiveView" crates/agent_ui/src/` — only `AgentPanelEvent::ActiveView*` valid
- [ ] `grep -rn "set_active_view" crates/agent_ui/src/` — clean
- [ ] `grep -rn "draft_threads\|background_threads" crates/agent_ui/src/` — clean
- [ ] `grep -rn "selected_agent_type" crates/agent_ui/src/` — clean
- [ ] `grep -nE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/` — clean (only doc comments)
- [ ] `grep -n "wait_for_tools_ready\|smol::Timer" crates/agent/src/agent.rs` — `wait_for_tools_ready` present, no `smol::Timer`
- [ ] `grep -n "allow_multiple_instances\|headless\|build_application" crates/zed/src/main.rs` — all present, `build_application(headless: bool)` pattern intact
- [ ] `grep -n "debug-embed" Cargo.toml` — present
- [ ] `grep -n "external_websocket_sync::get_thread" crates/agent_ui/src/agent_panel.rs` — Critical Fix #11 entity guard present
- [ ] `grep -n "ensure_thread_initialized\|activate_draft" crates/agent_ui/src/agent_panel.rs` — Fix 1b early-return present; **read the full function body** and confirm the cfg-gated `return;` is the FIRST statement of the `BaseView::Uninitialized` branch, before any source-agent-inheritance / terminal-spawn / ACP-restoration branches
- [ ] `grep -n "session_creation_chain\|_settings_subscription" crates/agent_servers/src/acp.rs` — both present (002029 PR #50 + 002029-round-2 coexistence)
- [ ] `grep -n "helix-org" crates/external_websocket_sync/e2e-test/Dockerfile.ci` — fork's `fd26c1a113` Dockerfile.ci fix present
- [ ] `grep -n "HELIX: External agent" crates/extensions_ui/src/extensions_ui.rs` — bypass markers retained at lines ~221, ~243, ~1513
- [ ] `grep -n "AcpBetaFeatureFlag\|enabled_for_all" crates/feature_flags/src/flags.rs` — Helix's `enabled_for_all() -> true` override present
- [ ] `grep -n "render_restricted_mode" crates/title_bar/src/title_bar.rs` — cfg-gated early return present
- [ ] `grep -rn "Workspace::show_error\|workspace.show_error\|\.show_error(" crates/external_websocket_sync/ crates/agent_ui/src/` — every site uses the new `WorkspaceError` generic signature (post-`215ca2fb0b`)
- [ ] `grep -rn "cumulative_token_usage\|TokenUsage" crates/external_websocket_sync/` — if any hit, confirm WS payload schema unchanged or document the bump (`27191913e9`)
- [ ] Confirm `ConversationView` field set matches what `from_existing_thread()` constructs (every previous merge required a repair — diff field-by-field against upstream `ConversationView::new()`)
- [ ] Confirm `BaseView` enum: if upstream added new variants past `AgentThread`, `Uninitialized`, `Terminal`, add arms to the Helix UI state query loop in `agent_panel.rs::new()` AND the headless responder in `zed/src/main.rs`
- [ ] Confirm `ContextServerStatus` enum: if upstream added new variants past the 002029 set (which added `ClientSecretRequired`), add arms in both UI-state-query loops

## Verify Critical Fixes (the 10 active fixes in `portingguide.md` §"Critical Fixes" — #10 stays retired)

- [ ] Fix #1: `load_session` keeps `Entity<NativeAgent>` alive (entity-clone or `pending_sessions` shared-task pattern)
- [ ] Fix #2: `thread_view.rs` has no `MessageAdded` / `MessageCompleted` / streaming `EntryUpdated` sends
- [ ] Fix #3: `content_only` present in `acp_thread.rs`
- [ ] Fix #4: `notify_thread_display` called in `thread_service.rs`
- [ ] Fix #5: `flush_stale_pending_for_thread` present in `thread_service.rs`
- [ ] Fix #6: `stopped_emitted_for_task` invariant — exactly one Stopped per `send()`, all paths
- [ ] Fix #7: `unregister_thread` called from `conversation_view.rs`
- [ ] Fix #8: `drop(turn.send_task)` not `cx.background_spawn(turn.send_task)`
- [ ] Fix #9: `stopped_emitted_for_task` guards normal-completion Stopped emission
- [ ] Fix #11: entity-identity guard `external_websocket_sync::get_thread(...)` at top of `load_agent_thread` in `agent_panel.rs` (`thread_id`-based form from 002029)

## Verify Helix Surface (per `requirements.md` acceptance criteria)

- [ ] `crates/external_websocket_sync/` crate intact (all source files)
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] `from_existing_thread()` constructor on `ConversationView`, matching current `ConversationView` field set + `ThreadView::new` arg list
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` in `feature_flags/src/flags.rs`
- [ ] Feature propagation chain intact: `zed/Cargo.toml` declares `external_websocket_sync = ["agent_ui/external_websocket_sync", ...]`; `title_bar` dep `optional = true`

## Verify PRs #50, #55, #56, #57 + `fd26c1a113` (Helix behaviour established before 002029)

- [ ] **PR #50** `session_creation_chain: Rc<RefCell<Option<Shared<Task<()>>>>>` field on `AcpConnection` present; `new_session` / `open_or_create_session` acquire the next slot with drop guard; coexists with `_settings_subscription` (002029-round-2)
- [ ] **PR #50** `test_concurrent_session_creation_is_serialized` still compiles and (locally) passes
- [ ] **PR #55** `EntryUpdated` emit after streaming-reveal drain present in `acp_thread.rs`
- [ ] **PR #56 Fix 1a** `defer_user_created_thread_until_first_user_message` plumbing in `external_websocket_sync`
- [ ] **PR #56 Fix 1b** `#[cfg(feature = "external_websocket_sync")] { return; }` is the FIRST statement of the `BaseView::Uninitialized` branch in `ensure_thread_initialized` — after `116e4bc184` source-agent-inheritance refactor
- [ ] **PR #56** the unit test asserting deferred `UserCreatedThread` emit still compiles and passes
- [ ] **PR #57** Phase 16 counter excludes Phase 10's synthetic `UserCreatedThread` ID in `helix-ws-test-server/main.go`
- [ ] **`fd26c1a113`** `Dockerfile.ci` still pulls `helix-org`

## Walk Rebase Checklist

- [ ] All numbered items in `portingguide.md` §"Rebase Checklist" walked (silent-drift sweep + critical-fix verification + Helix-surface checks cover most; treat any unchecked item as a real gap)
- [ ] Pay special attention to items 9 (cfg-gated `agent_panel.rs` blocks — Fix 1b position), 11 (`ConversationView` / `ConnectedServerState`), 12 (`AgentConnection` trait impls), 12a (`Stopped` patterns), 31/31a/37 (`acp_thread.rs` cancel/Stopped), 39 (`--allow-multiple-instances`), 39a (`--headless`), 40 (`debug-embed`), 41 (`smol::Timer`), 41a (`Stopped(_)` test pattern), plus 002029 additions on Fix 1b first-statement and `supports_delete(&self, &App)` signature
- [ ] **New checklist item (002077)**: "All Helix `Workspace::show_error` call sites use the new `<E: WorkspaceError>` generic signature (`215ca2fb0b`). Each documented in porting-guide entry."
- [ ] **New checklist item (002077, conditional on `27191913e9` impact)**: "If WS sync payload now includes `cumulative_token_usage`, the Helix API server tolerates it / the schema bump is documented."

## Build & Test (hard gate)

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds with zero errors
- [ ] If any new `BaseView` / `ContextServerStatus` variant or trait-signature change surfaces a build failure, fix it and append a "Pre-existing Breakage Repaired" subsection to `portingguide.md` §"Merge 002077"
- [ ] Pre-flight: `cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server && go mod tidy`
- [ ] Copy fresh binary into `e2e-test/zed-binary`: `cp /home/retro/work/helix/zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] Run E2E `zed-agent`: `cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test && ./run_docker_e2e.sh`
- [ ] All 17 phases pass for `zed-agent`, with **Phase 17 as the explicit gate that PR #56 Fix 1b draft suppression survived**
- [ ] Run E2E for both agents: `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` (one retry permitted for Claude Code npm-install bootstrap flake — see 001996 lesson)
- [ ] All 17 phases pass for `claude` as well
- [ ] **If Phase 17 fails**: stop, re-read `agent_panel.rs::ensure_thread_initialized`, restore the cfg-gated early return as the FIRST statement of the `BaseView::Uninitialized` branch, rebuild, re-run E2E. Do not mark the task complete with Phase 17 failing.
- [ ] If any other phase fails: diagnose root cause, fix, document in `portingguide.md`, re-run

## Update `portingguide.md` (incremental, not at the end)

- [ ] Each conflict resolution appended live (upstream change / resolution / why / risk)
- [ ] New top-level `## Merge 002077 (2026-06-08)` section created at the top of the merge-history list, mirroring 002029-extension round 2 structure (Divergence at start / Manual conflicts / Pre-existing Breakage Repaired / Ancillary upstream notes / Validation)
- [ ] **Mandatory subsection**: "`215ca2fb0b` Typed workspace errors — Helix `show_error` call-site migration" listing each Helix call site and the chosen migration approach
- [ ] **Mandatory subsection**: "`116e4bc184` Inherit source agent without draft content vs Helix PR #56 Fix 1b" — confirm Fix 1b's first-statement position survived; document any code-path change that required moving the guard
- [ ] **Mandatory subsection**: "`27191913e9` Cumulative token usage — WS sync payload schema check" — record whether the WS payload was affected
- [ ] **Mandatory subsection**: "`56b71271c4` Stabilised ACP session usage/deletion — Helix `AcpConnection` impl review" — record whether any Helix override needed updating
- [ ] Subsection (conditional): "`89cac4944d` Sandbox write-path hardening — coexistence with Helix `show_onboarding` / `auto_open_panel` fields"
- [ ] Any "Pre-existing Breakage Repaired" subsections written for build fixes (`BaseView::*` / `ContextServerStatus::*` exhaustiveness, `from_existing_thread()` signature drift, new trait signatures missed, etc.)
- [ ] Commit-history table at bottom of `portingguide.md` extended with this merge's commits and any follow-up fix commits
- [ ] Rebase checklist extended only with **net-new** fragilities discovered in this window (do not invent updates)
- [ ] Stale guide entries discovered along the way are corrected or deleted

## Re-merge Fork Main (only if needed)

- [ ] Check whether `origin/main` advanced during merge work: `git fetch origin && git log feature/002077-merge-latest-zed..origin/main`
- [ ] If yes: `git merge origin/main` into the feature branch, re-build, re-run E2E. (Less likely this round than past merges since baseline was clean at task start.)

## Finalise

- [ ] Push feature branch to Zed remote: `git push -u origin feature/002077-merge-latest-zed`
- [ ] Write `pull_request_zed.md` in this task directory with summary of upstream changes, conflict resolutions, and validation results
- [ ] In `/home/retro/work/helix/`, create branch `feature/002077-merge-latest-zed`, bump `ZED_COMMIT` in `sandbox-versions.txt` from `79b9bfb1d60cbef5b14ba7e3992ba5e8f6eb335c` to the new Zed merge HEAD, commit
- [ ] Push the Helix branch: `git push -u origin feature/002077-merge-latest-zed`
- [ ] Write `pull_request_helix.md` in this task directory
- [ ] Do NOT force-push `main` (Zed or Helix) without explicit user approval
- [ ] Do NOT open PRs from the agent — the Helix UI handles PR creation per task convention
