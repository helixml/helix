# Requirements: Merge Latest Zed Upstream (002251)

## Context

Today is **2026-07-13**. The Helix fork of Zed last merged `zed-industries/zed`
via **task 002100** (PR `helixml/zed#62`, two-round merge: base `0e0149ade5`
absorbing upstream `a31d3505da`, then the `## Merge 002100-extension (2026-06-18)`
round absorbing upstream `e45e42af6e`). Since that merge landed on fork main,
three Helix-side PRs shipped on top of it (#65, #66, #67 — see below).

The gap since the last upstream fence is **~25 days** — the **largest catch-up
window in this entire merge series**. Confirmed at runtime on `/home/retro/work/zed`
on 2026-07-13:

| | Commit | Date |
|---|---|---|
| Fork HEAD (`origin/main`) | `367ba0d489` ("Merge pull request #67 from helixml/feature/codex-subs") | 2026-07-12 |
| Helix `sandbox-versions.txt` `ZED_COMMIT` | `548da160ce` (**3 commits behind fork HEAD** — does not yet include PR #66 merge commit or PR #67) | 2026-07-12 |
| Last upstream merge fence | `e45e42af6e` ("agent_ui: Use the thread title for agent notifications (#59377)") — absorbed in 002100-extension | 2026-06-18 |
| Upstream HEAD (`upstream/main`) | `aeeacf5439` ("Indent statements under C/C++ case labels (#60758)") | 2026-07-12 |
| Upstream commits to merge (`e45e42af6e..upstream/main`) | **409** | ~25 days |
| Helix-only commits on fork main since fence | **3 merged PRs** (#65, #66, #67) | ~25 days |
| `upstream` git remote configured locally | **No** in fresh clones — must be added |

### Risk profile: HIGH (largest and most protocol-invasive window in the series)

This is not a 002100-style trivial catch-up. Every Helix-touched area has real
upstream churn, and one change is a hard breaking-API event:

**Headline risk — ACP crate major version bump `0.14.0` → `1.1.0`.**
`Cargo.toml` currently pins `agent-client-protocol = "=0.14.0"`; upstream pins
`"=1.1.0"`. Landed via `8ba35e5eac` "acp: Update agent client protocol crate to
0.15.0 (#59593)" and subsequent bumps within the window. This crate is the
foundation of the `AgentConnection` trait, session types, ACP event/stop-reason
types, and tool-call content types that the **entire** `external_websocket_sync`
layer depends on. Expect trait-signature and enum-shape drift that the build must
drive out across `agent_servers/src/acp.rs`, `acp_thread/`, `agent/src/`, and
possibly `external_websocket_sync/` itself.

**Upstream commit churn per Helix-sensitive path (`e45e42af6e..upstream/main`):**

| Path | Upstream commits | Risk | Concern |
|---|---|---|---|
| `crates/agent/src/` | **25** | **HIGH** | Critical Fix #1 (`load_session`/`pending_sessions`), `wait_for_tools_ready`, sandboxing overhaul (`c49a29f461`, `4a99aa870e`, `3648fe6f19`, `8f92822cbf`). |
| `crates/settings_content/` | **24** | **HIGH** | Three-way settings coexistence; last window already hit a both-sides-added-field conflict here (`RemoteSettingsContent`). |
| `crates/acp_thread/` | **22** | **HIGH** | Critical Fixes #3/#6/#8/#9 (Stopped invariant, `content_only`, `drop(send_task)`). Upstream `e783b2f063` "Defer status updates until turn completion", `c3a4bda331` "Emit agent thread status change events", `70fd3c5774` "Handle acp message ids", `fbceb2823b` elicitations, `2df74932bc` embedded resources in tool call content. |
| `crates/agent_ui/src/agent_panel.rs` | **20** | **HIGH** | Fix 1b / `send_agent_ready` / `wait_for_websocket_connected` / UI-state-query callback. `550ddc9405` "Replace thread controls with slash commands", `a5f696bfa3` remove duplicate MCP menu, `eef824cce5` MCP settings UI, `1fd93cbd34` "Unship shared threads". |
| `crates/agent_ui/src/conversation_view.rs` | **13** | **MEDIUM-HIGH** | `from_existing_thread()` field-set drift (recurring signature-drift repair site — has fired in multiple past merges). |
| `crates/agent_servers/src/acp.rs` | **7** | **HIGH** | PR #50 `session_creation_chain` + `_settings_subscription` coexistence; directly consumes the bumped ACP crate. |
| `crates/workspace/src/workspace.rs` | **6** | MEDIUM | `CollaboratorId::Agent` follow-focus guard; typed-error surface. |
| `crates/agent_settings/` | **6** | MEDIUM | `auto_compact`/`show_onboarding`/`auto_open_panel`/`sandbox_permissions` coexistence. |
| `crates/zed/src/main.rs` | **5** | MEDIUM | `--headless`/`--allow-multiple-instances`/`build_application(headless)`. |
| `crates/title_bar/` | **5** | MEDIUM | `external_websocket_sync` optional dep + `render_restricted_mode` cfg gate. |
| `crates/feature_flags/` | **4** | LOW-MED | `AcpBetaFeatureFlag::enabled_for_all() -> true` override. |
| `crates/agent/src/tools/grep_tool.rs` | **3** | MEDIUM | Helix 001410 `truncate_long_lines` patch — **conflicted in 002100-extension** (`40211567b8`); expect another conflict here. |
| `crates/extensions_ui/src/extensions_ui.rs` | **2** | LOW-MED | 3× `// HELIX: External agent` bypass markers. |
| `crates/agent_ui/src/acp/thread_view.rs` | **0** | LOW | Critical Fix #2/#7 site — no upstream churn this window (but auto-merged neighbours may still shift it). |

**Deleted upstream files** (agent settings UI refactor `4ff55d09e5`): 
`crates/agent_ui/src/agent_configuration/add_llm_provider_modal.rs` and
`configure_context_server_tools_modal.rs` are removed upstream. Confirm Helix has
no cfg-gated code depending on them.

### New Helix surface since 002100-extension (must survive the merge)

These are already on fork main and are **not** upstream — they must be preserved
verbatim (they are exactly the "recent Helix-side changes" the task brief flagged):

- **PR #65** (`9546054e68`) — `external_websocket_sync`: emit a terminal frame when an ACP agent crashes mid-turn.
- **PR #66** (`6799c947a0`, incl. `548da160ce`) — task **002228** "unify all agent message sender" + **new E2E prompt-queue phases** (busy-defer, interrupt). The E2E phase list has grown beyond the old "17 phases"; confirm the current phase list at runtime rather than assuming a count.
- **PR #67** (`367ba0d489`, `fe0490187a`) — route the Codex agent to ACP (`external-sync`).

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to absorb the 409-commit / 25-day upstream
> catch-up — including the ACP `0.14 → 1.1` protocol bump — while carefully
> preserving every Helix customisation, so the fork does not fall further behind
> and the next merge is smaller.

### 2. Helix User
> As a Helix user, I want ~4 weeks of upstream bug fixes, sandboxing
> improvements, ACP protocol advances, and performance work — without losing
> WebSocket sync, draft suppression, headless mode, Codex/Claude ACP routing,
> the crash-terminal-frame behaviour, unified agent-message sending, or any of
> the active Critical Fixes.

### 3. Future Merge Engineer
> As the engineer running the next merge, I want `portingguide.md` updated **as
> each conflict is resolved** with a dated `## Merge 002251 (2026-07-13)` entry
> documenting every conflict, every retired-because-upstream-absorbed patch, and
> the ACP-bump repair sites — because a large window with no notes is unmergeable
> next time.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through upstream HEAD (`aeeacf5439` or newer; any skipped commits explicitly justified in `portingguide.md`)
- [ ] No upstream commits silently cherry-picked out
- [ ] All three post-002100 Helix PRs (#65, #66, #67) preserved verbatim on the merge branch

### ACP Protocol Bump (the headline gate)
- [ ] `agent-client-protocol` moves to upstream's `=1.1.0` pin (do **not** keep `0.14.0`)
- [ ] All `AgentConnection` / ACP-trait impls in `agent_servers/src/acp.rs`, `agent/src/`, `acp_thread/` compile against `1.1.0`
- [ ] `external_websocket_sync` compiles and passes E2E against the bumped ACP crate
- [ ] Every ACP-signature repair forced by the bump is documented in `portingguide.md` (a new `### Pre-existing Breakage Repaired` / ACP-bump subsection)

### Critical Fix Preservation (the active fixes in `portingguide.md` §"Critical Fixes")
- [ ] Fix #1: `NativeAgent` clone / `pending_sessions` shared-task pattern in `load_session()`
- [ ] Fix #2: no duplicate WebSocket sends from `agent_ui/src/acp/thread_view.rs`
- [ ] Fix #3: `content_only()` strips `## Assistant` heading
- [ ] Fix #4: `notify_thread_display()` for follow-ups to non-visible threads
- [ ] Fix #5: stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6: every `send()` emits exactly one `Stopped` (`stopped_emitted_for_task` guard) — **re-verify against `e783b2f063` "defer status updates" + `c3a4bda331` "emit thread status change events"**
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on the normal-completion path
- [ ] Fix #11: entity-identity guard in `agent_panel.rs` `load_agent_thread` (`ThreadMetadataStore` / session_id form)
- [ ] `cargo test -p acp_thread test_second_send` passes (Fix #6 invariant against the new status-event flow)

### Helix-Specific Surface (must survive)
- [ ] `crates/external_websocket_sync/` crate intact (all source files), including PR #65 crash-terminal-frame and PR #67 Codex-ACP routing
- [ ] PR #60 `handle_follow_up_message` retry-on-`ede_diagnostic` loop intact in `thread_service.rs`
- [ ] PR #63 wedge-recovery (`agent_name` per thread, force-reset) + PR #64 `agent_ready` re-emit intact
- [ ] WebSocket thread-display callback + UI-state-query callback in `agent_panel.rs` (`send_agent_ready`, `wait_for_websocket_connected`)
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] `from_existing_thread()` constructor on `ConversationView` — re-verify field set matches `ConversationView::new` (recurring signature-drift site; 13 upstream commits here this window)
- [ ] PR #50 `session_creation_chain` + `_settings_subscription` coexist on `AcpConnection`; `test_concurrent_session_creation_is_serialized` passes
- [ ] PR #55 `EntryUpdated` emit after streaming-reveal drain preserved (against `e783b2f063` defer)
- [ ] PR #56 Fix 1a deferred `UserCreatedThread` plumbing intact
- [ ] PR #56 Fix 1b cfg-gated draft-suppression `return;` still the FIRST statement of its `BaseView::Uninitialized` branch (re-locate — `agent_panel.rs` had 20 upstream commits incl. `550ddc9405` slash-commands refactor)
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` override intact
- [ ] Built-in agent hiding (Claude Code / Codex / Gemini hidden in Helix builds) — reconcile with `54bf918329` "Set agent as default when installing from registry" and `1fd93cbd34` "Unship shared threads"
- [ ] Enterprise TLS skip (`sync_settings`) intact
- [ ] `--allow-multiple-instances` + `--headless` + `initialize_headless()` + `build_application(headless)` intact in `crates/zed/src/main.rs`
- [ ] `rust-embed` workspace dep still has `debug-embed` feature
- [ ] `wait_for_tools_ready` uses `cx.background_executor().timer()` (not `smol::Timer`)
- [ ] `title_bar` feature propagation intact (`external_websocket_sync` `optional = true`) + `render_restricted_mode` cfg-gated early return
- [ ] `extensions_ui.rs` 3× `// HELIX: External agent` bypass markers retained (line numbers will shift)
- [ ] Helix 001410 `grep_tool.rs` `truncate_long_lines` patch reconciled with upstream `grep_tool.rs` churn (conflicted last window)
- [ ] `BaseView` / `ContextServerStatus` (and any new ACP event/status) exhaustive matches in Helix-mode code updated for new upstream variants

### Trait/API Migrations Forced by Upstream (expect several this window)
- [ ] ACP `0.14 → 1.1` signature drift resolved compile-driven (see headline gate)
- [ ] Any new `AcpThreadEvent` / stop-reason / thread-status variant added exhaustively in Helix match arms
- [ ] Any `ToolCall` content-type change (`2df74932bc` embedded resources) reconciled in `thread_service.rs` markdown extraction
- [ ] Settings three-way coexistence resolved for all new `settings_content` / `agent_settings` fields
- [ ] No reliance on the two deleted `agent_configuration/*_modal.rs` files

### Build & Test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — zero errors
- [ ] (If local Rust toolchain available) `cargo check -p zed --features external_websocket_sync` — zero errors
- [ ] (If local Rust) `cargo test -p external_websocket_sync` — full pass (≤2 env-dependent ignored acceptable)
- [ ] (If local Rust) `cargo test -p acp_thread test_second_send` — passes
- [ ] (If local Rust) `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` — passes
- [ ] **External WebSocket sync E2E (Docker, hard gate)** — **all phases green** for **both** `zed-agent` and `claude`, including the new PR #66 prompt-queue phases (busy-defer, interrupt). Confirm the current phase list at runtime; do not assume the historical "17". Named regression gates that must pass: mid-stream interrupt / Stopped invariant, rapid 3-turn cancel (PR #60), `cancel_current_turn` happy path, streaming-patches-incremental (PR #55), zero-spontaneous-`UserCreatedThread` (PR #56 Fix 1a + PR #57), live-Claude-process-count == thread-count (PR #56 Fix 1b), and the new prompt-queue busy-defer + interrupt phases (PR #66).
- [ ] `go mod tidy` run in `e2e-test/helix-ws-test-server/` before E2E (runner does not tidy)
- [ ] One retry permitted per agent for the known Claude Phase 1 npm-install bootstrap flake / single-phase API-latency hiccup

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved**, not at the end
- [ ] New `## Merge 002251 (2026-07-13)` section at the top of the merge-history list
- [ ] Per-conflict subsection under `### Conflicts and Resolutions`
- [ ] `### Pre-existing Breakage Repaired` subsection for ACP-bump signature drift and any other forced repairs
- [ ] Explicit **retirement** notes for any Helix patch now absorbed upstream (dropped Helix-side version + why)
- [ ] Helix-surface survival check subsection (per-area confirmation)
- [ ] Commit-history table at the bottom extended with this merge's commits
- [ ] Any new recurring gotcha added to the Rebase Checklist (only if actually confirmed this merge)
- [ ] Stale guide entries discovered along the way corrected or deleted

### Process
- [ ] Feature branch `feature/002251-merge-latest-zed` created from current fork main (`367ba0d489`, or newer if anyone pushed meanwhile)
- [ ] Branch pushed to `helixml/zed`
- [ ] Helix repo `ZED_COMMIT` bumped in `sandbox-versions.txt` from `548da160ce…` to the new merge HEAD, on a `feature/002251-merge-latest-zed` branch, pushed
- [ ] `pull_request_zed.md` and `pull_request_helix.md` written into this task directory
- [ ] `main` is **not** force-pushed without explicit user approval
- [ ] The Helix UI handles PR creation — do not open PRs from the agent
- [ ] If any single conflict is too risky to resolve confidently (most likely an ACP-`1.1.0` trait change), stop and escalate to the user with the specifics rather than guessing

## Out of Scope
- Net-new Helix feature development
- Modifying E2E test assertions (unless a legitimate upstream ACP API change strictly requires it)
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix crates beyond what conflict resolution / the ACP bump forces
- Adopting upstream's slash-command thread-controls, elicitation UI, or sandboxing UX into Helix-mode flows beyond keeping them compiling
- Wiring upstream `/compact` / `auto_compact` into Helix-mode UX

## Open Questions

- **ACP `1.1.0` blast radius:** the crate went `0.14 → 1.1` (through `0.15` `8ba35e5eac` and later bumps). How deep does the trait/enum signature drift reach — is it a handful of mechanical repairs, or does it change the `AgentConnection`/session lifecycle in a way that touches `external_websocket_sync` semantics? This is the single largest unknown; the implementation agent should assess early (build-driven) and escalate if it is not a mechanical fix.
- **Stopped-invariant collision:** does upstream `e783b2f063` "Defer status updates until turn completion" + `c3a4bda331` "Emit agent thread status change events" change *when* `Stopped` is emitted relative to Helix's `stopped_emitted_for_task` guard (Fix #6/#9)? If so, Fix #6 may need re-implementation, not just re-verification.
- **Fix 1b relocation:** `550ddc9405` "Replace thread controls with slash commands" (+ 19 other agent_panel.rs commits) likely moved the `BaseView::Uninitialized` region. Assumption: Fix 1b is still needed and re-anchorable — confirm at merge time.
- **Built-in agent hiding vs `54bf918329`/`1fd93cbd34`:** does upstream's "set agent as default on install" or "unship shared threads" interfere with Helix's built-in-agent hiding? Assumption: reconcilable by keeping the Helix filter; confirm.
- **Single-round vs extension rounds:** given the 409-commit size, is a single `git merge upstream/main` expected, or should the agent anticipate upstream advancing further mid-work and do a second (002251-extension) round as 002100 did? Assumption: single round targeting `aeeacf5439`, with a re-fetch before declaring done.
- **`ZED_COMMIT` lag:** the helix pin (`548da160ce`) trails fork HEAD by 3 commits. Assumption: irrelevant because the merge bumps it to the new HEAD anyway — flag only if the trailing commits indicate an unmerged fork-main state.
- Otherwise: none.
