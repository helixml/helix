# Requirements: Merge Latest Zed Upstream (002029)

## Context

Today is **2026-05-21**. The Helix fork of Zed last merged with `zed-industries/zed` via **task 001996** (merge commit `8841edb2b1`, integrating upstream `8bdd78e023`) on **2026-05-11**. PR #54 landed on fork main and four further Helix-only PRs (#50, #55, #56, #57) plus one direct Dockerfile.ci fix have shipped since.

Confirmed at run time on `/home/retro/work/zed` on 2026-05-21:

| | Commit | Date |
|---|---|---|
| Fork HEAD (`origin/main`) | `fd26c1a113` ("fix: pull helix-org into ci container") | 2026-05-21 |
| Helix `sandbox-versions.txt` `ZED_COMMIT` | `b35224530f` (1 commit behind fork HEAD — only the Dockerfile.ci fix is missing) | — |
| Last upstream merge | `8841edb2b1` (task 001996, integrated `8bdd78e023`) | 2026-05-11 |
| Upstream HEAD (`upstream/main`) | `1399540715` ("settings_ui: Display scope in the breadcrumb (#57437)") | 2026-05-21 |
| Helix-only commits since 001996 | **15** (4 PRs: #50, #55, #56, #57 + 1 direct CI fix `fd26c1a113`) | 10 days |
| Upstream commits to merge (`8bdd78e023..upstream/main`) | **261** | 10 days |

### Helix PRs landed since 001996 (must survive verbatim)

| Merge commit | PR | Touches | Description |
|---|---|---|---|
| `50c4308fcb` | #50 | `crates/agent_servers/src/acp.rs` | Serialize ACP `session/new` & `session/load` per `AcpConnection` via `session_creation_chain`. Prevents two `claude-agent-acp` children racing on the npm `_npx` cache. |
| `cd4e279d80` | #55 | `crates/acp_thread/src/acp_thread.rs` (12 lines) | Emit `EntryUpdated` after streaming-reveal drain so WS sync sees fresh content. Phase 15 streaming-cadence gate. |
| `62cd60aaca` | #56 | `crates/external_websocket_sync/`, `crates/agent_ui/src/agent_panel.rs::ensure_thread_initialized` | Fix 1a: defer `UserCreatedThread` emit until first user message. Fix 1b: under `external_websocket_sync` feature, return early from `ensure_thread_initialized` instead of `activate_draft` (draft thread suppression). Adds Phase 16 + Phase 17. |
| `b2f2ebefb6` | #57 | `crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go` | Exclude Phase 10's own `UserCreatedThread` injection from Phase 16 counter (false-positive fix). |
| `fd26c1a113` | direct | `crates/external_websocket_sync/e2e-test/Dockerfile.ci` (2 lines) | Pull `helix-org` into CI container. Cosmetic CI infra — no merge risk. |

### Risk profile: VERY HIGH (escalated from HIGH on the 2026-05-18 plan — diff sizes have grown substantially)

Upstream diff sizes against Helix-touched files (`8bdd78e023..upstream/main`, refreshed 2026-05-21):

| File | Diff lines | Risk | Concrete concern |
|---|---|---|---|
| `crates/agent_ui/src/agent_panel.rs` | **12 501** (was 3 156 on 2026-05-18) | **VERY HIGH** | Now driven by FOUR overlapping upstream PRs in the entry-point area: (1) `bbe23cc40b` "Bring back draft threads" (1 256 lines), (2) `589dc95c87` "Restore last active agent panel entry" (684 lines, **modifies `ensure_thread_initialized` directly** — adds `pending_terminal_spawn` + `should_create_terminal_for_new_entry` branches before the `BaseView::Uninitialized` body), (3) `c84c22dab5` "Deprecate and migrate ACP extensions" (55 lines), (4) `e2c38b5358` "Replace Rules UI with Skills creation UI". Plus terminal-as-thread, mermaid, font-size propagation, agent-action gating in terminal menus. **Helix PR #56 Fix 1b's `#[cfg(feature = "external_websocket_sync")] { return; }` block lives at exactly the line `589dc95c87` rewrites.** |
| `crates/agent_servers/src/acp.rs` | **1 086** | **HIGH** | Three concurrent upstream changes plus Helix PR #50: (1) `23231879cd` "ACP session deletion" (+161 lines), (2) `f2df3f9e18` "ACP logout" (+108), (3) `c3951af24f` "Support additional session directories" (+489, **expanding `new_session` / `load_session` / `open_or_create_session` — the exact entry points PR #50's `session_creation_chain` chains**). Three-way fold required. |
| `crates/agent/src/agent.rs` | **2 339** | **HIGH** | `NativeAgent` and `NativeAgentConnection` heavily refactored upstream. Critical Fix #1 (entity-lifetime in `load_session`) and `wait_for_tools_ready` live here. `2e70059cd9` Skills feature flag removal drops 63 lines here; `800706d7a8` update-title-tool adds. |
| `crates/agent_ui/src/conversation_view.rs` | **893** | HIGH | `bbe23cc40b` draft threads, `bf423dfc45` ACP thread rename, `2301e61d2a` mermaid, `4074eabb3d` images, `1c884d13d3` terminal notifications, `f2df3f9e18` ACP logout (90-line addition) all touch this file. Helix `from_existing_thread()` lives here. |
| `crates/acp_thread/src/acp_thread.rs` | 404 | MEDIUM | PR #55's new 12-line `EntryUpdated` emit lives here; overlaps `2301e61d2a` mermaid and `495f8ba717` checkpoint-emit-on-edit. |
| `crates/acp_thread/src/connection.rs` | 82 | MEDIUM | **Confirmed at runtime**: in upstream HEAD the `AgentSessionList::supports_delete` signature is now `supports_delete(&self, &App)`; new `supports_logout(&self, &App) -> bool` and `logout(&self, &mut App) -> Task<Result<()>>` default impls added to `AgentConnection`. Helix HEAD still has the old single-arg `supports_delete(&self)`. **Compile-driven migration required across all impls and call sites.** |
| `crates/agent_ui/src/acp/thread_history.rs` | TBD | MEDIUM | Confirmed 10 `supports_delete` references at runtime (impl on line 362; call sites on 365, 563, 574, 700, 797; struct field/builder on 868, 879, 884–885). Every one needs the `cx` parameter threaded after upstream's signature change. |
| `crates/extensions_ui/src/extensions_ui.rs` | 82 (almost entirely DELETIONS) | MEDIUM | Upstream `c84c22dab5` "Deprecate ACP extensions" removes 80 lines — exactly the region containing Helix's `// HELIX: External agent keywords removed` (line 245) and `// HELIX: External agent upsells removed` (line 1586) bypass comments. The Helix bypass becomes a no-op silently if the surrounding extension surface is gone; verify the deletions still cover the same intent. |
| `crates/feature_flags/src/flags.rs` | small | LOW | `AcpBetaFeatureFlag` confirmed still present upstream (line 22). `2e70059cd9` removes `SkillsFeatureFlag`; `a7f037d94b` removes `AgentPanelTerminalFeatureFlag` and `ExperimentalSystemPromptFeatureFlag`; `800706d7a8` adds `UpdateTitleToolFeatureFlag`. Helix's `enabled_for_all()` override on `AcpBetaFeatureFlag` is unaffected. |
| `crates/title_bar/` | small | LOW | 6 upstream PRs touch this crate; the Helix `external_websocket_sync` cfg-gated icon + `optional = true` dep must survive. |
| `crates/workspace/src/workspace.rs` | 128 | LOW | Agent follow focus guard (`CollaboratorId::Agent` must not steal keyboard focus). |
| `crates/zed/src/main.rs` | 196 | LOW | `--allow-multiple-instances`, `--headless` flags must survive. |
| `Cargo.toml` (workspace) | 135 | LOW | `rust-embed`'s `debug-embed` feature must survive. |
| `crates/agent_settings/src/agent_settings.rs` | 0 | LOW | No upstream churn in this window; Helix fields safe. |
| `Cargo.lock` | always | TRIVIAL | `--theirs`. |

### Specific highest-risk upstream commit: `589dc95c87` "Restore last active agent panel entry" (NEW since 2026-05-18 plan)

This PR (`#57150`, 2026-05-19) is **the most dangerous single commit in this merge window**. It adds 684 lines to `agent_panel.rs` and **modifies the `ensure_thread_initialized` function itself** — the exact function containing Helix PR #56 Fix 1b's draft-suppression guard. The upstream rewrite turns:

```rust
fn ensure_thread_initialized(&mut self, window: &mut Window, cx: &mut Context<Self>) {
    if matches!(self.base_view, BaseView::Uninitialized) {
        self.activate_draft(false, AgentThreadSource::AgentPanel, window, cx);
    }
}
```

into a multi-branch body that handles `pending_terminal_spawn`, `should_create_terminal_for_new_entry`, and ACP-thread restoration before falling through to `activate_draft`. **The Helix `#[cfg(feature = "external_websocket_sync")] { return; }` guard must move to the very top of the function body, before any of these new branches** — otherwise the panel will create terminal threads / restore ACP threads / spawn drafts in Helix mode before the guard fires.

If the implementation agent silently keeps the guard inside the old `if matches!(BaseView::Uninitialized)` block but loses the early-return semantics for the new branches, Phase 17 of the E2E will start failing intermittently as draft Claudes get spawned through alternative paths.

### Specific second-highest-risk upstream commit: `bbe23cc40b` "Bring back draft threads" (carried from 2026-05-18 plan)

This PR (`#54292`, 2026-05-11) **directly conflicts in intent** with Helix PR #56 (`769a463a2f`). Upstream is doubling down on the draft-thread UX — adding `draft_prompt_store.rs`, multi-draft retention in `retained_threads`, draft "parking" semantics, plus 1 256 lines of churn in `agent_panel.rs` itself. Helix PR #56 specifically *disables* draft-thread spawning under `external_websocket_sync` to stop a duplicate `claude-agent-acp` spawn that brings 180s MCP `initialize` timeouts.

Combined with `589dc95c87` above, the agent panel's draft and entry-restoration lifecycle is now fundamentally different from what Helix PR #56 was written against. The implementation agent **must trace every entry point that can eventually call `activate_draft` or `connection.new_session()` under `external_websocket_sync` and confirm the Helix early-return covers them all.** Phase 17 of the E2E is the runtime safety net but should not be the only check.

### Specific medium-risk upstream commit: `c3951af24f` "Support additional session directories" (NEW since 2026-05-18 plan)

Adds **489 lines** to `crates/agent_servers/src/acp.rs`, expanding the signatures and bodies of `new_session`, `load_session`, and `open_or_create_session` — the exact methods PR #50's `session_creation_chain` wraps. Three-way fold required. The risk is that upstream changes the function signatures (e.g. additional `Vec<PathList>` parameter for extra worktrees) and the chain wrapper passes through the wrong arguments. After resolution, `test_concurrent_session_creation_is_serialized` should remain a hard local check.

### Specific medium-risk upstream commit: `23231879cd` "ACP session deletion"

Adds 161 lines to `crates/agent_servers/src/acp.rs` — same file as PR #50 and `c3951af24f` above. Conflict expected. Cascades the `AgentSessionList::supports_delete` signature change from `(&self)` to `(&self, &App)` through `crates/agent_ui/src/acp/thread_history.rs` (10 references) and `crates/agent/src/agent.rs:1838`.

### Specific medium-risk upstream commit: `f2df3f9e18` "ACP logout support"

Adds `supports_logout`/`logout` default methods to the `AgentConnection` trait (default impls, so not a compile break by itself), plus 90 lines to `conversation_view.rs`, 19 to `agent_panel.rs`, 108 to `agent_servers/src/acp.rs`. Confirm the new logout UI is not surfaced in Helix-mode builds (the default `supports_logout() -> false` should achieve this, but visually verify).

### Specific medium-risk upstream commit: `c84c22dab5` "Deprecate and migrate ACP extensions" (NEW since 2026-05-18 plan)

Removes **1 459 lines** across the agent-server extension surface (`crates/agent_servers/src/custom.rs`, `crates/project/src/agent_server_store.rs`, `crates/extensions_ui/src/extensions_ui.rs`, `crates/extension_host/`, etc.). Two concrete risks for Helix:

1. `extensions_ui.rs` loses 80 lines — exactly the region around Helix's `// HELIX: External agent keywords removed` and `// HELIX: External agent upsells removed` bypass comments. After the merge those Helix comments may be deleted by the merge itself; **verify the upstream deletions still achieve the equivalent of Helix's bypass intent (no agent keyword surfacing, no agent upsells).** If upstream is now just removing the surface entirely, the Helix bypass is redundant and can be dropped — document this in the porting guide rather than re-applying a now-no-op patch.
2. `crates/agent_servers/src/custom.rs` is heavily gutted (116 → ~40 lines). The Helix `ExternalAgent::server()` path uses `CustomAgentServer::new(AgentId(...))` (porting-guide checklist item 14). Confirm this still compiles after the upstream cull.

### New upstream crates (low merge risk, out of scope)

- `crates/agent_skills/` (~2 000 lines), `crates/skill_creator/` (1 073 lines) — new upstream crates. Out of scope for Helix integration; leave them as-is in the workspace.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to merge the latest upstream Zed into the Helix fork so we stay current and minimise future merge debt — particularly given **two** overlapping upstream PRs (`bbe23cc40b` "Bring back draft threads" and `589dc95c87` "Restore last active agent panel entry") that both restructure the exact `ensure_thread_initialized` code path containing Helix PR #56's draft-suppression guard.

### 2. Helix User
> As a Helix user, I want the new upstream Zed editor improvements (terminal-as-thread, mermaid diagrams, draft thread parking, ACP logout, ACP session deletion, additional session directories, agent skills, image attachments, last-active-entry restoration) without losing WebSocket sync or re-experiencing the duplicate-Claude / chrome MCP timeout issue PR #56 just fixed.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want `portingguide.md` updated **as work proceeds** — including a new entry documenting where Fix 1b's draft-suppression guard sits after `589dc95c87`'s rewrite of `ensure_thread_initialized`, the three-way fold of PR #50 with `23231879cd` + `c3951af24f` in `crates/agent_servers/src/acp.rs`, the `supports_delete(&self, &App)` signature migration, and whether Helix's `// HELIX: External agent ...` bypass markers in `extensions_ui.rs` are now obsolete after `c84c22dab5` — so the rebase checklist reflects current reality.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD (`1399540715` or newer; any skipped commits explicitly justified in `portingguide.md`)
- [ ] All Helix-specific commits since 001996 (PRs #50, #55, #56, #57 + the direct `fd26c1a113` Dockerfile.ci fix) are preserved and functional
- [ ] No upstream commits silently cherry-picked out

### Critical Fix Preservation (the 11 fixes in `portingguide.md` §"Critical Fixes")
- [ ] Fix #1: `NativeAgent` entity cloned before async task in `load_session()`
- [ ] Fix #2: No duplicate WebSocket sends from `crates/agent_ui/src/acp/thread_view.rs`
- [ ] Fix #3: `content_only()` strips `## Assistant` heading
- [ ] Fix #4: `notify_thread_display()` called for follow-ups to non-visible threads
- [ ] Fix #5: stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6: every `send()` emits exactly one `Stopped` (cf. `cargo test -p acp_thread test_second_send`)
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on the normal completion path
- [ ] Fix #10: context-server request timeout still 180s
- [ ] Fix #11: entity-identity guard at top of `load_agent_thread` in `agent_panel.rs` (PR #53)

### Helix-Specific Surface (must survive verbatim or be re-applied with equivalent semantics)
- [ ] `crates/external_websocket_sync/` crate intact (all 10+ source files)
- [ ] WebSocket thread display callback + UI state query callback in `agent_panel.rs::new()`
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] `from_existing_thread()` constructor on `ConversationView` (and matches current `ConnectedServerState` field set)
- [ ] Channel-based UI event forwarding (`tokio::sync::mpsc`)
- [ ] `OnboardingUpsell::set_dismissed` Helix-mode UI cleanup path
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true`
- [ ] Built-in agent hiding (Claude Code, Codex, Gemini hidden in Helix builds)
- [ ] Enterprise TLS skip (`sync_settings`)
- [ ] `--allow-multiple-instances` CLI flag still in `crates/zed/src/main.rs`
- [ ] `--headless` CLI flag + `initialize_headless()` + all 3 call sites intact
- [ ] `rust-embed` workspace dep still has `debug-embed` feature
- [ ] `wait_for_tools_ready` uses `cx.background_executor().timer()` (not `smol::Timer`)
- [ ] Feature propagation chain `zed → agent_ui → title_bar` intact (`title_bar` dep `optional = true`)
- [ ] `BaseView::Terminal` arm still exhaustively handled in Helix UI state query (added by 001996; may need a new arm if upstream added another variant)

### Newly Added Helix Behaviour Since 001996 (PRs #50, #55, #56, #57 + `fd26c1a113` — must survive merge)
- [ ] **PR #50** `session_creation_chain` field on `AcpConnection` + drop-guard in `new_session` / `open_or_create_session`. Compatible with upstream `23231879cd`'s ACP session-deletion additions AND `c3951af24f`'s 489-line expansion of the same methods (three-way fold)
- [ ] **PR #50** `test_concurrent_session_creation_is_serialized` test passes
- [ ] **PR #55** `EntryUpdated` emitted after streaming-reveal drain in `acp_thread.rs` (~12 lines)
- [ ] **PR #56 Fix 1a** `defer_user_created_thread_until_first_user_message` behaviour in `external_websocket_sync` intact
- [ ] **PR #56 Fix 1b** the `#[cfg(feature = "external_websocket_sync")]` early-return guard sits at the **very top** of `agent_panel.rs::ensure_thread_initialized` body, **before** the new `pending_terminal_spawn`/`should_create_terminal_for_new_entry` branches introduced by upstream `589dc95c87`. If upstream renames or splits the function, the guard must move to whichever entry point now precedes `activate_draft` AND any other path that can call `connection.new_session()` for the Helix surface.
- [ ] **PR #56** unit tests for deferred `UserCreatedThread` emit still pass
- [ ] **PR #57** Phase 16 counter excludes Phase 10's own `UserCreatedThread` injection (Go test-server change only)
- [ ] **`fd26c1a113`** `crates/external_websocket_sync/e2e-test/Dockerfile.ci` still pulls `helix-org` (the 2-line CI infra fix)

### Trait-Signature Migration (forced by upstream)
- [ ] `AgentSessionList::supports_delete(&self)` → `supports_delete(&self, &App)` propagated through all Helix impls AND call sites: 10 references in `crates/agent_ui/src/acp/thread_history.rs` and 1 impl on `crates/agent/src/agent.rs:1838`
- [ ] New `AgentConnection::supports_logout(&self, &App)` / `logout(&self, &mut App)` default impls accepted as-is (no Helix override needed — defaults return false / Err)
- [ ] Visual verification: Helix-mode agent panel does NOT surface a logout button (defaults from `f2df3f9e18` should achieve this)

### Build & Test (hard gates)
- [ ] `./stack build-zed dev` produces a working binary (`./zed-build/zed`) with zero errors
- [ ] (If local Rust toolchain available) `cargo check -p zed --features external_websocket_sync` — zero errors
- [ ] (If local Rust toolchain available) `cargo test -p external_websocket_sync` — full pass (≤2 env-dependent ignored acceptable)
- [ ] (If local Rust toolchain available) `cargo test -p acp_thread test_second_send` — passes
- [ ] **External WebSocket sync E2E (Docker, hard gate)** — all in-tree phases pass for **both** `zed-agent` and `claude` agents. Contractual minimum:
  - Phase 1–7 (basic sync + non-visible follow-up + UI state + open_thread)
  - Phase 8 (mid-stream interrupt — Stopped invariant)
  - Phase 9 (rapid 3-turn cancel)
  - Phase 10 (user_created_thread)
  - Phase 11 (spectask routing)
  - Phase 12 (reconnect)
  - Phase 13 (`cancel_current_turn` happy path)
  - Phase 14 (`cancel_current_turn` no-op)
  - **Phase 15** (streaming patches arrive incrementally — PR #55 + #56 streaming-cadence regression)
  - **Phase 16** (zero spontaneous `UserCreatedThread` emits — PR #56 Fix 1a regression)
  - **Phase 17** (live Claude process count == real thread count — PR #56 Fix 1b regression — **this is the explicit gate that "draft suppression survived the merge"**)

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] Each entry records *what changed upstream*, *what the resolution was*, *why*, *risk*
- [ ] New `## Merge 002029 (2026-05-21)` section mirroring 001996's structure
- [ ] Commit-history table at the bottom of `portingguide.md` extended with this merge's commits and any follow-up fix commits
- [ ] Rebase checklist extended with any new fragility uncovered — at minimum: where Fix 1b's draft-suppression guard sits in the post-`589dc95c87` `ensure_thread_initialized` body, and the `supports_delete(&self, &App)` signature requirement
- [ ] Stale guide entries discovered along the way are corrected or deleted (do not invent updates). In particular, if `c84c22dab5` has made the Helix `// HELIX: External agent ...` bypass markers in `extensions_ui.rs` redundant, document the removal explicitly rather than re-applying a now-no-op patch

### Process
- [ ] Feature branch `feature/002029-merge-latest-zed` created from current fork main (`fd26c1a113`, or newer if anyone pushed meanwhile)
- [ ] Branch pushed to `helixml/zed`
- [ ] Helix repo `ZED_COMMIT` bump prepared in `sandbox-versions.txt` (from `b35224530f` to the new merge HEAD), branch `feature/002029-merge-latest-zed` pushed to Helix
- [ ] `pull_request_zed.md` and `pull_request_helix.md` written into this task directory
- [ ] `main` is **not** force-pushed without explicit user approval
- [ ] The Helix UI handles PR creation — do not open PRs from the agent (per task convention)

## Out of Scope

- Net-new Helix feature development
- Modifying e2e test assertions themselves (unless a legitimate upstream API change strictly requires it)
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix-specific crates beyond what conflict resolution requires
- Adopting upstream's `agent_skills` crate into Helix workflows (it's a new upstream crate — let it sit; Helix-mode users don't see Skills today)
