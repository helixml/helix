# Design: Merge Latest Zed Upstream (002153)

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` — fork remote `origin` (in-cluster gitea URL is `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`). The in-cluster URL **is** the fork.
- **Upstream**: `zed-industries/zed`. The `upstream` remote is **not configured** in fresh clones — add it: `git remote add upstream https://github.com/zed-industries/zed.git`.
- **Porting guide** (canonical, living document): `/home/retro/work/zed/portingguide.md` — exists, **1109 lines** as of start of this task; latest entry `## Merge 002100-extension (2026-06-18)` at line 670.
- **Helix platform repo**: `/home/retro/work/helix/` — `sandbox-versions.txt` carries `ZED_COMMIT=9546054e68e2b771ac63e55821a70654684ac651`. This is **exactly at fork HEAD** — no sandbox catch-up debt.

## Current State (as of 2026-06-22)

| | Commit | Date |
|---|---|---|
| Fork HEAD | `9546054e68` ("fix(external_websocket_sync): emit terminal frame when ACP agent crashes mid-turn (#65)") | 2026-06-19 |
| Last upstream merge fence | `e45e42af6e` ("agent_ui: Use the thread title for agent notifications (#59377)") — absorbed in 002100-extension | 2026-06-18 |
| Upstream HEAD | **UNKNOWN** — must be measured at runtime (`git fetch upstream && git log --oneline upstream/main ^e45e42af6e | wc -l`) | — |
| Helix-only commits since 002100-extension | **1** (PR #65 on 2026-06-19) | 4 days |
| Upstream commits to merge | **UNKNOWN** — ~20–80 expected for a 4-day window | — |
| `upstream` git remote configured locally | **No** — must be added at start of work |

Recent merge precedent (size → conflict count):
- 002029: 261 commits, 10 days, **7** manual conflicts, 3 build-fix commits
- 002077: 256 commits, 10 days, **6** trivial conflicts, 0 signature-drift repairs
- 002100 round 1: 25 commits, 3 days, **1** trivial conflict (`RemoteSettingsContent` both-sides-added-a-field), 0 repairs, E2E green on retry
- 002100 round 2: 95 commits, 3 days, **1** trivial conflict (`grep_tool.rs` semantic reuse), 0 repairs, E2E green on full-rebuild retry
- **002153 outlook: ~4 days** — very similar profile to 002100 round 1. Predict **0–2 trivial conflicts** and **0** signature-drift repairs. The only new Helix surface is PR #65's `connection.rs` addition (`StubAgentConnection::fail_turn`), which is test-helper code unlikely to conflict with upstream.

## Merge Strategy

Use `git merge upstream/main` (consistent with every previous merge — preserves Helix commit history and makes conflict resolution traceable). Do **not** rebase.

```bash
cd /home/retro/work/zed
git remote add upstream https://github.com/zed-industries/zed.git   # only if missing
git fetch upstream
git log --oneline upstream/main ^e45e42af6e | wc -l               # measure delta
git log --oneline upstream/main -1                                  # record upstream HEAD SHA
git checkout main && git pull origin main                           # confirm still at 9546054e68
git checkout -b feature/002153-merge-latest-zed
git merge upstream/main
# Resolve conflicts (predicted 0–2) one at a time, updating portingguide.md as each is resolved.
# Build → critical-fix grep → unit tests → E2E (Phase 9 for PR #60, Phase 17 for Fix 1b).
git push -u origin feature/002153-merge-latest-zed
# Write pull_request_zed.md + pull_request_helix.md; bump ZED_COMMIT in helix.
```

### Conflict-resolution principles (carried from all prior merges)

1. **Prefer upstream ordering** for shared changes (model lists, feature lists, match arms).
2. **Keep Helix-specific code behind `#[cfg(feature = "external_websocket_sync")]`** — never let it leak into unconditional code paths.
3. **`Cargo.lock`**: always accept upstream (`git checkout --theirs`); regenerated on next build.
4. **CI / `.github/workflows/`**: always accept upstream — Helix doesn't use Zed's CI.
5. **Upstream renames**: after every merge — even when files auto-merge cleanly — grep for old identifiers in cfg-gated regions.
6. **Trait-signature changes**: walk all impls compile-driven; the post-merge build is the safety net.
7. **Document every non-trivial resolution in `portingguide.md` immediately**, not at the end. Even a conflict-free merge needs a dated entry.
8. **If a conflict is too risky to resolve**: stop, document in `portingguide.md`, and escalate.

## Likely Conflict Hot-spots (this window)

Inspect all Helix-touched files after the merge even if `git merge` reports "auto-merged".

### PR #65's files — new variable vs. 002100

| File | PR #65 surface | Risk | Concern |
|------|---------------|------|---------|
| `crates/acp_thread/src/connection.rs` | +14 lines — adds `fail_turn()` to `StubAgentConnection` | **LOW-MED** | `connection.rs` is an upstream file. If upstream added new methods to `AgentConnection` or `StubAgentConnection` since `e45e42af6e`, there may be a both-sides-modified-a-impl conflict. Resolve by keeping both sides' additions. Compile-gated: build will surface any missed interface change. |
| `crates/external_websocket_sync/src/thread_service.rs` | +194 lines — Error arm, ChatResponseError emit, regression test, `TEST_WEBSOCKET_SERVICE_GUARD` | NONE | Helix-only file; upstream never touches it. |
| `crates/external_websocket_sync/src/types.rs` | +15 lines — `SyncEvent::ChatResponseError` variant | NONE | Helix-only file; upstream never touches it. |

### Standard Helix-surface files (carry from every prior merge)

| File | Upstream activity (expected) | Risk | Helix concern |
|------|------------------------------|------|---------------|
| `crates/acp_thread/src/acp_thread.rs` | LOW-MED for 4-day window | MEDIUM | Critical Fixes #3/#6/#8/#9 + PR #55 emit. Re-grep `content_only`, `stopped_emitted_for_task`, `drop(turn.send_task)`, `EntryUpdated` post-merge. |
| `crates/agent/src/agent.rs` | LOW for 4-day window | LOW | Critical Fix #1 (`pending_sessions`), `wait_for_tools_ready`. |
| `crates/agent_ui/src/agent_panel.rs` | LOW-MED for 4-day window | MEDIUM | Fix 1b at `5468-5473` (FIRST statement of `BaseView::Uninitialized`) + Critical Fix #11. Line will shift — re-grep and read full `ensure_thread_initialized` body after merge. |
| `crates/agent_ui/src/conversation_view.rs` | LOW for 4-day window | LOW | `from_existing_thread()` field-set; build is the gate. |
| `crates/workspace/src/workspace.rs` | LOW for 4-day window | LOW | `CollaboratorId::Agent` follow-focus guard. |
| `crates/extensions_ui/src/extensions_ui.rs` | LOW | LOW | Three `// HELIX: External agent …` markers at lines 337/359/1629 — verify all three survive. |
| `crates/agent_servers/src/acp.rs` | LOW | LOW | PR #50: `session_creation_chain` + `_settings_subscription` at lines 438-439. |
| `crates/zed/src/main.rs` | LOW | LOW | `--allow-multiple-instances`, `--headless`, `build_application(headless: bool)`. |
| `crates/title_bar/` | LOW | LOW | `optional = true` dep + `render_restricted_mode` cfg-gated early return. |
| `crates/feature_flags/src/flags.rs` | LOW | LOW | `AcpBetaFeatureFlag::enabled_for_all() -> true`. May see new flag additions — all fine as long as the `AcpBetaFeatureFlag` impl block is not deleted. |
| `crates/settings_content/src/settings_content.rs` | LOW | LOW | Helix's `suggest_dev_container`, `helix_mode`, `auto_open_panel`, `show_onboarding`, `dev_container_use_buildkit` fields coexist. "Both-sides-added-a-field" is the historical pattern here (002100 round 1, 002029 round 2). |
| `Cargo.toml` | always | LOW | `rust-embed` `debug-embed` feature, `external_websocket_sync` + `cloud_api_types` workspace members. |
| `Cargo.lock` | always | TRIVIAL | `--theirs`. |
| `crates/external_websocket_sync/` (all other files) | 0 upstream | NONE | No upstream churn expected; verify by construction. |

## Resolution Patterns Worth Re-stating

- **`HeadlessConnection` is dead code** (since 001909). Don't be misled by old porting-guide entries.
- **`ConversationView` field set** — every merge from 002029 through 002100-extension confirmed `from_existing_thread()` field-checking is build-gated. Build green = no drift.
- **`AcpThreadEvent::Stopped(StopReason)`** is a tuple variant — pattern matches need `Stopped(_)`. Test code is not exempt.
- **`BaseView::*` arms** must remain exhaustive — Helix UI state queries in `agent_panel.rs::new()` and `zed/src/main.rs` headless responder.
- **`ContextServerStatus::*` arms** likewise — `ClientSecretRequired { .. }` was added in 002029.
- **`title_bar`'s `external_websocket_sync` dep MUST be `optional = true`** with the matching `[features]` entry.
- **Auto-merged ≠ correct** — always grep for renamed identifiers after the merge.
- **Build via `./stack build-zed dev`** in the Helix repo (~46s–1m31s warm cache; cold can be 16+ minutes). No local Rust toolchain in this environment.
- **`go mod tidy` in `e2e-test/helix-ws-test-server/`** before E2E — the runner doesn't tidy itself.
- **GPUI events flush at end of entity update closure** (lesson from 001996 Phase 13 fix).
- **PR #60 retry loop** in `thread_service.rs::handle_follow_up_message` — 4 attempts × 750ms backoff on `ede_diagnostic`. Phase 9 is the regression gate.
- **PR #65 `TEST_WEBSOCKET_SERVICE_GUARD`** — the crash-regression test and the reconnect test share this mutex; `cargo test -p external_websocket_sync` must not deadlock.
- **Never use `--no-build` when investigating E2E failures** — learned 002100-extension. When the Go handler code may have changed, always do a full rebuild.

## Post-Merge Validation

### 1. Compile check
```bash
cd /home/retro/work/zed
cargo check -p zed                                          # no features (if local rust)
cargo check -p zed --features external_websocket_sync       # with Helix gate (if local rust)
cd /home/retro/work/helix
./stack build-zed dev                                       # Docker-based, produces ./zed-build/zed
```

### 2. Grep verification of critical fixes / silent drift

```bash
cd /home/retro/work/zed

# PR #65 (new since 002100) — verify all three files
grep -n "fail_turn"                           crates/acp_thread/src/connection.rs           # StubAgentConnection method
grep -n "ChatResponseError\|chat_response_error" crates/external_websocket_sync/src/types.rs crates/external_websocket_sync/src/thread_service.rs
grep -n "TEST_WEBSOCKET_SERVICE_GUARD"        crates/external_websocket_sync/src/thread_service.rs  # shared by crash + reconnect tests

# Critical fixes
grep -n "load_session\|pending_sessions"      crates/agent/src/agent.rs                     | head
grep -n "content_only"                        crates/acp_thread/src/acp_thread.rs
grep -n "drop(turn.send_task)"                crates/acp_thread/src/acp_thread.rs
grep -n "stopped_emitted_for_task"            crates/acp_thread/src/acp_thread.rs
grep -rn "unregister_thread"                  crates/agent_ui/src/conversation_view.rs
grep -n "ThreadMetadataStore\|load_agent_thread" crates/agent_ui/src/agent_panel.rs         # Critical Fix #11

# PR #50 (serialize ACP session creation) — must coexist with _settings_subscription
grep -n "session_creation_chain"              crates/agent_servers/src/acp.rs
grep -n "_settings_subscription"              crates/agent_servers/src/acp.rs

# PR #55 (streaming-reveal EntryUpdated emit)
grep -n "EntryUpdated"                        crates/acp_thread/src/acp_thread.rs           # expect 16 occurrences

# PR #56 (Fix 1a deferred UserCreatedThread + Fix 1b draft suppression)
grep -n "ensure_thread_initialized"           crates/agent_ui/src/agent_panel.rs
# Read the FULL function body — the Helix cfg-gated `return;` MUST be the FIRST statement
# of the BaseView::Uninitialized branch.
grep -rn "defer.*UserCreatedThread\|first_user_message" crates/external_websocket_sync/

# PR #60 retry loop
grep -n "ede_diagnostic\|handle_follow_up_message" crates/external_websocket_sync/src/thread_service.rs

# PR #63 wedge recovery
grep -n "force_reset_session\|clear_keep_alive\|agent_name" crates/external_websocket_sync/src/thread_service.rs

# PR #64 agent_ready re-emit
grep -n "agent_ready" crates/external_websocket_sync/src/thread_service.rs

# fd26c1a113 (Dockerfile.ci helix-org)
grep -n "helix-org"                           crates/external_websocket_sync/e2e-test/Dockerfile.ci

# PR #57 (Go test-server Phase 16 counter fix)
grep -n "phase10\|Phase 10's own"             crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go

# Helix bypass markers (line numbers shift each merge)
grep -n "HELIX: External agent"               crates/extensions_ui/src/extensions_ui.rs     # expect 3 hits (currently 337/359/1629)

# Test-pattern drift
grep -nE "AcpThreadEvent::Stopped\b([^(]|$)"  crates/acp_thread/src/                        # should be 0

# Upstream rename silent drift
grep -rn "ActiveView"                         crates/agent_ui/src/   # only AgentPanelEvent::ActiveView*
grep -rn "set_active_view"                    crates/agent_ui/src/   # should be 0
grep -rn "draft_threads\|background_threads"  crates/agent_ui/src/   # should be 0
grep -rn "selected_agent_type"                crates/agent_ui/src/   # should be 0

# Carry-over fixes
grep -n "allow_multiple_instances\|headless\|build_application" crates/zed/src/main.rs
grep -n "debug-embed"                         Cargo.toml
grep -n "smol::Timer"                         crates/agent/src/agent.rs                      # should be 0

# Helix-specific surface
grep -n "CollaboratorId::Agent"               crates/workspace/src/workspace.rs
grep -n "render_restricted_mode"              crates/title_bar/src/title_bar.rs
grep -n "AcpBetaFeatureFlag\|enabled_for_all" crates/feature_flags/src/flags.rs

# Workspace member sanity
grep -n "external_websocket_sync\|cloud_api_types" Cargo.toml
```

### 3. Walk the rebase checklist
Step through every numbered item in `portingguide.md` §"Rebase Checklist". Pay special attention to:
- Item 9 (`agent_panel.rs` cfg-gated blocks) — Fix 1b first-statement invariant
- Item 11 (`ConversationView` / `ConnectedServerState`) — field-set drift (build is the gate)
- Items 31, 31a, 37 (`acp_thread.rs` cancel/Stopped) — `stopped_emitted_for_task` invariant
- PR #65 `connection.rs` — `fail_turn` still compiles after any upstream `AgentConnection` changes

### 4. Unit tests (if local Rust toolchain available)
```bash
cargo test -p external_websocket_sync          # full pass, ≤2 env-dependent ignored acceptable
                                                # — PR #65 crash-regression test and reconnect test
                                                # must both pass (TEST_WEBSOCKET_SERVICE_GUARD)
cargo test -p acp_thread test_second_send      # Stopped invariant (Critical Fix #6)
cargo test -p agent_servers test_concurrent_session_creation_is_serialized   # PR #50
```

### 5. E2E test (the canonical regression check — **hard gate**)
```bash
cd /home/retro/work/helix && ./stack build-zed dev
cp ./zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary
cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test
(cd helix-ws-test-server && go mod tidy)            # per 001980/002077 lesson — runner doesn't tidy
./run_docker_e2e.sh                                  # zed-agent only
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh    # both agents — always full rebuild
```

All 17 phases must pass for **both** `zed-agent` and `claude`. Special gates:
- **Phase 9** (rapid 3-turn cancel) — PR #60's retry-loop survival; API-latency flake documented, one retry permitted
- **Phase 13** (`cancel_current_turn` happy path — `turn_cancelled` ordering)
- **Phase 15** (streaming patches arrive incrementally) — PR #55 emit gate
- **Phase 16** (zero spontaneous `UserCreatedThread`) — PR #56 Fix 1a + PR #57
- **Phase 17** (live Claude process count == real thread count) — PR #56 Fix 1b

If any phase fails: do **not** mark the task complete. Diagnose, fix, document in `portingguide.md`, re-run.

One retry permitted for Claude Phase 1 npm-install bootstrap flake; one retry permitted for Phase 9 API-latency flake. Never use `--no-build` when investigating a failure.

## Documentation Updates (incremental — not retrospective)

After **each** non-trivial conflict resolution, append to `/home/retro/work/zed/portingguide.md`:

1. **Conflict context**: file + region + which Helix concern it touches
2. **Upstream change**: the PR or commit and what it did
3. **Resolution**: what was kept from each side and why
4. **Risk**: any follow-up regression risk

Add new top-level section `## Merge 002153 (2026-06-22)` at the top of the merge-history list, mirroring the 002100 structure.

**Minimum subsections (regardless of how the merge auto-resolves)**:

1. **Window summary** — "~N upstream commits over 4 days" (fill N at execution time).
2. **PR #65 survival check** — `fail_turn` intact in `connection.rs`; Error arm intact in `thread_service.rs`; `SyncEvent::ChatResponseError` intact in `types.rs`; `TEST_WEBSOCKET_SERVICE_GUARD` present.
3. **Helix-surface auto-merge survival check** — per-area confirmation (Fix 1b position; three `// HELIX:` bypass markers; `external_websocket_sync/` untouched by upstream; all critical fixes intact).
4. **PR #60/#63/#64 survival check** — confirm `ede_diagnostic` retry block and wedge-recovery surface intact.
5. **Cargo.toml / Cargo.lock notes** — record any new upstream deps or version bumps.

If any of the above actually conflicted (rather than auto-merging), upgrade the subsection to a "Conflicts and Resolutions" entry with the chosen resolution and rationale.

Always extend the commit-history table at the bottom with this merge's commits and any follow-up fix commits.

## Helix Repo Bump

After the Zed branch is pushed:
1. In `/home/retro/work/helix/`, update `sandbox-versions.txt` `ZED_COMMIT=` from `9546054e68e2b771ac63e55821a70654684ac651` to the new merge HEAD.
2. Commit on a `feature/002153-merge-latest-zed` branch in helix.
3. Push the helix branch.
4. The Helix UI handles PR creation per task convention — do not open PRs from the agent.

## Open Questions (for the implementation agent to answer at runtime)

- What is the upstream HEAD SHA and how many commits are in `e45e42af6e..upstream/main`? (Confirm at `git fetch upstream` time.)
- Has upstream modified `crates/acp_thread/src/connection.rs` since `e45e42af6e`? (Most likely no; check if `StubAgentConnection` or `AgentConnection` trait changed — affects PR #65's `fail_turn` integration.)
- Has upstream added new `BaseView` or `ContextServerStatus` variants since `e45e42af6e`? (Very unlikely for a 4-day window, but build failure is the safety net.)
- Did `from_existing_thread()` drift from `ConversationView::new`? (Build is the gate; zero drift in the last three merges.)
- Did anyone push to fork main during merge work? (Re-fetch `origin/main` before declaring done — prior merges all picked up at least one out-of-band push.)
- Has `portingguide.md` been updated for 002100-extension through 2026-06-18? (Yes — already confirmed at 1109 lines with the 002100-extension entry at line 670.)

## Notes

### `stack` is the canonical builder
No local Rust toolchain in this environment — all builds go via `cd /home/retro/work/helix && ./stack build-zed dev` (Docker-based, persistent cache). `cargo check` / `cargo test` items are **best-effort**; the E2E gate is the hard contractual requirement.

### E2E phase count is 17
Phases 1–14 from 001996; 15–17 added by PR #55/#56/#57. PR #65 added a unit test (not an E2E phase). Phase 9 is the explicit regression gate for PR #60; Phase 17 is the explicit gate for PR #56 Fix 1b.

### Out-of-band fork pushes
Every prior merge in this series picked up at least one out-of-band push to fork main while the branch was open. Re-fetch `origin/main` before declaring done and merge if needed.

### Branch naming
Use `feature/002153-merge-latest-zed`. Do not reuse any earlier branch name.

## Out of Scope

- Changes to Helix API, frontend, or other non-Zed components unless forced by a breaking upstream change.
- New feature development beyond what conflict resolution requires.
- Rewriting or refactoring Helix-specific crates.
- Wiring upstream features (compaction, sandboxing, etc.) into Helix workflows.
