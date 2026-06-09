# Requirements: Merge Latest Zed Upstream (002077)

## Context

Today is **2026-06-08**. The Helix fork of Zed last merged `zed-industries/zed` via **task 002029** (PR `helixml/zed#58`, merge commit `79b9bfb1d6`, integrating upstream `9d50bab893` "git_ui: Add total diff stats to git panel (#58018)") on **2026-06-02**. There have been **zero Helix-only commits on fork main since then** — the baseline is clean.

Confirmed at runtime on `/home/retro/work/zed` on 2026-06-08:

| | Commit | Date |
|---|---|---|
| Fork HEAD (`origin/main`) | `79b9bfb1d6` ("Merge pull request #58 from helixml/feature/002029-merge-latest-zed") | 2026-06-02 |
| Helix `sandbox-versions.txt` `ZED_COMMIT` | `79b9bfb1d6` (perfectly aligned — no drift) | 2026-06-02 |
| Last upstream merge fence | `9d50bab893` (002029-extension round 2) | 2026-06-02 |
| Upstream HEAD (`upstream/main`) | `3f5705b985` ("extension_ci: Bump extension CLI version to `9ee3c50` (#58785)") | 2026-06-08 |
| Helix-only commits since 002029 | **0** | 6 days |
| Upstream commits to merge (`9d50bab893..upstream/main`) | **139** | 6 days |
| `upstream` git remote configured locally | **No** — must be added (was added in the planning environment to verify divergence; implementation agent re-adds in its own clone) | — |

### Note: task 002059 was planned but never executed

The intervening task `002059_merge-latest-zed/` exists in helix-specs with requirements/design/tasks files, but no `feature/002059-merge-latest-zed` branch ever appeared on `origin` and the planned posture decision (stack on 002029 vs. wait for 002029 to land) was rendered moot when PR #58 landed on 2026-06-02. Treat 002077 as the next direct upstream-catch-up after 002029.

### Risk profile: MEDIUM (down from VERY HIGH in 002029)

This is a small, well-scoped merge: 139 upstream commits over 6 days, with zero Helix-only commits in flight to fold against. The 002029-extension round 2 precedent (242 commits, 4 trivial conflicts, 3 signature-drift repairs) suggests this will be similar in shape.

Upstream diff sizes against Helix-touched files (`9d50bab893..upstream/main`, measured 2026-06-08):

| File | Commits | +/- lines | Risk | Concrete concern |
|---|---|---|---|---|
| `crates/agent_ui/src/agent_panel.rs` | 6 | +612 / -52 | **MEDIUM** | `116e4bc184` "Inherit source agent without draft content (#58636)" touches the activate-draft/draft-inheritance flow — same area as Helix PR #56 Fix 1b. The Fix 1b `#[cfg(feature = "external_websocket_sync")] { return; }` guard must remain the FIRST statement inside the `BaseView::Uninitialized` branch of `ensure_thread_initialized`. |
| `crates/agent/src/agent.rs` | 3 | +257 / -80 | MEDIUM | `27191913e9` "Accumulate and persist cumulative token usage for threads (#58378)" revives `Thread::cumulative_token_usage` accumulation. May add new persistence calls or fields visible to the WebSocket sync layer. |
| `crates/acp_thread/src/acp_thread.rs` | 4 | +184 / -14 | MEDIUM | Critical Fixes #6/#8/#9 (cancel/Stopped state machine) and PR #55 (streaming-reveal `EntryUpdated` emit) live here. Inspect each conflict; do not trust auto-merges. |
| `crates/agent_ui/src/conversation_view.rs` | 4 | +80 / -3 | LOW-MED | Signature-drift magnet for `from_existing_thread()` (the prior two merges both repaired this constructor). `de744e744c` "Correctly handle file links in markdown and agent threads (#56024)" touches this area. |
| `crates/workspace/src/workspace.rs` | 4 | small | MEDIUM | `215ca2fb0b` "Typed workspace errors (#57649)" migrates `Workspace::show_error` to a generic taking a `WorkspaceError` trait impl. Every Helix call site that passes a string/anyhow into `show_error` will break the build. Also `a32999e00b` "Update window title when switching active workspace (#58401)" introduces a shared `Rc<Cell<EntityId>>` across multi-workspace windows — re-verify the `CollaboratorId::Agent` follow-focus guard is unaffected. |
| `crates/agent_settings/src/agent_settings.rs` + `crates/settings_content/src/agent.rs` | 1+ | small | LOW-MED | `89cac4944d` "Improve sandbox write-path handling (#58283)" extends the `sandbox_permissions` plumbing that was added in 002029-round-2 alongside Helix's `show_onboarding` / `auto_open_panel`. Three-way coexistence expected to be a "both sides added a field" trivial merge. |
| `crates/agent_servers/src/acp.rs` | 2 | +4 / -91 | LOW | Small upstream cleanup. PR #50 `session_creation_chain` + `_settings_subscription` (002029-round-2) should survive cleanly. |
| `crates/zed/src/main.rs` | 2 | small | LOW | `--headless` / `--allow-multiple-instances` flags must survive. The 002029-round-2 `build_application(headless: bool)` parameter pattern is now in fork main; re-verify it persists. |
| `crates/title_bar/` | 4 | small | LOW | `external_websocket_sync = { workspace = true, optional = true }` dep + cfg-gated `render_restricted_mode` override must survive. |
| `crates/extensions_ui/src/extensions_ui.rs` | 1 | small | LOW | `// HELIX: External agent ...` bypass markers at lines ~221, ~243, ~1513 (confirmed retained in 002029); re-grep. |
| `crates/feature_flags/` | 0 | none | TRIVIAL | No upstream churn this window; `AcpBetaFeatureFlag::enabled_for_all() -> true` safe. |
| `Cargo.lock` | always | always | TRIVIAL | `--theirs`. |

### Specific high-risk upstream commit: `215ca2fb0b` "Typed workspace errors (#57649)"

Migrates `Workspace::show_error` to a generic `<E: WorkspaceError>` taking a trait-bound error type instead of the prior `&anyhow::Error`/`&str`. Helix calls `show_error` in at least the WebSocket sync layer's error reporting and possibly `external_websocket_sync/src/thread_service.rs`. **Every Helix call site needs migration to the new signature.** The build will surface them; document each in the porting guide as a "Pre-existing Breakage Repaired" entry.

### Specific medium-risk upstream commit: `116e4bc184` "agent_ui: Inherit source agent without draft content (#58636)"

Touches the draft-inheritance path that intersects Helix PR #56 Fix 1b. The fix description ("early `?` return on an option, fixed by removing it") suggests the patch rewrites the early-return logic in the draft path. **Re-verify Fix 1b's `#[cfg(feature = "external_websocket_sync")] { return; }` is still the FIRST statement of `BaseView::Uninitialized` after the merge** — Phase 17 of the E2E is the runtime gate.

### Specific medium-risk upstream commit: `27191913e9` "agent: Accumulate cumulative token usage (#58378)"

Revives `Thread::cumulative_token_usage` field/accumulation that has been dead code since the agent2 rewrite. Adds accumulation logic on `request_completed`/`tool_finished` paths and persistence to `threads.db`. Risk: if the WebSocket sync layer marshals `Thread` state, the accumulated counter may now appear in events it didn't before. Inspect `external_websocket_sync/src/thread_service.rs` for `cumulative_token_usage` and confirm no payload schema breaks.

### Specific medium-risk upstream commit: `56b71271c4` "acp: Enable ACP session usage and deletion features (#58680)"

Stabilizes context windows and persistent session/delete for ACP agents that support them. May flip a feature-flag gate or change a default that affects Helix's `AcpConnection` impl (`session_creation_chain` field, `supports_delete(&self, &App)` override). Walk the trait impl and confirm no Helix override needs updating.

### Specific medium-risk upstream commit: `89cac4944d` "Improve sandbox write-path handling (#58283)"

Follow-up to `#57972` (sandbox permissions, absorbed in 002029-round-2). Touches `agent_settings/src/agent_settings.rs` and `settings_content/src/agent.rs` — the files where Helix's `show_onboarding` and `auto_open_panel` fields coexist with upstream's `sandbox_permissions`. Expected to be a "both sides added a field" trivial three-way coexistence; verify by inspecting the post-merge struct layouts.

### Specific low-risk upstream commit: `a32999e00b` "workspace: Update window title when switching active workspace (#58401)"

Adds a shared `Rc<Cell<EntityId>>` between member workspaces of a multi-workspace window for tracking the active workspace id. Helix's `CollaboratorId::Agent` follow-focus guard sits in the same file (`crates/workspace/src/workspace.rs`); re-grep after the merge and confirm it's unaffected.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to merge the latest 139 upstream commits into the Helix fork to stay current — particularly absorbing `215ca2fb0b` "Typed workspace errors" (which will force a compile-driven migration of all Helix `show_error` call sites) and `116e4bc184` "Inherit source agent without draft content" (which touches the same draft path as Helix PR #56 Fix 1b).

### 2. Helix User
> As a Helix user, I want the new upstream improvements (typed workspace errors, ACP session deletion/usage features stabilised, cumulative token usage accumulation, sandbox write-path hardening, file-link handling in agent threads) without losing WebSocket sync, draft suppression, headless mode, or any of the Critical Fixes.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want `portingguide.md` updated **as work proceeds** — including a dedicated subsection on whether `215ca2fb0b` typed workspace errors required new Helix `WorkspaceError` impls (and where they live), whether `27191913e9`'s revived `cumulative_token_usage` accumulation affects any WS sync payload, and whether `116e4bc184`'s draft-inheritance change required moving Fix 1b's guard — so the next rebase checklist reflects current reality.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD (`3f5705b985` or newer; any skipped commits explicitly justified in `portingguide.md`)
- [ ] No upstream commits silently cherry-picked out
- [ ] Since there are no Helix-only commits since 002029, no Helix-only preservation work is required at this layer — but every Helix patch documented in `portingguide.md` must remain functional after the merge

### Critical Fix Preservation (the 10 active fixes in `portingguide.md` §"Critical Fixes" — #10 stays retired)
- [ ] Fix #1: `NativeAgent` entity cloned before async task in `load_session()`
- [ ] Fix #2: No duplicate WebSocket sends from `crates/agent_ui/src/acp/thread_view.rs`
- [ ] Fix #3: `content_only()` strips `## Assistant` heading
- [ ] Fix #4: `notify_thread_display()` called for follow-ups to non-visible threads
- [ ] Fix #5: stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6: every `send()` emits exactly one `Stopped` (cf. `cargo test -p acp_thread test_second_send`)
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on the normal completion path
- [ ] Fix #11: entity-identity guard at top of `load_agent_thread` in `agent_panel.rs` (`thread_id`-based lookup form from 002029)

### Helix-Specific Surface (must survive verbatim or be re-applied with equivalent semantics)
- [ ] `crates/external_websocket_sync/` crate intact (all 10+ source files)
- [ ] WebSocket thread display callback + UI state query callback in `agent_panel.rs::new()`
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] `from_existing_thread()` constructor on `ConversationView` (and matches current `ConnectedServerState`/`ThreadView::new` arg list — every previous merge required a signature-drift repair here; expect another)
- [ ] PR #50 `session_creation_chain` field on `AcpConnection` + drop-guard intact, coexisting with `_settings_subscription` (002029-round-2)
- [ ] PR #50 `test_concurrent_session_creation_is_serialized` test passes
- [ ] PR #55 `EntryUpdated` emit after streaming-reveal drain in `acp_thread.rs` preserved
- [ ] PR #56 Fix 1a deferred `UserCreatedThread` plumbing in `external_websocket_sync` intact
- [ ] PR #56 Fix 1b `#[cfg(feature = "external_websocket_sync")] { return; }` is still the FIRST statement of `BaseView::Uninitialized` branch in `ensure_thread_initialized`
- [ ] PR #57 Phase 16 counter exclusion in `helix-ws-test-server/main.go` intact
- [ ] `fd26c1a113` Dockerfile.ci `helix-org` pull intact
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` override intact
- [ ] Built-in agent hiding (Claude Code, Codex, Gemini hidden in Helix builds)
- [ ] Enterprise TLS skip (`sync_settings`)
- [ ] `--allow-multiple-instances` CLI flag in `crates/zed/src/main.rs`
- [ ] `--headless` CLI flag + `initialize_headless()` + the `build_application(headless: bool)` parameter pattern from 002029-round-2 intact
- [ ] `rust-embed` workspace dep still has `debug-embed` feature
- [ ] `wait_for_tools_ready` uses `cx.background_executor().timer()` (not `smol::Timer`)
- [ ] `title_bar` feature propagation chain intact (`optional = true`)
- [ ] `title_bar::render_restricted_mode` cfg-gated early return intact
- [ ] `extensions_ui.rs` `// HELIX: External agent ...` bypass markers retained (lines ~221, ~243, ~1513)
- [ ] `BaseView` and `ContextServerStatus` exhaustive matches in Helix-mode code (agent_panel.rs UI state callback, main.rs headless responder) updated for any new variants added upstream this window

### Trait/API Migrations Forced by Upstream
- [ ] `Workspace::show_error` call sites in Helix code migrated to the new generic-over-`WorkspaceError` signature introduced by `215ca2fb0b` — every site documented as "Pre-existing Breakage Repaired"
- [ ] If `116e4bc184` "Inherit source agent without draft content" or any other upstream commit renames/restructures `activate_draft` / `ensure_thread_initialized` / the `BaseView::Uninitialized` branch body, Fix 1b's early-return is moved to whichever entry point now precedes both `activate_draft` AND the source-agent inheritance path
- [ ] If `27191913e9` "Accumulate cumulative token usage" adds new fields to `Thread`/`AgentThread`/the persistence payload that flow through WS sync, the WS sync payload schema is verified unchanged (or the schema bump is documented)

### Build & Test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — zero errors
- [ ] (If local Rust toolchain available) `cargo check -p zed --features external_websocket_sync` — zero errors
- [ ] (If local Rust toolchain available) `cargo test -p external_websocket_sync` — full pass (≤2 env-dependent ignored acceptable)
- [ ] (If local Rust toolchain available) `cargo test -p acp_thread test_second_send` — passes
- [ ] (If local Rust toolchain available) `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` — passes
- [ ] **External WebSocket sync E2E (Docker, hard gate)** — all 17 phases pass for **both** `zed-agent` and `claude` agents. Contractual minimum (matches 002029):
  - Phase 1–7 (basic sync + non-visible follow-up + UI state + open_thread)
  - Phase 8 (mid-stream interrupt — Stopped invariant)
  - Phase 9 (rapid 3-turn cancel)
  - Phase 10 (user_created_thread)
  - Phase 11 (spectask routing)
  - Phase 12 (reconnect)
  - Phase 13 (`cancel_current_turn` happy path — `turn_cancelled` ordering)
  - Phase 14 (`cancel_current_turn` no-op)
  - **Phase 15** (streaming patches arrive incrementally — PR #55)
  - **Phase 16** (zero spontaneous `UserCreatedThread` emits — PR #56 Fix 1a + PR #57)
  - **Phase 17** (live Claude process count == real thread count — PR #56 Fix 1b regression — **the explicit gate that "draft suppression survived the merge"**)
- [ ] Claude E2E Phase 1 npm-install bootstrap flake (lesson from 001996): one retry permitted

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] Each entry records *what changed upstream*, *what the resolution was*, *why*, *risk*
- [ ] New `## Merge 002077 (2026-06-08)` section appended at the top of the merge-history list, mirroring the 002029-extension round 2 structure
- [ ] Per-conflict subsection in `### Conflicts and Resolutions`
- [ ] `### Pre-existing Breakage Repaired` subsection for every signature-drift / typed-error / new-variant fix
- [ ] Commit-history table at the bottom of `portingguide.md` extended with this merge's commits
- [ ] Rebase checklist extended only with **net-new** fragilities discovered in this window (do not invent updates)
- [ ] Stale guide entries discovered along the way are corrected or deleted

### Process
- [ ] Feature branch `feature/002077-merge-latest-zed` created from current fork main (`79b9bfb1d6`, or newer if anyone pushed meanwhile)
- [ ] Branch name `feature/002059-merge-latest-zed` is **not** reused (that task was planned but never executed; reusing the name would be confusing — even though no branch was ever pushed, the helix-specs directory exists)
- [ ] Branch pushed to `helixml/zed`
- [ ] Helix repo `ZED_COMMIT` bump prepared in `sandbox-versions.txt` (from `79b9bfb1d6` to the new merge HEAD), branch `feature/002077-merge-latest-zed` pushed to Helix
- [ ] `pull_request_zed.md` and `pull_request_helix.md` written into this task directory
- [ ] `main` is **not** force-pushed without explicit user approval
- [ ] The Helix UI handles PR creation — do not open PRs from the agent (per task convention)

## Out of Scope

- Net-new Helix feature development
- Modifying e2e test assertions themselves (unless a legitimate upstream API change strictly requires it)
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix-specific crates beyond what conflict resolution requires
- Wiring upstream's `agent_skills` / `skill_creator` crates into Helix workflows (still out of scope — Helix-mode users don't see Skills)
- Adopting upstream's typed-error infrastructure beyond Helix's existing `show_error` call sites (only migrate what the merge forces)
