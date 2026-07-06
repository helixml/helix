# Requirements: Merge Latest Zed Upstream Into Helix Fork (002223)

## Context

Today is **2026-07-06**. This task merges the latest `zed-industries/zed` (upstream)
commits into the Helix fork of Zed at `/home/retro/work/zed/`.

**Critical finding from reconnaissance (must be re-confirmed at execution time):**
The immediately-preceding upstream-merge spec, **002153** (dated 2026-06-22), was
**never landed**. Evidence in this clone:

| Signal | Value | Implication |
|---|---|---|
| Fork HEAD (`origin/main`) | `9546054e68` ("fix(external_websocket_sync): emit terminal frame when ACP agent crashes mid-turn (#65)") | Unchanged since 002153's *baseline* |
| `helix/sandbox-versions.txt` `ZED_COMMIT` | `9546054e68e2b771ac63e55821a70654684ac651` | Exactly at fork HEAD — no catch-up debt |
| Newest `portingguide.md` merge entry | `## Merge 002100-extension (2026-06-18)` (line 670, guide is 1109 lines) | 002153 entry never written |
| Merge branches on `origin` | latest is `feature/002100-merge-latest-zed`; **no `feature/002153-...` branch exists** | 002153 was planned but not executed/pushed |
| Last upstream merge fence | `e45e42af6e` ("agent_ui: Use the thread title for agent notifications (#59377)") — absorbed in 002100-extension | Merge base for this task |

**Consequence:** 002223 is effectively the merge that 002153 was meant to perform,
starting from the same baseline (`e45e42af6e` fence → upstream HEAD). The 002153
plan documents (`requirements.md`, `design.md`, `tasks.md`) are the direct,
still-valid playbook and must be read first. The only material difference is the
window is now **~18 days** (2026-06-18 → 2026-07-06) rather than 4, so expect
**more upstream commits and more conflicts** than 002153 predicted.

### Window & size estimate (confirm at execution time)

Based on prior cadence (002029: 261 commits/10 days; 002077: 256 commits/10 days;
002100: 120 commits/6 days), an ~18-day window likely yields **~150–450 upstream
commits** and **2–8 conflicts**. The exact upstream HEAD SHA and count
(`git log --oneline e45e42af6e..upstream/main | wc -l`) **must be measured at
execution time** — `upstream` is not configured in fresh clones.

### PR #65 — newest Helix-only surface to preserve

Fork HEAD `9546054e68` (PR #65) is the newest Helix commit and adds crash-recovery
error-path coverage. When the ACP agent process exits mid-turn, `AcpThread::run_turn`
takes its `Err` arm and emits `AcpThreadEvent::Error` (not `Stopped`); PR #65 adds an
Error arm to the persistent-thread subscription so Helix receives a terminal frame
and the worker is not wedged in `state=waiting` forever. Changed files:

- `crates/acp_thread/src/connection.rs` — `fail_turn()` on `StubAgentConnection` (upstream file → conflict-capable)
- `crates/external_websocket_sync/src/thread_service.rs` — Error arm + `chat_response_error` emit + regression test sharing `TEST_WEBSOCKET_SERVICE_GUARD` (Helix-only)
- `crates/external_websocket_sync/src/types.rs` — `SyncEvent::ChatResponseError` variant (Helix-only)

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to absorb the accumulated upstream catch-up (larger
> now that 002153 never landed) in one controlled merge so future merges stay small
> and the fork keeps benefiting from upstream bug fixes and performance work.

### 2. Helix User
> As a Helix user, I want upstream improvements without losing PR #65's crash-recovery
> path, PR #60's `ede_diagnostic` retry loop, PR #63's wedge recovery, PR #64's
> `agent_ready` re-emit, or any of the active Critical Fixes.

### 3. Future Merge Engineer
> As the engineer doing the *next* merge, I want `portingguide.md` updated **as each
> conflict is resolved** (a dated entry even if zero conflicts) so the timeline stays
> unbroken — 002153's missing entry must not repeat.

## Acceptance Criteria

### Baseline Confirmation
- [ ] Re-confirm at execution time that no `feature/002153-...` merge landed; if it *did* land out-of-band, re-measure the true merge base before proceeding
- [ ] Fork HEAD confirmed (`9546054e68` or newer); `ZED_COMMIT` confirmed at fork HEAD (no pre-merge sandbox catch-up)
- [ ] `upstream` remote added (`git remote add upstream https://github.com/zed-industries/zed.git`) and fetched; upstream HEAD SHA and commit count `e45e42af6e..upstream/main` documented

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD (any skipped commit explicitly justified in `portingguide.md`)
- [ ] No merge conflict markers remain (`git diff --check` clean)

### PR #65 Preservation
- [ ] `SyncEvent::ChatResponseError` intact in `types.rs`
- [ ] Error arm + `chat_response_error` emit intact in `thread_service.rs` persistent-thread subscription
- [ ] `TEST_WEBSOCKET_SERVICE_GUARD` shared between reconnect test and crash-regression test
- [ ] `fail_turn()` on `StubAgentConnection` intact in `connection.rs`; still compiles if upstream changed the `AgentConnection` trait

### Critical Fix Preservation (10 active fixes — lines will shift; verify by grep + build)
- [ ] Fix #1: `pending_sessions` shared-task path in `load_session()` — `agent/src/agent.rs`
- [ ] Fix #2: no duplicate WebSocket sends from `agent_ui/src/acp/thread_view.rs`
- [ ] Fix #3: `content_only()` strips `## Assistant` heading — `acp_thread.rs`
- [ ] Fix #4: `notify_thread_display()` for follow-ups to non-visible threads
- [ ] Fix #5: stale pending entries flushed when a different entry streams
- [ ] Fix #6: exactly one `Stopped` per `send()` (`stopped_emitted_for_task` guard) — `acp_thread.rs`
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement — `conversation_view.rs`
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting — `acp_thread.rs`
- [ ] Fix #9: `stopped_emitted_for_task` guard on the normal-completion path — same sites as #6
- [ ] Fix #11: entity-identity guard at top of `load_agent_thread` (via `ThreadMetadataStore`) — `agent_panel.rs`

### Helix-Specific Surface (must survive; verify by grep + build)
- [ ] `crates/external_websocket_sync/` crate intact (all source files)
- [ ] PR #50 `session_creation_chain` + `_settings_subscription` coexist — `agent_servers/src/acp.rs`
- [ ] PR #55 `EntryUpdated` emit after streaming-reveal drain (~16 occurrences) — `acp_thread.rs`
- [ ] PR #56 Fix 1a deferred `UserCreatedThread` plumbing — `external_websocket_sync`
- [ ] PR #56 Fix 1b `#[cfg(feature = "external_websocket_sync")] { return; }` is the FIRST statement of the `BaseView::Uninitialized` branch in `ensure_thread_initialized` — `agent_panel.rs`
- [ ] PR #57 Phase 16 counter exclusion — `helix-ws-test-server/main.go`
- [ ] PR #60 `handle_follow_up_message` 4×750ms retry on `ede_diagnostic` — `thread_service.rs`
- [ ] PR #63 wedge recovery (`force_reset_session`, `clear_keep_alive`, agent_name tracking) — `thread_service.rs`
- [ ] PR #64 `agent_ready` re-emit on reopening loaded thread — `thread_service.rs`
- [ ] `fd26c1a113` `Dockerfile.ci` `helix-org` pull — `e2e-test/Dockerfile.ci`
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` — `feature_flags/src/flags.rs`
- [ ] `--allow-multiple-instances` + `--headless` + `build_application(headless: bool)` — `crates/zed/src/main.rs`
- [ ] `render_restricted_mode` cfg-gated early return — `title_bar/src/title_bar.rs`; `title_bar` dep stays `optional = true`
- [ ] `extensions_ui.rs` three `// HELIX: External agent …` bypass markers survive (verify by grep — lines shift)
- [ ] `CollaboratorId::Agent` follow-focus guard — `workspace/src/workspace.rs`
- [ ] Helix `settings_content.rs` fields (`suggest_dev_container`, `helix_mode`, `auto_open_panel`, `show_onboarding`, `dev_container_use_buildkit`) coexist
- [ ] `rust-embed` `debug-embed` feature + `external_websocket_sync`/`cloud_api_types` workspace members intact — `Cargo.toml`
- [ ] `wait_for_tools_ready` uses `cx.background_executor().timer()` (not `smol::Timer`)
- [ ] `BaseView` / `ContextServerStatus` exhaustive matches updated for any new upstream variants (build is the gate)

### Build & Test (hard gates)
- [ ] `cd /home/retro/work/helix && ./stack build-zed dev` succeeds — zero errors
- [ ] (best-effort, if local Rust toolchain) `cargo test -p external_websocket_sync`, `cargo test -p acp_thread test_second_send`, `cargo test -p agent_servers test_concurrent_session_creation_is_serialized`
- [ ] **External WebSocket sync crate E2E (Docker) — the hard gate emphasised by this task:** all 17 phases pass for **both** `zed-agent` and `claude` agents. Special gates: Phase 8 (Stopped invariant), **Phase 9** (PR #60 retry-loop), Phase 13 (`cancel_current_turn`), **Phase 15** (PR #55 emit), **Phase 16** (PR #56 Fix 1a + PR #57), **Phase 17** (PR #56 Fix 1b)
- [ ] One retry permitted for Claude Phase 1 npm-install flake and for Phase 9 API-latency flake; never investigate a failure with `--no-build`
- [ ] **Do not finalise / bump `ZED_COMMIT` if the external WebSocket sync E2E tests are failing**

### Documentation (hard gate — written incrementally, not retrospectively)
- [ ] New `## Merge 002223 (2026-07-06)` section at the top of the merge-history list in `portingguide.md`, started when `git merge upstream/main` is issued
- [ ] Per-conflict subsection (or explicit "0 conflicts, auto-merge clean" note) written as each conflict is resolved
- [ ] PR #65 survival check, Helix-surface auto-merge survival check, and PR #60/#63/#64 survival check subsections written
- [ ] `### Pre-existing Breakage Repaired` subsection only if a signature-drift / typed-error / new-variant fix actually fires
- [ ] Commit-history table extended; any stale guide entries corrected/deleted

### Process
- [ ] Feature branch `feature/002223-merge-latest-zed` created from fork main and pushed to `origin` (helixml/zed)
- [ ] `helix` branch `feature/002223-merge-latest-zed` created; `ZED_COMMIT` bumped from `9546054e68e2b771ac63e55821a70654684ac651` to the new merge HEAD; pushed
- [ ] `pull_request_zed.md` and `pull_request_helix.md` written into this task directory summarising significant upstream changes and porting decisions
- [ ] `main` **not** force-pushed; PRs are opened by the Helix UI — the agent does not open PRs

## Out of Scope
- Net-new Helix feature development beyond what conflict resolution requires
- Modifying E2E test assertions (unless a legitimate upstream API change strictly requires it)
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix-specific crates beyond conflict resolution

## Open Questions

- **Did 002153 actually land out-of-band since this clone was taken?** Reconnaissance
  says no (no `feature/002153-...` branch, porting guide latest is `002100-extension`,
  fork HEAD unchanged). The executing agent must re-verify at start; if 002153 *did*
  land, the merge base and PR-inventory shift and the plan must be adjusted.
- **What is the exact upstream HEAD SHA and commit count in `e45e42af6e..upstream/main`?**
  Unknown until `upstream` is added and fetched at execution time (~150–450 expected).
- **Are there known upstream breaking changes in this ~18-day range** (new `BaseView` /
  `ContextServerStatus` variants, `AgentConnection` trait changes, Cargo/edition bumps)?
  Unknown; the `./stack build-zed dev` compile is the safety net.
- **Should the merge be split into rounds** if the commit count is very large (as 002100
  was done in two rounds)? Left to the executing agent's judgement based on the measured
  count and conflict density.
