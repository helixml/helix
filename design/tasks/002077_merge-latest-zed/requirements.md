# Requirements: Merge Latest Zed Upstream (002077)

## Context

Today is **2026-06-12**. The Helix fork of Zed last merged `zed-industries/zed` via **task 002029** (PR `helixml/zed#58`, merge commit `79b9bfb1d6`, integrating upstream `9d50bab893` "git_ui: Add total diff stats to git panel (#58018)") on **2026-06-02**. Since then **PR #60** ("fix(external_websocket_sync): retry follow-up sends past claude-agent-acp drain race") has landed on fork main with two commits in `crates/external_websocket_sync/src/thread_service.rs`.

Confirmed at runtime on `/home/retro/work/zed` on 2026-06-12:

| | Commit | Date |
|---|---|---|
| Fork HEAD (`origin/main`) | `ecdc2ea67d` ("Merge pull request #60 from helixml/fix/thread-service-claude-acp-drain-retry") | 2026-06-09 |
| Helix `sandbox-versions.txt` `ZED_COMMIT` | `79b9bfb1d6` (**2 Helix-only commits behind fork HEAD** — PR #60's retry-loop + cleanup are not yet shipped to the helix sandbox image) | 2026-06-02 |
| Last upstream merge fence | `9d50bab893` (002029-extension round 2) | 2026-06-02 |
| Upstream HEAD (`upstream/main`) | `992f395c3d` ("editor: Fix columnar selection alignment on rows with multi-byte chars (#57097)") | 2026-06-12 |
| Helix-only commits since 002029 | **2** (`27e8867c9e` retry, `e4c36d837c` cleanup — both in `external_websocket_sync/src/thread_service.rs`) | 10 days |
| Upstream commits to merge (`9d50bab893..upstream/main`) | **256** | 10 days |
| `upstream` git remote configured locally | **No** in fresh clones — must be added |

### Note: task 002059 was planned but never executed

The intervening task `002059_merge-latest-zed/` exists in helix-specs with requirements/design/tasks files, but no `feature/002059-merge-latest-zed` branch ever appeared on `origin`. Treat 002077 as the next direct upstream-catch-up after 002029.

### Helix PRs landed since 002029 (must survive verbatim)

| Merge commit | PR | Touches | Description |
|---|---|---|---|
| `27e8867c9e` | #60 | `crates/external_websocket_sync/src/thread_service.rs` | `handle_follow_up_message` retries the send up to 4 attempts × 750ms backoff when the `claude-agent-acp` wrapper returns its post-cancel `ede_diagnostic` transient. Phase 9 of the E2E provoked this race deterministically; the retry resolves it. Same race class as the `acp_thread::AcpThread::cancel` workaround. |
| `e4c36d837c` | #60 | `crates/external_websocket_sync/src/thread_service.rs` | Cleanup: drops the unreachable `last_err` accumulator that the prior commit's `return Err(e)` already made redundant. 6-line deletion, no behaviour change. |

### Risk profile: HIGH (upgraded from MEDIUM in the 2026-06-08 snapshot — diff sizes have nearly tripled in 4 days)

This merge has grown from a small catch-up to a substantial integration. Upstream diff sizes against Helix-touched files (`9d50bab893..upstream/main`, refreshed 2026-06-12):

| File | Commits | +/- lines | Risk | Concrete concern |
|---|---|---|---|---|
| `crates/acp_thread/src/acp_thread.rs` | **12** | **+1102 / -81** | **VERY HIGH** | The state machine containing Critical Fixes #6, #8, #9 (cancel/Stopped invariants) and PR #55's streaming-reveal `EntryUpdated` emit just absorbed ~1000 lines of upstream change. `d7ac5e6cf4` "Preserve waiting tool call status on updates (#58537)" alone is +602 lines (across 6 files) and reworks how `ToolCall` status updates flow through the entry-update path — direct overlap with PR #55's emit site. Multiple compaction-related commits also touch this file. Inspect every conflict; don't trust auto-merges. |
| `crates/agent/src/agent.rs` | **12** | **+765 / -199** | **HIGH** | Critical Fix #1 (entity-lifetime clone in `load_session`) and `wait_for_tools_ready` (must use `cx.background_executor().timer()`) live here. New compaction cluster lands here: `e5052961af` "/compact slash command" (+1065 lines across 6 files), `9baefe701e` "auto_compact agent settings", `e17e272d24` "compaction UX", `5c90b0664f` "fix race where compaction would be marked as cancelled" (+97 lines), `620ceaaaca` "flush thread content to database on app quit" (+103). All exercise the same `Thread` state machine PR #50 and Fix #1 sit on top of. `27191913e9` cumulative-token-usage accumulation (carried from 06-08 snapshot) compounds the schema-drift risk. |
| `crates/agent_ui/src/agent_panel.rs` | **10** | **+731 / -305** | **HIGH** | `116e4bc184` "Inherit source agent without draft content (#58636)" still in play. New: `c486f6f529` "Iterate on title display on the toolbar (#59162)" + `638b33ca2b` "placeholder title in empty-state toolbar (#59126)" + `c78bd36fd8` "Keep pending subagent edits when regenerating a prompt (#59060)" all touch agent-panel surface. Fix 1b's `#[cfg(feature = "external_websocket_sync")] { return; }` guard must remain the FIRST statement inside `BaseView::Uninitialized` of `ensure_thread_initialized`. Critical Fix #11 entity-identity guard (now `thread_id`-based after 002029) at the top of `load_agent_thread` must survive. |
| `crates/agent_ui/src/conversation_view.rs` | **11** | **+334 / -8** | **MEDIUM-HIGH** | Signature-drift magnet for `from_existing_thread()` — repaired in 002029, 002029-extension, and 002029-extension round 2. Expect a fourth round. Compaction UI commits (`0e9e8d0e68` refinements, `e17e272d24` UX) plus `8432a26a9d` "thinking toggle button fix" all land here. |
| `crates/workspace/src/workspace.rs` | **9** | **+430 / -102** | **MEDIUM-HIGH** | `215ca2fb0b` "Typed workspace errors (#57649)" still the dominant concern — Helix `show_error` call sites must migrate to the `<E: WorkspaceError>` generic. `83aa943705` "Fix overflow in error popup (#59185)" is the follow-up to that work and may further constrain the API. `a32999e00b` "Update window title (#58401)" adds `Rc<Cell<EntityId>>` for multi-workspace. `dde7c1c07f` "Add command to reset pane sizes (#59046)" — minor surface. |
| `crates/agent_servers/src/acp.rs` | **3** | **+5 / -92** | LOW | Small upstream cleanup. PR #50 `session_creation_chain` + `_settings_subscription` (002029-round-2) should survive cleanly. `56b71271c4` "Enable ACP session usage and deletion features" still requires the trait-default check. |
| `crates/zed/src/main.rs` | **3** | **+11 / -14** | LOW | `--allow-multiple-instances`, `--headless`, `build_application(headless: bool)` (002029-round-2 pattern) must survive. |
| `crates/title_bar/` | 4 | small | LOW | `external_websocket_sync = { workspace = true, optional = true }` dep + cfg-gated `render_restricted_mode` override must survive. |
| `crates/external_websocket_sync/src/thread_service.rs` | 0 (Helix-only) | 0 upstream | LOW | **PR #60 retry loop in `handle_follow_up_message` (4×750ms backoff for `ede_diagnostic` transient) is load-bearing** — must not be lost in any cleanup. No upstream churn this window. |
| `crates/agent_settings/src/agent_settings.rs` + `crates/settings_content/src/agent.rs` | 2+ | small | LOW-MED | `89cac4944d` extends `sandbox_permissions`; new `9baefe701e` adds `auto_compact` settings field. Three-way coexistence with Helix's `show_onboarding` / `auto_open_panel` — expect a "both sides added a field" trivial merge. |
| `crates/extensions_ui/src/extensions_ui.rs` | 1+ | small | LOW | `// HELIX: External agent ...` bypass markers at ~221, ~243, ~1513 must survive. |
| `crates/feature_flags/src/flags.rs` | small | small | LOW | `AcpBetaFeatureFlag::enabled_for_all() -> true` override must survive. May see new flags from `88e5c6d2fa` "Don't offer feature-flag-gated tools in agent profiles (#58581)" and `d24b14a26c` "Fix handoff feature flag tests (#58916)". |
| `Cargo.lock` | always | always | TRIVIAL | `--theirs`. |

### Specific highest-risk upstream commit: `d7ac5e6cf4` "acp_thread: Preserve waiting tool call status on updates (#58537)" — NEW since 06-08

+602 lines across 6 files including the core `acp_thread.rs`. Reworks how `ToolCall::status` propagates through entry updates. **Direct overlap with PR #55's 12-line streaming-reveal `EntryUpdated` emit and with Critical Fix #6 (`stopped_emitted_for_task` invariant) territory.** After resolution, `cargo test -p acp_thread test_second_send` is the local invariant check and E2E Phases 8, 9, 15 are the runtime gates.

### Specific high-risk upstream cluster: Compaction (`e5052961af`, `9baefe701e`, `e17e272d24`, `5c90b0664f`, `0bc6c76fcf`, `0e9e8d0e68`) — NEW since 06-08

Six commits introducing the built-in `/compact` slash command, `auto_compact` settings field, compaction UI refinements, and a fix for a compaction-cancellation race. Concentrated in `crates/agent/src/agent.rs` (+1065 lines in `e5052961af` alone) and the agent-settings files. Risks for Helix:

1. **WS payload schema**: compaction emits new event types and modifies `Thread` state. The Helix WS sync layer marshals `Thread` state to the API server. Inspect `external_websocket_sync/` for any new payload fields or schema changes.
2. **`auto_compact` settings field**: another "both sides added a field" three-way coexistence with `show_onboarding` / `auto_open_panel` / `sandbox_permissions`.
3. **Compaction race fix (`5c90b0664f`)**: +97 lines patching a "compaction marked as cancelled" race. Critical Fixes #6/#8/#9 (cancel/Stopped invariants) are in the same family — verify the upstream race-fix doesn't conflict with Helix's invariant.
4. **Token-usage hiding (`0bc6c76fcf`)**: combines with `27191913e9` (cumulative token usage) to change what tokens flow through the UI/WS layers post-compact.

### Specific high-risk upstream commit: `620ceaaaca` "agent: Flush thread content to database on app quit (#58962)" — NEW since 06-08

+103 lines in `agent/src/agent.rs`. Adds shutdown-time persistence to `threads.db`. Risk for Helix: the Helix WS sync layer is the authoritative store; if the `flush-on-quit` path runs in `external_websocket_sync` builds and stomps on/duplicates WS-managed state, the next session-restart could see split-brain. Verify by inspecting whether the flush path is `external_websocket_sync`-cfg-gated upstream (almost certainly not) and decide whether Helix needs a guard.

### Specific high-risk upstream commit: `215ca2fb0b` "Typed workspace errors (#57649)" — carried from 06-08

Migrates `Workspace::show_error` to a generic `<E: WorkspaceError>` taking a trait-bound type. `83aa943705` "Fix overflow in error popup (#59185)" is a follow-up that may further tighten the API. Helix call sites that currently pass a string or `anyhow::Error` will break the build; each needs migration. Build-driven discovery via `./stack build-zed dev`.

### Specific medium-risk upstream commit: `116e4bc184` "agent_ui: Inherit source agent without draft content (#58636)" — carried from 06-08

Touches the draft-inheritance path that intersects Helix PR #56 Fix 1b. **Re-verify Fix 1b is still the FIRST statement of `BaseView::Uninitialized` after the merge** — Phase 17 is the runtime gate.

### Specific medium-risk upstream commit: `27191913e9` "agent: Cumulative token usage (#58378)" — carried from 06-08

Revives `Thread::cumulative_token_usage` accumulation + persistence to `threads.db`. Now compounded by the compaction cluster's token-handling changes (`0bc6c76fcf`). Schema-drift check on WS payloads is mandatory.

### Specific medium-risk upstream commit: `56b71271c4` "acp: Enable ACP session usage and deletion features (#58680)" — carried from 06-08

Stabilises context windows + persistent session/delete for ACP agents. Confirm no default-flip the Helix `AcpConnection` impl needs to override.

### Specific medium-risk upstream commit: `fef979dec4` "language_models: Add Anthropic-compatible provider support in settings (#50381)" — NEW since 06-08

Settings/provider plumbing. Low conflict risk directly, but Helix's enterprise-TLS-skip and built-in-agent-hiding patches sit in adjacent files; re-grep after merge.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to merge the latest 256 upstream commits into the Helix fork to absorb 10 days of upstream work in one sitting — particularly the compaction cluster (six commits, ~1700 net lines), `d7ac5e6cf4` tool-call-status preservation (+602 lines in the acp_thread state machine), `215ca2fb0b` typed workspace errors (forces `show_error` call-site migration), and the lingering `116e4bc184` source-agent draft inheritance (Fix 1b position re-verification).

### 2. Helix User
> As a Helix user, I want the new upstream improvements (built-in `/compact` slash command, `auto_compact` setting, compaction UX, tool-call-status preservation, typed workspace errors, Anthropic-compatible provider, file-link handling in agent threads) without losing WebSocket sync, draft suppression, headless mode, the PR #60 `claude-agent-acp` drain-race retry, or any of the 10 active Critical Fixes.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want `portingguide.md` updated **as work proceeds** — including a dedicated subsection on how PR #55's `EntryUpdated` emit + Critical Fix #6's `stopped_emitted_for_task` survived `d7ac5e6cf4`'s 602-line tool-call-status rewrite, whether `620ceaaaca` flush-on-quit interacts with Helix's WS-authoritative thread store, whether the compaction cluster introduced new WS payload fields, and where `Workspace::show_error` call sites were migrated to the typed-error API — so the next rebase checklist reflects current reality.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD (`992f395c3d` or newer; any skipped commits explicitly justified in `portingguide.md`)
- [ ] No upstream commits silently cherry-picked out
- [ ] **PR #60's retry loop in `handle_follow_up_message` is preserved verbatim** (no upstream touches `thread_service.rs`, but a careless cleanup could lose it)

### Critical Fix Preservation (the 10 active fixes in `portingguide.md` §"Critical Fixes" — #10 stays retired)
- [ ] Fix #1: `NativeAgent` entity cloned before async task in `load_session()` (survives `620ceaaaca` flush-on-quit + compaction cluster)
- [ ] Fix #2: No duplicate WebSocket sends from `crates/agent_ui/src/acp/thread_view.rs`
- [ ] Fix #3: `content_only()` strips `## Assistant` heading
- [ ] Fix #4: `notify_thread_display()` called for follow-ups to non-visible threads
- [ ] Fix #5: stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6: every `send()` emits exactly one `Stopped` — **survives `d7ac5e6cf4`'s ToolCall-status rewrite + `5c90b0664f`'s compaction-cancel race fix** (cf. `cargo test -p acp_thread test_second_send`)
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on the normal completion path
- [ ] Fix #11: entity-identity guard at top of `load_agent_thread` in `agent_panel.rs` (`thread_id`-based form from 002029)

### Helix-Specific Surface (must survive verbatim or be re-applied with equivalent semantics)
- [ ] `crates/external_websocket_sync/` crate intact (all 10+ source files)
- [ ] **PR #60 `handle_follow_up_message` 4×750ms retry on `ede_diagnostic` transient intact in `thread_service.rs`**
- [ ] WebSocket thread display callback + UI state query callback in `agent_panel.rs::new()`
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] `from_existing_thread()` constructor on `ConversationView` (matches current `ConversationView`/`ThreadView::new` arg list — expect a fourth signature-drift repair)
- [ ] PR #50 `session_creation_chain` field on `AcpConnection` + drop-guard intact, coexisting with `_settings_subscription` (002029-round-2)
- [ ] PR #50 `test_concurrent_session_creation_is_serialized` test passes
- [ ] PR #55 `EntryUpdated` emit after streaming-reveal drain in `acp_thread.rs` preserved through `d7ac5e6cf4`'s rewrite
- [ ] PR #56 Fix 1a deferred `UserCreatedThread` plumbing in `external_websocket_sync` intact
- [ ] PR #56 Fix 1b `#[cfg(feature = "external_websocket_sync")] { return; }` is still the FIRST statement of `BaseView::Uninitialized` branch in `ensure_thread_initialized`
- [ ] PR #57 Phase 16 counter exclusion in `helix-ws-test-server/main.go` intact
- [ ] `fd26c1a113` Dockerfile.ci `helix-org` pull intact
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` override intact
- [ ] Built-in agent hiding (Claude Code, Codex, Gemini hidden in Helix builds)
- [ ] Enterprise TLS skip (`sync_settings`)
- [ ] `--allow-multiple-instances` CLI flag in `crates/zed/src/main.rs`
- [ ] `--headless` CLI flag + `initialize_headless()` + `build_application(headless: bool)` pattern (002029-round-2) intact
- [ ] `rust-embed` workspace dep still has `debug-embed` feature
- [ ] `wait_for_tools_ready` uses `cx.background_executor().timer()` (not `smol::Timer`)
- [ ] `title_bar` feature propagation chain intact (`optional = true`)
- [ ] `title_bar::render_restricted_mode` cfg-gated early return intact
- [ ] `extensions_ui.rs` `// HELIX: External agent ...` bypass markers retained
- [ ] `BaseView` / `ContextServerStatus` exhaustive matches in Helix-mode code updated for any new variants

### Trait/API Migrations Forced by Upstream
- [ ] `Workspace::show_error` call sites in Helix code migrated to the new generic-over-`WorkspaceError` signature introduced by `215ca2fb0b` + tightened by `83aa943705` — every site documented as "Pre-existing Breakage Repaired"
- [ ] `d7ac5e6cf4` `ToolCall::status`-preservation API surface integrated cleanly with PR #55's `EntryUpdated` emit site — no duplicate emits, no lost status updates
- [ ] `620ceaaaca` `flush-on-quit` path reviewed for interaction with Helix WS-authoritative thread store — either confirmed harmless or gated under `not(feature = "external_websocket_sync")` with documented rationale
- [ ] Compaction-cluster fields (e.g. `auto_compact`) coexist with Helix's `show_onboarding` / `auto_open_panel` in `agent_settings` / `settings_content`
- [ ] If `116e4bc184` or any other upstream commit rewrote `ensure_thread_initialized` body, Fix 1b's early-return is moved to whichever entry point now precedes both `activate_draft` and the source-agent inheritance path
- [ ] If `27191913e9` cumulative-token-usage or the compaction cluster add new fields to `Thread`/`AgentThread`/WS payloads, the schema bump is documented and the Helix API server tolerates additional fields

### Build & Test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — zero errors
- [ ] (If local Rust toolchain available) `cargo check -p zed --features external_websocket_sync` — zero errors
- [ ] (If local Rust toolchain available) `cargo test -p external_websocket_sync` — full pass (≤2 env-dependent ignored acceptable)
- [ ] (If local Rust toolchain available) `cargo test -p acp_thread test_second_send` — passes (Fix #6 invariant against `d7ac5e6cf4`)
- [ ] (If local Rust toolchain available) `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` — passes
- [ ] **External WebSocket sync E2E (Docker, hard gate)** — all 17 phases pass for **both** `zed-agent` and `claude` agents. Contractual minimum:
  - Phases 1–7 (basic sync + non-visible follow-up + UI state + open_thread)
  - Phase 8 (mid-stream interrupt — Stopped invariant)
  - **Phase 9 (rapid 3-turn cancel — PR #60 retry-loop regression gate)**
  - Phase 10 (user_created_thread)
  - Phase 11 (spectask routing)
  - Phase 12 (reconnect)
  - Phase 13 (`cancel_current_turn` happy path — `turn_cancelled` ordering)
  - Phase 14 (`cancel_current_turn` no-op)
  - **Phase 15** (streaming patches arrive incrementally — PR #55 + `d7ac5e6cf4` interaction gate)
  - **Phase 16** (zero spontaneous `UserCreatedThread` emits — PR #56 Fix 1a + PR #57)
  - **Phase 17** (live Claude process count == real thread count — PR #56 Fix 1b regression — **the explicit gate that "draft suppression survived the merge"**)
- [ ] Claude E2E Phase 1 npm-install bootstrap flake (lesson from 001996): one retry permitted

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] Each entry records *what changed upstream*, *what the resolution was*, *why*, *risk*
- [ ] New `## Merge 002077 (2026-06-12)` section appended at the top of the merge-history list
- [ ] Per-conflict subsection in `### Conflicts and Resolutions`
- [ ] `### Pre-existing Breakage Repaired` subsection for every signature-drift / typed-error / new-variant fix
- [ ] Commit-history table at the bottom of `portingguide.md` extended with this merge's commits
- [ ] Rebase checklist extended only with **net-new** fragilities discovered in this window (do not invent updates)
- [ ] Stale guide entries discovered along the way are corrected or deleted

### Process
- [ ] Feature branch `feature/002077-merge-latest-zed` created from current fork main (`ecdc2ea67d`, or newer if anyone pushed meanwhile)
- [ ] Branch name `feature/002059-merge-latest-zed` is **not** reused
- [ ] Branch pushed to `helixml/zed`
- [ ] Helix repo `ZED_COMMIT` bump prepared in `sandbox-versions.txt` (from current `79b9bfb1d6` to the new merge HEAD — this bump will also ship PR #60's retry loop, which was never bumped into the sandbox after #60 merged)
- [ ] Helix branch `feature/002077-merge-latest-zed` pushed
- [ ] `pull_request_zed.md` and `pull_request_helix.md` written into this task directory
- [ ] `main` is **not** force-pushed without explicit user approval
- [ ] The Helix UI handles PR creation — do not open PRs from the agent (per task convention)

## Out of Scope

- Net-new Helix feature development
- Modifying e2e test assertions themselves (unless a legitimate upstream API change strictly requires it)
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix-specific crates beyond what conflict resolution requires
- Wiring upstream's `agent_skills` / `skill_creator` crates into Helix workflows
- Wiring upstream's `/compact` slash command or `auto_compact` setting into Helix-mode UX (out of scope — let the upstream cluster sit; Helix-mode users may or may not benefit, that's a separate decision)
- Adopting upstream's typed-error infrastructure beyond Helix's existing `show_error` call sites (only migrate what the merge forces)
