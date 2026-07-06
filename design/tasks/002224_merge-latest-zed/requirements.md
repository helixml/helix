# Requirements: Merge Latest Zed Upstream Into Helix Fork (002224)

## Context

Today is **2026-07-06**. This task pulls all new commits from `zed-industries/zed` main into
the `helixml/zed` fork, resolves conflicts following the patterns established over ~16 prior
merges, updates the porting guide **as decisions are made**, and confirms the full test suite
(including the external WebSocket-sync Docker e2e) passes.

### Baseline observed in the reference clone (`/home/retro/work/zed`)

The reference clone available in this planning environment shows this state:

| | Value |
|---|---|
| Fork HEAD (`origin/main`) | `9546054e68` ("fix(external_websocket_sync): emit terminal frame when ACP agent crashes mid-turn (#65)") |
| Latest porting-guide merge entry | `## Merge 002100-extension (2026-06-18)` |
| Last upstream merge fence | `e45e42af6e` ("agent_ui: Use the thread title for agent notifications (#59377)") — absorbed in 002100-extension |
| `agent-client-protocol` (Cargo.lock) | `0.14.0` |
| `agent-client-protocol-schema` (Cargo.lock) | `0.13.6` |
| E2E phases present | **17** |
| Helix `sandbox-versions.txt` `ZED_COMMIT` | `9546054e68e2b771ac63e55821a70654684ac651` |

**CRITICAL — the reference clone is a snapshot and may be stale.** The most recent
merge-latest-zed **spec** in `helix-specs` is `002153` (dated 2026-06-22, targeting the same
`9546054e68` base), but its branch and porting-guide entry are **absent** from this clone —
so from this snapshot's perspective it has not landed. The real working repo
(`/prod/home/luke/pm/zed-upstream`, branch `helix-fork`) is **not accessible here** and is
almost certainly further ahead (14 days have passed since 002153; the merge cadence is every
few days). **Therefore the true baseline — current fork HEAD, the last-merged upstream fence,
upstream HEAD, the commit count, the ACP version, and the E2E phase count — MUST be
re-measured in the real working repo at execution time.** The table above is the starting
hypothesis, not ground truth.

### Repository layout (two views of the same fork)

Prior specs describe the in-cluster mirror workflow (`origin` = gitea mirror, feature branch
`feature/NNNNNN-merge-latest-zed`, Helix UI creates the PR). The task constraints describe the
canonical working repo on the maintainer's machine:

- Working repo: `/prod/home/luke/pm/zed-upstream`, branch `helix-fork`
- `helix` remote = `git@github.com:helixml/zed.git` (the fork; push here)
- `upstream` remote (read-only) = `https://github.com/zed-industries/zed.git`

The implementation agent must confirm which layout applies in its environment before starting
and follow the established branch-naming/PR convention (`feature/002224-merge-latest-zed`;
Helix UI opens PRs — do **not** open PRs from the agent).

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to absorb the upstream catch-up now, following the documented
> conflict-resolution patterns, so merge debt stays small and future rebases stay cheap.

### 2. Helix User
> As a Helix user, I want upstream reliability/performance fixes without losing any of the
> Helix-only surface: the `external_websocket_sync` crate, all active Critical Fixes, the
> headless/multi-instance flags, branding/feature-flag overrides, and the crash-recovery path.

### 3. Future Merge Engineer
> As the engineer running the next merge, I want `portingguide.md` extended **incrementally as
> each conflict is resolved** (a dated entry is required even for a zero-conflict merge) so the
> guide's uninterrupted timeline and lessons are preserved.

## Acceptance Criteria

### Baseline confirmation (measure, don't assume)
- [ ] `upstream` remote present and fetched; `helix`/`origin` fetched
- [ ] Current fork HEAD recorded; confirmed against `sandbox-versions.txt` `ZED_COMMIT`
- [ ] Last-merged upstream fence identified from the top of `portingguide.md`'s merge history
- [ ] Upstream HEAD SHA recorded and commit count `<fence>..upstream/main` recorded
  (per open question: `git log --oneline helix-fork..upstream/main | wc -l`)
- [ ] `agent-client-protocol` / `-schema` versions in `Cargo.lock` recorded (check for a bump)

### Merge completeness
- [ ] `helix-fork` includes all upstream commits through current upstream HEAD
- [ ] No upstream commits silently cherry-picked out; any deliberate skip justified in the guide
- [ ] `git log helix-fork..upstream/main --oneline` is empty after the merge

### Feature-gating (hard invariant)
- [ ] All Helix custom code remains wrapped in `#[cfg(feature = "external_websocket_sync")]`
- [ ] `cargo check` passes **both** with and without the `external_websocket_sync` feature

### ACP protocol handling (if version bumped)
- [ ] If `agent-client-protocol[-schema]` bumped: every `non_exhaustive` ACP struct literal
  updated to the builder pattern; all `ErrorCode` match arms checked; `AgentConnection` /
  `StubAgentConnection` impls compile
- [ ] Compile-driven: build with the feature must be zero-error

### Critical Fix preservation (per `portingguide.md` "Critical Fixes")
- [ ] Fix #1: `NativeAgent` entity kept alive during `load_session` (`pending_sessions` shared-task) — `agent/src/agent.rs`
- [ ] Fix #2: no duplicate WebSocket sends from `thread_view.rs`
- [ ] Fix #3: `content_only()` strips the `## Assistant` heading — `acp_thread.rs`
- [ ] Fix #4: follow-up to a non-visible thread calls `notify_thread_display()` — `thread_service.rs`
- [ ] Fix #5: stale pending entries flushed when a different entry starts streaming — `thread_service.rs`
- [ ] Fix #6: exactly one `Stopped(_)` per turn (`stopped_emitted_for_task` guard) — `acp_thread.rs`
- [ ] Fix #7: `THREAD_REGISTRY` unregistered on entity replacement
- [ ] Fix #8: `cancel()` **drops** `send_task` (does not await/spawn it) — `acp_thread.rs`
- [ ] Fix #9: normal-completion `Stopped` guarded against duplicate emission — `acp_thread.rs`
- [ ] Fix #11: entity-identity guard at top of `load_agent_thread` via `ThreadMetadataStore` — `agent_panel.rs`

### Helix surface (must survive; grep + build are the gates)
- [ ] `crates/external_websocket_sync/` crate intact (all source files)
- [ ] PR #65 crash-recovery: `StubAgentConnection::fail_turn()` in `acp_thread/src/connection.rs`;
  `Error`-arm handling + `chat_response_error` emit in `thread_service.rs`;
  `SyncEvent::ChatResponseError` in `types.rs`; `TEST_WEBSOCKET_SERVICE_GUARD` shared by the
  crash + reconnect tests
- [ ] PR #60 `ede_diagnostic` 4×750ms retry in `handle_follow_up_message` — `thread_service.rs`
- [ ] PR #63 wedge-recovery surface (`force_reset_session`, `clear_keep_alive`, agent_name tracking) — `thread_service.rs`
- [ ] PR #64 `agent_ready` re-emit on reopening an already-loaded thread — `thread_service.rs`
- [ ] PR #55 `EntryUpdated` emit after streaming-reveal drain — `acp_thread.rs`
- [ ] PR #56 Fix 1a deferred `UserCreatedThread` plumbing in `external_websocket_sync`
- [ ] PR #56 Fix 1b `#[cfg(feature="external_websocket_sync")] { return; }` remains the **FIRST**
  statement of the `BaseView::Uninitialized` branch in `ensure_thread_initialized` — `agent_panel.rs`
- [ ] PR #57 Phase-16 counter exclusion in `helix-ws-test-server/main.go`
- [ ] PR #50 `session_creation_chain` coexists with `_settings_subscription` — `agent_servers/src/acp.rs`;
  `test_concurrent_session_creation_is_serialized` passes
- [ ] `AcpBetaFeatureFlag::enabled_for_all() -> true` — `feature_flags/src/flags.rs`
- [ ] `trust_all_worktrees: true` in `assets/settings/default.json`
- [ ] `show_sign_in: false` + branding/onboarding settings in `assets/settings/default.json`
- [ ] `OnboardingUpsell::set_dismissed(true, cx)` still called in the ThreadDisplayNotification handler
- [ ] `NativeAgentSessionList` still initialised in the ThreadDisplayNotification handler
- [ ] Built-in agent hiding (Claude Code / Codex / Gemini) wrapped in `cfg(not(feature = "external_websocket_sync"))`
- [ ] Streaming WebSocket updates: windowless `cx.subscribe()` in `thread_service.rs` preserved (incremental `message_added`)
- [ ] GPUI subscription pattern preserved: channel-based UI forwarding where no window context exists; `App::subscribe` for WebSocket forwarding
- [ ] `--allow-multiple-instances` and `--headless` CLI flags + `initialize_headless()` — `crates/zed/src/main.rs`
- [ ] `rust-embed` workspace dep keeps the `debug-embed` feature; `wait_for_tools_ready` uses `cx.background_executor().timer()` (no `smol::Timer`)
- [ ] `render_restricted_mode` cfg-gated early return — `title_bar`; `// HELIX: External agent` bypass markers — `extensions_ui.rs`
- [ ] `BaseView` / `ContextServerStatus` exhaustive matches updated for any new upstream variants
- [ ] `fd26c1a113` `Dockerfile.ci` `helix-org` pull intact

### Build & test (hard gates)
- [ ] Zed builds clean (via `cd /home/retro/work/helix && ./stack build-zed dev`, or the working repo's canonical builder) — zero errors
- [ ] CI `.drone.yml` still uses `cargo build --locked` (source is mounted read-only in CI)
- [ ] Unit tests in `external_websocket_sync` pass (incl. PR #65 crash-regression + reconnect, sharing `TEST_WEBSOCKET_SERVICE_GUARD`)
- [ ] `cargo test -p acp_thread test_second_send` (Fix #6) and `-p agent_servers test_concurrent_session_creation_is_serialized` (PR #50) pass where a local toolchain is available
- [ ] **External WebSocket-sync E2E (Docker) — HARD GATE**: the complete current suite passes for **both** `zed-agent` and `claude`. The reference clone has **17** phases; confirm the actual count by reading the test. The task's named core phases must all pass:
  - Phase 1: new thread — agent_ready → chat_message → thread_created → message_completed
  - Phase 2: follow-up on same thread — entry_count increases
  - Phase 3: new second thread created and switched to
  - Phase 4: message to a non-visible Thread A (entity-released regression)
  - Plus, in the current suite: Phase 8 (mid-stream interrupt / Stopped invariant), **Phase 9** (rapid 3-turn cancel — PR #60 retry gate), **Phase 15** (incremental streaming — PR #55), **Phase 16** (zero spontaneous `UserCreatedThread` — PR #56 Fix 1a + PR #57), **Phase 17** (live agent process count == real thread count — PR #56 Fix 1b)
  - UI state queries pass (correct `thread_id`, `entry_count`, `active_view`)
- [ ] One retry permitted for the Claude Phase-1 npm-install bootstrap flake and for the Phase-9 API-latency flake (documented patterns). Never use `--no-build` when diagnosing an E2E failure.
- [ ] `helixml/zed` CI pipeline green

### Documentation (hard gate — incremental, not retrospective)
- [ ] `portingguide.md` entry started **when `git merge upstream/main` is issued** and extended as each conflict is resolved
- [ ] New `## Merge 002224 (2026-07-06)` section at the top of the merge-history list
- [ ] Window summary (actual commit count + upstream HEAD SHA), per-conflict resolutions (or explicit "0 conflicts, auto-merge clean" note), Helix-surface survival check, PR #60/#63/#64/#65 survival check, Cargo.toml/Cargo.lock notes
- [ ] `### Pre-existing Breakage Repaired` subsection only if a signature-drift / typed-error / new-variant fix fires
- [ ] Commit-history table extended; stale guide entries corrected/deleted as found

### Process
- [ ] Feature branch `feature/002224-merge-latest-zed` created from current fork HEAD
- [ ] Branch pushed to the fork (`helix`/`origin`); `main`/`helix-fork` **not** force-pushed without explicit user approval
- [ ] Helix `sandbox-versions.txt` `ZED_COMMIT` bumped to the new merge HEAD on `feature/002224-merge-latest-zed`; helix branch pushed
- [ ] `pull_request_zed.md` and `pull_request_helix.md` written into this task directory
- [ ] PRs opened by the Helix UI, not the agent

## Out of Scope

- Net-new Helix feature development
- Rewriting the porting guide from scratch (amend/extend in place)
- Modifying E2E assertions themselves unless a legitimate upstream API change strictly requires it
- Upstreaming Helix patches back to `zed-industries/zed`
- Refactors of Helix-specific crates beyond what conflict resolution requires

## Open Questions

- **How many commits are new on upstream since the last merge?** Must be measured at runtime in
  the real working repo: `git log helix-fork..upstream/main --oneline | wc -l`. The reference
  clone's fence (`e45e42af6e`) is likely stale.
- **Has 002153 (and any merges between 2026-06-22 and today) already landed on `helix-fork`?**
  This determines the true baseline fence. The reference clone does not show 002153, but the
  real working repo probably does. Confirm from the top of `portingguide.md`'s merge history
  and `git log`.
- **Has the ACP crate version bumped again?** Check `agent-client-protocol[-schema]` in
  `Cargo.lock` (reference clone: 0.14.0 / 0.13.6). If bumped, expect `non_exhaustive` builder
  and `ErrorCode` churn in `agent_servers/src/acp.rs`.
- **Have any high-conflict files been significantly restructured upstream?** Specifically
  `agent_ui/src/agent_panel.rs`, `zed/src/zed.rs` (`initialize_agent_panel`),
  `agent_servers/src/acp.rs`, `anthropic/src/anthropic.rs`, `agent_ui/src/acp/thread_view.rs`.
- **Is upstream's session list / resume UI now implemented in a way that conflicts with or could
  replace the Helix `from_existing_thread` approach?** If so, flag rather than silently rework;
  escalate before diverging from the established pattern.
- **Which repo layout applies in the execution environment** — the in-cluster mirror
  (`origin` + `feature/NNNNNN` + Helix-UI PR) or the maintainer's `/prod/home/luke/pm/zed-upstream`
  `helix-fork` layout? Confirm before pushing.
