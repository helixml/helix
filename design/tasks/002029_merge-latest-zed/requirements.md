# Requirements: Merge Latest Zed Upstream (002029)

## Context

Today is **2026-05-18**. The Helix fork of Zed last merged with `zed-industries/zed` via **task 001996** (merge commit `8841edb2b1`, integrating upstream `8bdd78e023`) on **2026-05-11**. PR #54 landed on fork main and four further Helix-only PRs (#50, #55, #56, #57) have shipped since.

Confirmed at run time on `/home/retro/work/zed`:

| | Commit | Date |
|---|---|---|
| Fork HEAD (`origin/main`) | `b2f2ebefb6` (PR #57 — Phase 16 counter fix) | 2026-05-14 |
| Helix `sandbox-versions.txt` `ZED_COMMIT` | `b35224530f` (the `b2f2ebefb6` merge's `^2`) | matches fork tip |
| Last upstream merge | `8841edb2b1` (task 001996, integrated `8bdd78e023`) | 2026-05-11 |
| Upstream HEAD (`upstream/main`) | `f2df3f9e18` ("acp: Add logout support for ACP agents (#56959)") | 2026-05-17 |
| Helix-only commits since 001996 | **14** (4 PRs: #50, #55, #56, #57) | 7 days |
| Upstream commits to merge (`8bdd78e023..upstream/main`) | **158** | 7 days |

### Helix PRs landed since 001996 (must survive verbatim)

| Merge commit | PR | Touches | Description |
|---|---|---|---|
| `50c4308fcb` | #50 | `crates/agent_servers/src/acp.rs` | Serialize ACP `session/new` & `session/load` per `AcpConnection` via `session_creation_chain`. Prevents two `claude-agent-acp` children racing on the npm `_npx` cache. |
| `cd4e279d80` | #55 | `crates/acp_thread/src/acp_thread.rs` (12 lines) | Emit `EntryUpdated` after streaming-reveal drain so WS sync sees fresh content. Phase 15 streaming-cadence gate. |
| `62cd60aaca` | #56 | `crates/external_websocket_sync/`, `crates/agent_ui/src/agent_panel.rs::ensure_thread_initialized` | Fix 1a: defer `UserCreatedThread` emit until first user message. Fix 1b: under `external_websocket_sync` feature, return early from `ensure_thread_initialized` instead of `activate_draft` (draft thread suppression). Adds Phase 16 + Phase 17. |
| `b2f2ebefb6` | #57 | `crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go` | Exclude Phase 10's own `UserCreatedThread` injection from Phase 16 counter (false-positive fix). |

### Risk profile: HIGH

Upstream diff sizes against Helix-touched files (`8bdd78e023..upstream/main`):

| File | Diff lines | Risk | Concrete concern |
|---|---|---|---|
| `crates/agent_ui/src/agent_panel.rs` | **3156** | **VERY HIGH** | Upstream PR `bbe23cc40b` "Bring back draft threads" rewrites 1256 lines of this file. **Helix PR #56's draft-suppression sits inside `ensure_thread_initialized()`** — the exact code path upstream is restructuring. Plus 9 other upstream PRs touch this file (rename support, terminal-as-thread, mermaid, skills, draft thread UX refinements, etc.). |
| `crates/agent/src/agent.rs` | 1897 | **HIGH** | `NativeAgent` and `NativeAgentConnection` heavily refactored upstream. Critical Fix #1 (entity-lifetime in `load_session`) and `wait_for_tools_ready` live here. |
| `crates/agent_ui/src/conversation_view.rs` | 720 | HIGH | `bbe23cc40b` draft threads, `bf423dfc45` ACP thread rename, `2301e61d2a` mermaid, `4074eabb3d` images, `1c884d13d3` terminal notifications, `f2df3f9e18` ACP logout (90-line addition) all touch this file. Helix `from_existing_thread()` lives here. |
| `crates/acp_thread/src/acp_thread.rs` | 313 | MEDIUM | Smaller than 001996, but PR #55 just added 12 lines to streaming-reveal that touch the same area as `2301e61d2a` mermaid and `495f8ba717` checkpoint-emit-on-edit. |
| `crates/agent_servers/src/acp.rs` | — (large) | MEDIUM-HIGH | PR #50 added `session_creation_chain` field + drop-guard. Upstream `23231879cd` "ACP session deletion" adds 161 lines to the same file. Likely real conflict. |
| `crates/acp_thread/src/connection.rs` | small | MEDIUM | **Trait-signature change**: `AgentSessionList::supports_delete(&self)` → `supports_delete(&self, &App)`. Plus new `supports_logout`/`logout` defaults on `AgentConnection`. Touches every Helix impl. |
| `crates/zed/src/main.rs` | 67 | LOW | `--allow-multiple-instances`, `--headless` flags must survive. |
| `Cargo.toml` (workspace) | 101 | LOW | `rust-embed`'s `debug-embed` feature must survive. |
| `Cargo.lock` | always | TRIVIAL | `--theirs`. |

### Specific highest-risk upstream commit: `bbe23cc40b` "Bring back draft threads"

This PR (`#54292`, 2026-05-11) **directly conflicts in intent** with Helix PR #56 (`769a463a2f`). Upstream is doubling down on the draft-thread UX — adding `draft_prompt_store.rs`, multi-draft retention in `retained_threads`, draft "parking" semantics, plus 1256 lines of churn in `agent_panel.rs` itself. Helix PR #56 specifically *disables* draft-thread spawning under `external_websocket_sync` to stop a duplicate `claude-agent-acp` spawn that brings 180s MCP `initialize` timeouts.

The Helix patch is a 25-line guard at the top of `ensure_thread_initialized()`. The merge resolution must keep that guard intact even if upstream renames or restructures the surrounding function. **If the function is gone entirely upstream, the implementation agent must find the equivalent entry point and re-apply the suppression.** Silent loss of this guard returns the duplicate-Claude bug and re-introduces the recurring "chrome MCP not working" symptom for spec-tasks.

### Specific medium-risk upstream commit: `23231879cd` "ACP session deletion"

Adds 161 lines to `crates/agent_servers/src/acp.rs` — the same file Helix PR #50 just modified to add `session_creation_chain`. Conflict expected. The `AgentSessionList::supports_delete` signature change cascades through `crates/agent_ui/src/acp/thread_history.rs` (multiple call sites) and `crates/agent/src/agent.rs:1838`.

### Specific medium-risk upstream commit: `f2df3f9e18` "ACP logout support"

Adds `supports_logout`/`logout` default methods to the `AgentConnection` trait (default impls, so not a compile break by itself), plus 90 lines to `conversation_view.rs`, 19 to `agent_panel.rs`, 108 to `agent_servers/src/acp.rs`. Last commit in this merge window — fresh from upstream as of 2026-05-17.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to merge the latest upstream Zed into the Helix fork so we stay current and minimise future merge debt — particularly given the heavy upstream churn in `agent_panel.rs` from "Bring back draft threads" which directly overlaps the draft-suppression guard Helix just added in PR #56.

### 2. Helix User
> As a Helix user, I want the new upstream Zed editor improvements (terminal-as-thread, mermaid diagrams, draft thread parking, ACP logout, agent skills, image attachments) without losing WebSocket sync or re-experiencing the duplicate-Claude / chrome MCP timeout issue PR #56 just fixed.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want `portingguide.md` updated **as work proceeds** — including a new entry for `bbe23cc40b`'s overlap with PR #56, the `supports_delete` signature change, and any silent renames in the 3156-line `agent_panel.rs` diff — so the rebase checklist reflects current reality.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD (`f2df3f9e18` or newer; any skipped commits explicitly justified in `portingguide.md`)
- [ ] All Helix-specific commits since 001996 (PRs #50, #55, #56, #57) are preserved and functional
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

### Newly Added Helix Behaviour Since 001996 (PRs #50, #55, #56, #57 — must survive merge)
- [ ] **PR #50** `session_creation_chain` field on `AcpConnection` + drop-guard in `new_session` / `open_or_create_session`. Compatible with upstream `23231879cd`'s ACP session-deletion additions to the same file
- [ ] **PR #50** `test_concurrent_session_creation_is_serialized` test passes
- [ ] **PR #55** `EntryUpdated` emitted after streaming-reveal drain in `acp_thread.rs` (~12 lines)
- [ ] **PR #56 Fix 1a** `defer_user_created_thread_until_first_user_message` behaviour in `external_websocket_sync` intact
- [ ] **PR #56 Fix 1b** the `#[cfg(feature = "external_websocket_sync")]` early-return guard in `agent_panel.rs::ensure_thread_initialized` preserved — re-applied if the upstream "Bring back draft threads" PR (`bbe23cc40b`) renames/restructures the entry point
- [ ] **PR #56** unit tests for deferred `UserCreatedThread` emit still pass
- [ ] **PR #57** Phase 16 counter excludes Phase 10's own `UserCreatedThread` injection (Go test-server change only)

### Trait-Signature Migration (forced by upstream)
- [ ] `AgentSessionList::supports_delete(&self)` → `supports_delete(&self, &App)` propagated through all Helix call sites (`thread_history.rs` and any `HeadlessConnection`-style impl)
- [ ] New `AgentConnection::supports_logout` / `logout` default impls accepted as-is (no Helix override needed unless the logout UI fires in Helix mode — confirm with E2E)

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
- [ ] New `## Merge 002029 (2026-05-18)` section mirroring 001996's structure
- [ ] Commit-history table at the bottom of `portingguide.md` extended with this merge's commits and any follow-up fix commits
- [ ] Rebase checklist extended with any new fragility uncovered (esp. anything about Fix 1b's location if upstream moved `ensure_thread_initialized`)
- [ ] Stale guide entries discovered along the way are corrected or deleted (do not invent updates)

### Process
- [ ] Feature branch `feature/002029-merge-latest-zed` created from current fork main (`b2f2ebefb6`, or newer if anyone pushed meanwhile)
- [ ] Branch pushed to `helixml/zed`
- [ ] Helix repo `ZED_COMMIT` bump prepared in `sandbox-versions.txt`, branch `feature/002029-merge-latest-zed` pushed to Helix
- [ ] `pull_request_zed.md` and `pull_request_helix.md` written into this task directory
- [ ] `main` is **not** force-pushed without explicit user approval
- [ ] The Helix UI handles PR creation — do not open PRs from the agent (per task convention)

## Out of Scope

- Net-new Helix feature development
- Modifying e2e test assertions themselves (unless a legitimate upstream API change strictly requires it)
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix-specific crates beyond what conflict resolution requires
- Adopting upstream's `agent_skills` crate into Helix workflows (it's a new upstream crate — let it sit; Helix-mode users don't see Skills today)
