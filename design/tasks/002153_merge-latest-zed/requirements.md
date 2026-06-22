# Requirements: Merge Latest Zed Upstream (002153)

## Context

Today is **2026-06-22**. The Helix fork of Zed last merged `zed-industries/zed` via **task 002100** (PR `helixml/zed#62`, completed 2026-06-18 in two rounds). That task absorbed 120 upstream commits total (25 in round 1, 95 in round 2), reaching upstream fence `e45e42af6e` ("agent_ui: Use the thread title for agent notifications (#59377)").

Since that merge, **one Helix-only commit** has landed on fork main:

| | Commit | Date |
|---|---|---|
| Fork HEAD (`origin/main`) | `9546054e68` ("fix(external_websocket_sync): emit terminal frame when ACP agent crashes mid-turn (#65)") | 2026-06-19 |
| Helix `sandbox-versions.txt` `ZED_COMMIT` | `9546054e68e2b771ac63e55821a70654684ac651` (**already at fork HEAD** — no sandbox catch-up debt) | 2026-06-19 |
| Last upstream merge fence | `e45e42af6e` ("agent_ui: Use the thread title for agent notifications") — absorbed in 002100-extension | 2026-06-18 |
| Upstream HEAD | **UNKNOWN** — must be measured at execution time by adding the `upstream` remote and fetching | — |
| Upstream remote configured locally | **No** — must add it: `git remote add upstream https://github.com/zed-industries/zed.git` |

The window since the last merge is **4 days** (2026-06-18 → 2026-06-22). Based on prior cadence (002100 round 1: 25 commits / 3 days; round 2: 95 commits / 3 days), expect anywhere from **20–80 upstream commits**. The actual count and upstream HEAD SHA must be confirmed at execution time.

### PR #65 — the new Helix-only surface to preserve

PR #65 (`9546054e68`) adds error-path coverage that was absent in prior merges:

> "When the ACP agent process exits mid-turn, `AcpThread::run_turn` takes its `Err` arm and emits `AcpThreadEvent::Error` — not `Stopped`. The persistent thread subscription only sent a terminal frame to Helix from its `Stopped` handler; there was no `Error` arm, so the crash was swallowed. Helix only leaves `state=waiting` on a terminal event, so a turn that streamed partial output and then crashed stayed waiting forever, permanently wedging the worker."

Changed files:
- `crates/acp_thread/src/connection.rs` — +14 lines: adds `fail_turn()` to `StubAgentConnection` (test helper)
- `crates/external_websocket_sync/src/thread_service.rs` — +194 lines: Error arm combined with Stopped handler; emits `chat_response_error` for the in-flight `request_id`; new regression test sharing `TEST_WEBSOCKET_SERVICE_GUARD` with the existing reconnect test
- `crates/external_websocket_sync/src/types.rs` — +15 lines: new `SyncEvent::ChatResponseError` variant

`connection.rs` is an upstream file; the other two are Helix-only.

### Risk profile: LOW

The 4-day window is identical in length to 002100 round 1 (25 commits / 3 days). The dominant new variable vs. 002100 is PR #65's `connection.rs` addition — but it only adds to `StubAgentConnection` (a test helper struct), not to the `AgentConnection` trait itself, so upstream trait additions are unlikely to conflict. All other PR #65 surface (`thread_service.rs`, `types.rs`) is Helix-only and will never conflict.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to absorb the ~4-day upstream catch-up while it is small, rather than letting it grow into a multi-week backlog that inflates conflict risk.

### 2. Helix User
> As a Helix user, I want upstream bug fixes without losing PR #65's crash-recovery path, PR #60's `ede_diagnostic` retry loop, PR #63's wedge-recovery logic, PR #64's `agent_ready` re-emit, or any of the 10 active Critical Fixes.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want `portingguide.md` updated **as each conflict is resolved** (even if there are zero conflicts, a dated entry is still required) so the porting guide's uninterrupted timeline is maintained.

## Acceptance Criteria

### Baseline Confirmation
- [ ] PR `helixml/zed#62` is confirmed merged to fork main (fork HEAD is `9546054e68` or newer — PR #65 is on top of #62)
- [ ] Upstream remote added and fetched; upstream HEAD SHA and commit count `e45e42af6e..upstream/main` documented at execution time
- [ ] `sandbox-versions.txt` `ZED_COMMIT` confirmed already at fork HEAD (no pre-merge catch-up needed)

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD (any skipped commits explicitly justified in `portingguide.md`)
- [ ] No upstream commits silently cherry-picked out

### PR #65 Preservation (new since 002100)
- [ ] `SyncEvent::ChatResponseError` variant intact in `crates/external_websocket_sync/src/types.rs`
- [ ] Error arm combined with Stopped handler intact in `thread_service.rs::handle_persistent_thread_subscription` — flushes partial content and emits `chat_response_error` on `AcpThreadEvent::Error`
- [ ] `TEST_WEBSOCKET_SERVICE_GUARD` shared between reconnect test and the new crash-regression test
- [ ] `fail_turn()` on `StubAgentConnection` in `crates/acp_thread/src/connection.rs` intact
- [ ] If upstream has added new methods to `AgentConnection` / `StubAgentConnection` since `e45e42af6e`, the post-merge `connection.rs` still compiles with `fail_turn` intact

### Critical Fix Preservation (10 active fixes)
- [ ] Fix #1: `NativeAgent` entity cloned/`pending_sessions` shared-task pattern in `load_session()`
- [ ] Fix #2: No duplicate WebSocket sends from `crates/agent_ui/src/acp/thread_view.rs`
- [ ] Fix #3: `content_only()` strips `## Assistant` heading — `acp_thread.rs:335` (line may shift after merge)
- [ ] Fix #4: `notify_thread_display()` called for follow-ups to non-visible threads
- [ ] Fix #5: stale pending entries flushed when a different entry starts streaming
- [ ] Fix #6: every `send()` emits exactly one `Stopped` (`stopped_emitted_for_task` guard) — `acp_thread.rs:2887/2931/3026`
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it — `acp_thread.rs:3079`
- [ ] Fix #9: `stopped_emitted_for_task` guard on the normal completion path — same sites as #6
- [ ] Fix #11: entity-identity guard at top of `load_agent_thread` in `agent_panel.rs` (via `ThreadMetadataStore` session_id lookup)

### Helix-Specific Surface (must survive verbatim)
- [ ] `crates/external_websocket_sync/` crate intact (all source files)
- [ ] PR #60 `handle_follow_up_message` 4×750ms retry on `ede_diagnostic` transient intact in `thread_service.rs:1916/1976` (lines may shift)
- [ ] PR #63 claude-agent-acp wedge recovery surface (`force_reset_session`, `clear_keep_alive`, agent_name tracking, stripped dispatch diagnostics) intact in `thread_service.rs`
- [ ] PR #64 `agent_ready` re-emit on reopening already-loaded thread intact in `thread_service.rs`
- [ ] WebSocket thread display callback + UI state query callback in `agent_panel.rs::new()`
- [ ] `acp_history_store()` accessor on `AgentPanel`
- [ ] `from_existing_thread()` constructor on `ConversationView` — verify field set matches `ConversationView::new` (build is the gate)
- [ ] PR #50 `session_creation_chain` field + drop-guard intact, coexisting with `_settings_subscription` at `agent_servers/src/acp.rs:438-439`
- [ ] PR #50 `test_concurrent_session_creation_is_serialized` test passes
- [ ] PR #55 `EntryUpdated` emit after streaming-reveal drain in `acp_thread.rs` (16 occurrences) preserved
- [ ] PR #56 Fix 1a deferred `UserCreatedThread` plumbing in `external_websocket_sync` intact
- [ ] PR #56 Fix 1b `#[cfg(feature = "external_websocket_sync")] { return; }` still the FIRST statement of `BaseView::Uninitialized` branch in `ensure_thread_initialized` — currently at `agent_panel.rs:5468-5473` (line will shift)
- [ ] PR #57 Phase 16 counter exclusion in `helix-ws-test-server/main.go` intact
- [ ] `fd26c1a113` `Dockerfile.ci` `helix-org` pull intact
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` override intact — `feature_flags/src/flags.rs:30`
- [ ] Built-in agent hiding (Claude Code, Codex, Gemini hidden in Helix builds)
- [ ] Enterprise TLS skip (`sync_settings`)
- [ ] `--allow-multiple-instances` CLI flag in `crates/zed/src/main.rs`
- [ ] `--headless` CLI flag + `initialize_headless()` + `build_application(headless: bool)` pattern intact
- [ ] `rust-embed` workspace dep still has `debug-embed` feature
- [ ] `wait_for_tools_ready` uses `cx.background_executor().timer()` (not `smol::Timer`)
- [ ] `title_bar` feature propagation chain intact (`optional = true`)
- [ ] `title_bar::render_restricted_mode` cfg-gated early return intact — `title_bar.rs:699` (line may shift)
- [ ] `extensions_ui.rs` `// HELIX: External agent …` bypass markers retained — currently at lines 337, 359, 1629 (will shift); verify by grep
- [ ] `BaseView` / `ContextServerStatus` exhaustive matches updated for any new variants added upstream this window

### Build & Test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — zero errors
- [ ] (If local Rust toolchain available) `cargo check -p zed --features external_websocket_sync` — zero errors
- [ ] (If local Rust toolchain available) `cargo test -p external_websocket_sync` — full pass (the PR #65 regression test must pass; `TEST_WEBSOCKET_SERVICE_GUARD` must be in place)
- [ ] (If local Rust toolchain available) `cargo test -p acp_thread test_second_send` — passes (Fix #6 invariant)
- [ ] (If local Rust toolchain available) `cargo test -p agent_servers test_concurrent_session_creation_is_serialized` — passes
- [ ] **External WebSocket sync E2E (Docker, hard gate)** — all 17 phases pass for **both** `zed-agent` and `claude` agents. Key phases:
  - Phases 1–7 (basic sync + non-visible follow-up + UI state + open_thread)
  - Phase 8 (mid-stream interrupt — Stopped invariant)
  - **Phase 9 (rapid 3-turn cancel — PR #60 retry-loop regression gate)**
  - Phase 10 (user_created_thread)
  - Phase 11 (spectask routing)
  - Phase 12 (reconnect)
  - Phase 13 (`cancel_current_turn` happy path)
  - Phase 14 (`cancel_current_turn` no-op)
  - **Phase 15** (streaming patches incrementally — PR #55 emit gate)
  - **Phase 16** (zero spontaneous `UserCreatedThread` — PR #56 Fix 1a + PR #57)
  - **Phase 17** (live Claude process count == real thread count — PR #56 Fix 1b draft-suppression gate)
- [ ] One retry permitted for Claude Phase 1 npm-install bootstrap flake; one retry permitted for Phase 9 API-latency flake (both documented patterns from 002100)

### Documentation (hard gate — written incrementally)
- [ ] `portingguide.md` updated **as each conflict is resolved** (even if zero conflicts, start the entry when `git merge upstream/main` is issued)
- [ ] New `## Merge 002153 (2026-06-22)` section appended at the top of the merge-history list
- [ ] Per-conflict subsection (or explicit "0 conflicts, auto-merge clean" note)
- [ ] **PR #65 survival check** subsection — confirm all three changed files (`connection.rs`, `thread_service.rs`, `types.rs`) are intact post-merge
- [ ] Helix-surface auto-merge survival check subsection
- [ ] `### Pre-existing Breakage Repaired` subsection only if any signature-drift / typed-error / new-variant fix fires
- [ ] Commit-history table at the bottom of `portingguide.md` extended with this merge's commits
- [ ] Stale guide entries discovered along the way are corrected or deleted

### Process
- [ ] Feature branch `feature/002153-merge-latest-zed` created from current fork main (`9546054e68`)
- [ ] Branch pushed to `helixml/zed`
- [ ] Helix repo `ZED_COMMIT` bump prepared in `sandbox-versions.txt` (from `9546054e68e2b771ac63e55821a70654684ac651` to the new merge HEAD)
- [ ] Helix branch `feature/002153-merge-latest-zed` pushed
- [ ] `pull_request_zed.md` and `pull_request_helix.md` written into this task directory
- [ ] `main` is **not** force-pushed without explicit user approval
- [ ] The Helix UI handles PR creation — do not open PRs from the agent (per task convention)

## Out of Scope

- Net-new Helix feature development
- Modifying E2E test assertions themselves (unless a legitimate upstream API change strictly requires it)
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix-specific crates beyond what conflict resolution requires
