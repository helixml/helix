# Implementation Tasks: Merge Latest Zed Upstream (002077)

## Setup

- [~] Read `/home/retro/work/zed/portingguide.md` in full — canonical reference, **892 lines** as of start of task; latest entry `## Merge 002029-extension round 2 (2026-06-02)` at line 750
- [x] Read prior plan `002029_merge-latest-zed/` end-to-end (requirements.md, design.md, tasks.md, pull_request_zed.md, pull_request_helix.md) — closest precedent (mandatory)
- [x] Skim 002059 plan to understand context; do NOT reuse `feature/002059-merge-latest-zed` (task was planned but never executed)
- [~] Read PR #60 commits in full: `git show 27e8867c9e` (retry loop) + `git show e4c36d837c` (cleanup). The retry logic in `crates/external_websocket_sync/src/thread_service.rs::handle_follow_up_message` must survive any cleanup
- [x] Verify upstream remote: `cd /home/retro/work/zed && git remote -v`. If missing, add: `git remote add upstream https://github.com/zed-industries/zed.git`
- [x] `git fetch upstream && git fetch origin`
- [x] Verify divergence: **256** commits to merge, fork HEAD `ecdc2ea67d`, upstream HEAD `992f395c3d` (re-confirm at runtime — numbers may shift if anyone pushed since planning)
- [x] Confirm Helix-only commits since 002029: `git log 79b9bfb1d6..origin/main --no-merges` should show `27e8867c9e` + `e4c36d837c` (PR #60). If more, read them.
- [ ] Pull `origin/main` first in case fork main moved
- [ ] Create feature branch: `feature/002077-merge-latest-zed` from current fork main

## Pre-Merge Reconnaissance

- [ ] Read upstream commit `d7ac5e6cf4` "acp_thread: Preserve waiting tool call status on updates (#58537)" in full — **highest-risk single commit; +602 lines across 6 files reworking `ToolCall::status` propagation through entry updates**. Identify where PR #55's streaming-reveal `EntryUpdated` emit needs to land post-merge and trace every new emit site for Fix #6 (`stopped_emitted_for_task`) regression risk
- [ ] Read the **compaction cluster**: `e5052961af` "/compact slash command" (+1065 lines), `9baefe701e` "auto_compact agent settings", `e17e272d24` "compaction UX", `5c90b0664f` "compaction-cancel race fix" (+97), `620ceaaaca` "flush thread content to database on app quit" (+103), `0bc6c76fcf` "Hide token usage after /compact", `0e9e8d0e68` "compaction UI refinements". Identify (a) whether the cluster adds WS payload fields, (b) whether `auto_compact` is a new settings field needing three-way coexistence with `show_onboarding`/`auto_open_panel`/`sandbox_permissions`, (c) whether `620ceaaaca` flush-on-quit races the Helix WS-authoritative store
- [ ] Read upstream commit `215ca2fb0b` "Typed workspace errors (#57649)" + follow-up `83aa943705` "Fix overflow in error popup (#59185)" — identify every Helix `Workspace::show_error` call site that will break
- [ ] Read upstream commit `116e4bc184` "agent_ui: Inherit source agent without draft content (#58636)" — determine Fix 1b position risk
- [ ] Read upstream commit `27191913e9` "agent: Accumulate cumulative token usage (#58378)" — combined with `0bc6c76fcf`, determine WS payload schema risk
- [ ] Read upstream commit `56b71271c4` "acp: Enable ACP session usage and deletion features (#58680)" — confirm no default-flip the Helix `AcpConnection` impl needs
- [ ] Read upstream commit `89cac4944d` "Improve sandbox write-path handling (#58283)" — confirm coexistence with Helix settings fields
- [ ] Read upstream commit `a32999e00b` "workspace: Update window title (#58401)" — confirm `CollaboratorId::Agent` follow-focus guard unaffected
- [ ] Read upstream commit `fef979dec4` "language_models: Add Anthropic-compatible provider support (#50381)" — confirm enterprise-TLS-skip and built-in-agent-hiding patches unaffected
- [ ] Skim upstream commits touching `acp_thread.rs` (12 commits, +1102 lines): `git log 9d50bab893..upstream/main -- crates/acp_thread/`
- [ ] Skim upstream commits touching `agent.rs` (12 commits, +765 lines): `git log 9d50bab893..upstream/main -- crates/agent/src/agent.rs`

## Merge Execution

- [ ] `git merge upstream/main`
- [ ] Triage conflicts; for each, append to `portingguide.md` §"Merge 002077" with `(upstream change / resolution / why / risk)` BEFORE moving to the next one
- [ ] `Cargo.lock` (if conflicting): `git checkout --theirs Cargo.lock`
- [ ] Any `.github/workflows/` conflicts: accept upstream
- [ ] Resolve `crates/acp_thread/src/acp_thread.rs` conflicts — Critical Fixes #6/#8/#9 (cancel/Stopped invariants) preserved; PR #55 streaming-reveal `EntryUpdated` emit re-anchored against `d7ac5e6cf4`'s tool-call-status rewrite; `5c90b0664f`'s compaction-cancel race fix integrated without re-introducing double-`Stopped` emits
- [ ] Resolve `crates/agent/src/agent.rs` conflicts — Critical Fix #1 (entity-lifetime clone in `load_session`) preserved; `wait_for_tools_ready` uses `cx.background_executor().timer()` (not `smol::Timer`); `supports_delete(&self, &App)` impl preserved; compaction cluster integration reviewed; `620ceaaaca` flush-on-quit reviewed (gate behind `not(feature = "external_websocket_sync")` if it races the WS-authoritative store, otherwise leave and document)
- [ ] Resolve `crates/agent_ui/src/agent_panel.rs` conflicts — Critical Fix #11 entity-identity guard (now `thread_id`-based) must survive verbatim; **Fix 1b draft suppression `#[cfg(feature = "external_websocket_sync")] { return; }` MUST be the FIRST statement of the `BaseView::Uninitialized` branch**, even if `116e4bc184` or other commits restructure the surrounding code; thread display callback, UI state query, `acp_history_store()`, onboarding bypass, ACP auto-approve preserved
- [ ] Resolve `crates/agent_ui/src/conversation_view.rs` conflicts — `from_existing_thread()` likely needs a fourth round of signature-drift repair (mirror upstream's `ConversationView::new()` field-by-field including any compaction-related fields); `THREAD_REGISTRY` registration, `is_resume`, history refresh, unregister-on-reset preserved
- [ ] Resolve `crates/workspace/src/workspace.rs` conflicts — `215ca2fb0b` typed-errors + `83aa943705` overflow fix + `a32999e00b` window-title-tracking all touch this file; `CollaboratorId::Agent` follow-focus guard must survive
- [ ] Resolve `crates/agent_servers/src/acp.rs` conflicts (only 3 upstream commits, +5/-92 — should be cleanup) — PR #50 `session_creation_chain` + `_settings_subscription` (002029-round-2) coexist
- [ ] Resolve `crates/zed/src/main.rs` conflicts — `--allow-multiple-instances`, `--headless`, `initialize_headless()`, `build_application(headless: bool)` (002029-round-2) preserved
- [ ] Resolve `crates/agent_settings/src/agent_settings.rs` + `crates/settings_content/src/agent.rs` — `89cac4944d` extends `sandbox_permissions`; `9baefe701e` adds `auto_compact`; "both sides added a field" three-way coexistence with Helix's `show_onboarding` / `auto_open_panel`
- [ ] Resolve `crates/title_bar/` conflicts — `external_websocket_sync = { workspace = true, optional = true }` dep + cfg-gated `render_restricted_mode` early return preserved
- [ ] Resolve `crates/extensions_ui/src/extensions_ui.rs` if touched — `// HELIX: External agent ...` bypass markers retained at lines ~221, ~243, ~1513
- [ ] Resolve `crates/Cargo.toml` workspace deps if conflicting — `rust-embed` `debug-embed` feature preserved
- [ ] Compile-driven `Workspace::show_error` migration: walk every Helix call site surfaced by `./stack build-zed dev` and migrate to the new `<E: WorkspaceError>` signature (implement `WorkspaceError` for a Helix error type, ad-hoc wrap per site, or use upstream convenience constructor — document chosen approach in porting guide)
- [ ] **Verify PR #60 retry loop intact**: `grep -n "ede_diagnostic\|handle_follow_up_message" crates/external_websocket_sync/src/thread_service.rs` — must show the 4×750ms backoff retry block
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
- [ ] `grep -n "ensure_thread_initialized\|activate_draft" crates/agent_ui/src/agent_panel.rs` — Fix 1b early-return present; **read the full function body** and confirm the cfg-gated `return;` is the FIRST statement of the `BaseView::Uninitialized` branch, before any source-agent-inheritance / terminal-spawn / ACP-restoration / title-display branches
- [ ] `grep -n "session_creation_chain\|_settings_subscription" crates/agent_servers/src/acp.rs` — both present (PR #50 + 002029-round-2 coexistence)
- [ ] `grep -n "helix-org" crates/external_websocket_sync/e2e-test/Dockerfile.ci` — fork's `fd26c1a113` Dockerfile.ci fix present
- [ ] `grep -n "ede_diagnostic" crates/external_websocket_sync/src/thread_service.rs` — PR #60 retry loop intact
- [ ] `grep -n "HELIX: External agent" crates/extensions_ui/src/extensions_ui.rs` — bypass markers retained at lines ~221, ~243, ~1513
- [ ] `grep -n "AcpBetaFeatureFlag\|enabled_for_all" crates/feature_flags/src/flags.rs` — Helix's `enabled_for_all() -> true` override present
- [ ] `grep -n "render_restricted_mode" crates/title_bar/src/title_bar.rs` — cfg-gated early return present
- [ ] `grep -rn "Workspace::show_error\|workspace.show_error\|\.show_error(" crates/external_websocket_sync/ crates/agent_ui/src/` — every site uses the new `WorkspaceError` generic signature
- [ ] `grep -rn "cumulative_token_usage\|TokenUsage\|compact\|Compact\|compaction" crates/external_websocket_sync/` — if any hit, confirm WS payload schema unchanged or document the bump
- [ ] Confirm `ConversationView` field set matches what `from_existing_thread()` constructs (diff field-by-field against upstream `ConversationView::new()`)
- [ ] Confirm `BaseView` enum: if upstream added new variants past `AgentThread`, `Uninitialized`, `Terminal`, add arms to the Helix UI state query loop in `agent_panel.rs::new()` AND the headless responder in `zed/src/main.rs`
- [ ] Confirm `ContextServerStatus` enum: if upstream added new variants past the 002029 set (which added `ClientSecretRequired`), add arms in both UI-state-query loops

## Verify Critical Fixes (the 10 active fixes — #10 stays retired)

- [ ] Fix #1: `load_session` keeps `Entity<NativeAgent>` alive (survives compaction cluster + `620ceaaaca` flush-on-quit)
- [ ] Fix #2: `thread_view.rs` has no `MessageAdded` / `MessageCompleted` / streaming `EntryUpdated` sends
- [ ] Fix #3: `content_only` present in `acp_thread.rs`
- [ ] Fix #4: `notify_thread_display` called in `thread_service.rs`
- [ ] Fix #5: `flush_stale_pending_for_thread` present in `thread_service.rs`
- [ ] Fix #6: `stopped_emitted_for_task` invariant — exactly one Stopped per `send()`, all paths (survives `d7ac5e6cf4`'s ToolCall-status rewrite + `5c90b0664f`'s compaction-cancel race fix)
- [ ] Fix #7: `unregister_thread` called from `conversation_view.rs`
- [ ] Fix #8: `drop(turn.send_task)` not `cx.background_spawn(turn.send_task)`
- [ ] Fix #9: `stopped_emitted_for_task` guards normal-completion Stopped emission
- [ ] Fix #11: entity-identity guard `external_websocket_sync::get_thread(...)` at top of `load_agent_thread` in `agent_panel.rs` (`thread_id`-based form)

## Verify Helix Surface

- [ ] `crates/external_websocket_sync/` crate intact (all source files)
- [ ] **PR #60 `handle_follow_up_message` 4×750ms `ede_diagnostic` retry intact**
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] `from_existing_thread()` constructor on `ConversationView`, matching current field set + `ThreadView::new` arg list
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` in `feature_flags/src/flags.rs`
- [ ] Feature propagation chain intact: `zed/Cargo.toml` declares `external_websocket_sync = ["agent_ui/external_websocket_sync", ...]`; `title_bar` dep `optional = true`

## Verify PRs #50, #55, #56, #57, #60 + `fd26c1a113`

- [ ] **PR #50** `session_creation_chain` field on `AcpConnection` present; coexists with `_settings_subscription`
- [ ] **PR #50** `test_concurrent_session_creation_is_serialized` compiles and (locally) passes
- [ ] **PR #55** `EntryUpdated` emit after streaming-reveal drain present in `acp_thread.rs` — re-anchored against `d7ac5e6cf4`'s tool-call-status rewrite; document the post-merge location
- [ ] **PR #56 Fix 1a** `defer_user_created_thread_until_first_user_message` plumbing in `external_websocket_sync`
- [ ] **PR #56 Fix 1b** `#[cfg(feature = "external_websocket_sync")] { return; }` is the FIRST statement of `BaseView::Uninitialized` branch
- [ ] **PR #56** the unit test asserting deferred `UserCreatedThread` emit compiles and passes
- [ ] **PR #57** Phase 16 counter excludes Phase 10's synthetic `UserCreatedThread` ID in `helix-ws-test-server/main.go`
- [ ] **PR #60** retry loop in `handle_follow_up_message` intact (no upstream churn this window — guard against careless cleanup)
- [ ] **`fd26c1a113`** `Dockerfile.ci` still pulls `helix-org`

## Walk Rebase Checklist

- [ ] All numbered items in `portingguide.md` §"Rebase Checklist" walked
- [ ] Pay special attention to items 9 (cfg-gated `agent_panel.rs` blocks — Fix 1b position), 11 (`ConversationView` field set), 12 (`AgentConnection` trait impls), 12a (`Stopped` patterns), 31/31a/37 (`acp_thread.rs` cancel/Stopped — `d7ac5e6cf4` + compaction-cancel race risk), 39 (`--allow-multiple-instances`), 39a (`--headless`), 40 (`debug-embed`), 41 (`smol::Timer`), 41a (`Stopped(_)` test pattern), plus 002029 additions on Fix 1b first-statement and `supports_delete(&self, &App)` signature
- [ ] **New checklist item (002077)**: "All Helix `Workspace::show_error` call sites use the new `<E: WorkspaceError>` generic signature (`215ca2fb0b`+`83aa943705`)."
- [ ] **New checklist item (002077)**: "PR #60 retry block in `external_websocket_sync/src/thread_service.rs::handle_follow_up_message` retains the 4×750ms `ede_diagnostic` backoff. Phase 9 of the E2E is the regression gate."
- [ ] **New checklist item (002077, conditional)**: "If `620ceaaaca` flush-on-quit was gated behind `not(feature = "external_websocket_sync")`, document the rationale; otherwise document why the WS-authoritative store tolerates the upstream flush."
- [ ] **New checklist item (002077, conditional)**: "If the compaction cluster introduced new WS payload fields, the schema bump is documented and the Helix API server tolerates them."

## Build & Test (hard gate)

- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds with zero errors
- [ ] If any new `BaseView` / `ContextServerStatus` variant or trait-signature change surfaces a build failure, fix it and append a "Pre-existing Breakage Repaired" subsection to `portingguide.md` §"Merge 002077"
- [ ] Pre-flight: `cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server && go mod tidy`
- [ ] Copy fresh binary into `e2e-test/zed-binary`: `cp /home/retro/work/helix/zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary`
- [ ] Run E2E `zed-agent`: `cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test && ./run_docker_e2e.sh`
- [ ] All 17 phases pass for `zed-agent`, with:
  - **Phase 9** as the explicit gate that PR #60's `ede_diagnostic` retry-loop survived
  - **Phase 15** as the explicit gate that PR #55's `EntryUpdated` emit survived `d7ac5e6cf4`'s rewrite
  - **Phase 17** as the explicit gate that PR #56 Fix 1b draft suppression survived
- [ ] Run E2E for both agents: `E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh` (one retry permitted for Claude Code npm-install bootstrap flake — see 001996 lesson)
- [ ] All 17 phases pass for `claude` as well
- [ ] **If Phase 9 fails**: re-verify PR #60 retry block is intact and that no upstream commit added a new send path that bypasses it
- [ ] **If Phase 15 fails**: re-verify PR #55's `EntryUpdated` emit position post-`d7ac5e6cf4`; the WS sync layer must still receive an event on streaming-reveal completion
- [ ] **If Phase 17 fails**: stop, re-read `agent_panel.rs::ensure_thread_initialized`, restore the cfg-gated early return as the FIRST statement of the `BaseView::Uninitialized` branch, rebuild, re-run E2E. Do not mark the task complete with Phase 17 failing
- [ ] If any other phase fails: diagnose root cause, fix, document in `portingguide.md`, re-run

## Update `portingguide.md` (incremental, not at the end)

- [ ] Each conflict resolution appended live (upstream change / resolution / why / risk)
- [ ] New top-level `## Merge 002077 (2026-06-12)` section created at the top of the merge-history list, mirroring 002029-extension round 2 structure
- [ ] **Mandatory subsection**: "`d7ac5e6cf4` Preserve waiting tool call status — PR #55 emit + Critical Fix #6 invariant" — document post-merge emit location, confirm exactly-once `Stopped`
- [ ] **Mandatory subsection**: "Compaction cluster (`e5052961af` et al.) — WS payload schema check" — record whether the cluster added new payload fields
- [ ] **Mandatory subsection**: "`620ceaaaca` Flush-on-quit — Helix WS-authoritative store interaction" — record the reachability analysis and any `not(external_websocket_sync)` gate
- [ ] **Mandatory subsection**: "`215ca2fb0b` Typed workspace errors — Helix `show_error` call-site migration" — list each call site and chosen migration approach
- [ ] **Mandatory subsection**: "`116e4bc184` Inherit source agent without draft content vs Helix PR #56 Fix 1b" — confirm first-statement position
- [ ] **Mandatory subsection**: "`27191913e9` + `0bc6c76fcf` Token usage changes — WS schema check"
- [ ] **Mandatory subsection**: "PR #60 (`27e8867c9e`/`e4c36d837c`) `ede_diagnostic` retry-loop — survival check" — confirm retry block intact, document any new event path that bypasses it
- [ ] Subsection (conditional): "`89cac4944d` Sandbox write-path + `9baefe701e` auto_compact — settings field coexistence with Helix"
- [ ] Any "Pre-existing Breakage Repaired" subsections written for build fixes
- [ ] Commit-history table at bottom of `portingguide.md` extended with this merge's commits and any follow-up fix commits
- [ ] Rebase checklist extended only with **net-new** fragilities discovered in this window (do not invent updates)
- [ ] Stale guide entries discovered along the way are corrected or deleted

## Re-merge Fork Main (only if needed)

- [ ] Check whether `origin/main` advanced during merge work: `git fetch origin && git log feature/002077-merge-latest-zed..origin/main`
- [ ] If yes: `git merge origin/main` into the feature branch, re-build, re-run E2E. (PR #60 demonstrated active WS-sync-layer development during the planning window; not unlikely.)

## Finalise

- [ ] Push feature branch to Zed remote: `git push -u origin feature/002077-merge-latest-zed`
- [ ] Write `pull_request_zed.md` in this task directory with summary of upstream changes (highlight: compaction cluster, `d7ac5e6cf4` tool-call-status, `215ca2fb0b` typed errors, `116e4bc184` source-agent inheritance), conflict resolutions, and validation results (Phase 9, 15, 17 all green)
- [ ] In `/home/retro/work/helix/`, create branch `feature/002077-merge-latest-zed`, bump `ZED_COMMIT` in `sandbox-versions.txt` from `79b9bfb1d60cbef5b14ba7e3992ba5e8f6eb335c` to the new Zed merge HEAD, commit. **Note**: this bump also ships PR #60's retry loop, which was never bumped into the sandbox after #60 merged
- [ ] Push the Helix branch: `git push -u origin feature/002077-merge-latest-zed`
- [ ] Write `pull_request_helix.md` in this task directory — call out that PR #60 is included in this bump (was not in the previous `ZED_COMMIT`)
- [ ] Do NOT force-push `main` (Zed or Helix) without explicit user approval
- [ ] Do NOT open PRs from the agent — the Helix UI handles PR creation per task convention
