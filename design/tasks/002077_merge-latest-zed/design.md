# Design: Merge Latest Zed Upstream (002077)

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` — fork remote `origin` (in-cluster gitea URL is `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`). The in-cluster URL **is** the fork; there is no separate origin/fork distinction.
- **Upstream**: `zed-industries/zed`. The `upstream` remote is **not configured** in fresh clones — add it: `git remote add upstream https://github.com/zed-industries/zed.git`.
- **Porting guide** (canonical, living document): `/home/retro/work/zed/portingguide.md` — exists, **892 lines** as of start of this task with the latest entry `## Merge 002029-extension round 2 (2026-06-02)` at line 750.
- **Helix platform repo**: `/home/retro/work/helix/` — `sandbox-versions.txt` carries `ZED_COMMIT=79b9bfb1d60cbef5b14ba7e3992ba5e8f6eb335c`. This is **2 Helix-only commits behind fork main** (PR #60 has not yet been bumped into the sandbox image). The bump must move to the new 002077 merge HEAD — which will also ship PR #60.

## Current State (as of 2026-06-12)

| | Commit | Date |
|---|---|---|
| Fork HEAD | `ecdc2ea67d` ("Merge pull request #60 from helixml/fix/thread-service-claude-acp-drain-retry") | 2026-06-09 |
| Last upstream merge fence | `9d50bab893` ("git_ui: Add total diff stats to git panel (#58018)") — 002029-extension round 2 | 2026-06-02 |
| Upstream HEAD | `992f395c3d` ("editor: Fix columnar selection alignment on rows with multi-byte chars (#57097)") | 2026-06-12 |
| Helix-only commits since 002029 | **2** (PR #60: `27e8867c9e` retry, `e4c36d837c` cleanup, both in `external_websocket_sync/src/thread_service.rs`) | 10 days |
| Upstream commits to merge | **256** | 10 days |

Recent merge precedent (size → conflict count):
- 002029: 261 commits, 10 days, **7** manual conflicts (incl. Fix 1b position-critical), 3 build-fix commits
- 002029-extension: 287 commits, 3 days, **0** manual conflicts (`ort` auto-resolved), 1 signature-drift repair
- 002029-extension round 2: 242 commits, 8 days, **4** manual conflicts ("both sides added a field"), 3 signature-drift repairs
- 002077 outlook: **256 commits, 10 days** — closest in shape to 002029. Expect **4–8 manual conflicts** and **3–5 signature-drift repairs**. The compaction cluster (+1700 net lines concentrated in `agent.rs`) and `d7ac5e6cf4`'s tool-call-status rewrite (+602 lines including the core `acp_thread.rs`) are the dominant new variables not present in 002029.

## Merge Strategy

Use `git merge upstream/main` (consistent with every previous merge — preserves Helix commit history and makes conflict resolution traceable). Do **not** rebase.

```bash
cd /home/retro/work/zed
git remote add upstream https://github.com/zed-industries/zed.git   # only if missing
git fetch upstream
git checkout main && git pull origin main                            # in case main moved
git checkout -b feature/002077-merge-latest-zed
git merge upstream/main
# Resolve conflicts one at a time, updating portingguide.md as each is resolved.
# Build → critical-fix grep → unit tests → E2E (Phase 9 for PR #60, Phase 17 for Fix 1b).
git push -u origin feature/002077-merge-latest-zed
# Write pull_request_zed.md + pull_request_helix.md; bump ZED_COMMIT in helix.
```

### Conflict-resolution principles (carried from 001864 / 001909 / 001980 / 001996 / 002029)

1. **Prefer upstream ordering** for shared changes (model lists, feature lists, match arms) — keeps the diff small and minimises future conflict surface.
2. **Keep Helix-specific code behind `#[cfg(feature = "external_websocket_sync")]`** — never let it leak into unconditional code paths.
3. **`Cargo.lock`**: always accept upstream (`git checkout --theirs`); regenerated on next `cargo build --features external_websocket_sync`.
4. **CI / `.github/workflows/`**: always accept upstream — Helix doesn't use Zed's CI.
5. **Upstream renames**: after every merge — even when files auto-merge cleanly — grep for the old identifiers in cfg-gated regions.
6. **Trait-signature changes**: walk all impls compile-driven; the post-merge build is the safety net.
7. **Document every non-trivial resolution in `portingguide.md` immediately**, not at the end.
8. **If a conflict is too risky to resolve**: stop, document explicitly in `portingguide.md`, and escalate to the user.

## Likely Conflict Hot-spots

Upstream diff sizes against Helix-touched files (`9d50bab893..upstream/main`, measured 2026-06-12). Inspect each even if `git merge` reports "auto-merged":

| File | Commits | +/- lines | Risk | Helix concern |
|------|---------|-----------|------|---------------|
| `crates/acp_thread/src/acp_thread.rs` | **12** | **+1102 / -81** | **VERY HIGH** | `d7ac5e6cf4` "Preserve waiting tool call status on updates (#58537)" alone is +602 lines across 6 files and reworks how `ToolCall::status` updates flow through entry updates — direct overlap with PR #55's streaming-reveal `EntryUpdated` emit and Critical Fix #6 (`stopped_emitted_for_task` invariant) territory. Multiple compaction-cluster commits also touch this file. Inspect every conflict. |
| `crates/agent/src/agent.rs` | **12** | **+765 / -199** | **HIGH** | Critical Fix #1 (entity-lifetime clone in `load_session`) and `wait_for_tools_ready` live here. New compaction cluster: `e5052961af` "/compact slash command" (+1065 lines, multi-file), `9baefe701e` "auto_compact agent settings", `e17e272d24` "compaction UX", `5c90b0664f` "compaction-cancel race fix" (+97), `620ceaaaca` "flush thread content to database on app quit" (+103). All exercise the same `Thread` state machine PR #50 and Fix #1 sit on top of. Plus `27191913e9` cumulative-token-usage accumulation. |
| `crates/agent_ui/src/agent_panel.rs` | **10** | **+731 / -305** | **HIGH** | `116e4bc184` "Inherit source agent without draft content" + `c486f6f529` "title display iteration" + `638b33ca2b` "placeholder title in empty-state toolbar" + `c78bd36fd8` "keep pending subagent edits when regenerating a prompt". Fix 1b's `#[cfg(feature = "external_websocket_sync")] { return; }` must remain the FIRST statement of `BaseView::Uninitialized`. Critical Fix #11 entity-identity guard (now `thread_id`-based) must survive. |
| `crates/agent_ui/src/conversation_view.rs` | **11** | **+334 / -8** | **MEDIUM-HIGH** | `from_existing_thread()` signature-drift magnet — repaired in 002029, 002029-extension, and 002029-extension round 2; expect a fourth round. `0e9e8d0e68` compaction refinements + `e17e272d24` UX + `8432a26a9d` thinking-toggle fix all land here. |
| `crates/workspace/src/workspace.rs` | **9** | **+430 / -102** | **MEDIUM-HIGH** | `215ca2fb0b` "Typed workspace errors (#57649)" still dominant — Helix `show_error` call sites need migration to `<E: WorkspaceError>`. `83aa943705` "Fix overflow in error popup (#59185)" follow-up may tighten the API further. `a32999e00b` "Update window title (#58401)" adds `Rc<Cell<EntityId>>` for multi-workspace tracking. |
| `crates/agent_servers/src/acp.rs` | **3** | **+5 / -92** | LOW | Small upstream cleanup. PR #50 `session_creation_chain` + `_settings_subscription` should survive cleanly. `56b71271c4` "Enable ACP session usage and deletion features" — trait-default check. |
| `crates/zed/src/main.rs` | **3** | **+11 / -14** | LOW | `--allow-multiple-instances`, `--headless`, `build_application(headless: bool)` (002029-round-2) must survive. |
| `crates/title_bar/` | 4 | small | LOW | `external_websocket_sync = { workspace = true, optional = true }` + cfg-gated `render_restricted_mode` preserved. |
| `crates/extensions_ui/src/extensions_ui.rs` | 1+ | small | LOW | `// HELIX: External agent ...` bypass markers retained (~221, ~243, ~1513). |
| `crates/feature_flags/src/flags.rs` | small | small | LOW | `AcpBetaFeatureFlag::enabled_for_all() -> true` safe. May see new flags from `88e5c6d2fa` / `d24b14a26c`. |
| `crates/external_websocket_sync/` (incl. `thread_service.rs`) | (Helix-only) | 0 upstream | LOW | **PR #60 retry loop in `handle_follow_up_message` (4×750ms backoff for `ede_diagnostic`) is load-bearing** — must not be lost. No upstream churn. Likely site for new `WorkspaceError` impl wrappers if `215ca2fb0b` migration requires them. |
| `crates/agent_settings/src/agent_settings.rs` + `crates/settings_content/src/agent.rs` | 2+ | small | LOW-MED | `89cac4944d` extends `sandbox_permissions`; `9baefe701e` adds `auto_compact`. Three-way coexistence with Helix's `show_onboarding` / `auto_open_panel` — "both sides added a field" merges. |
| `Cargo.lock` | always | always | TRIVIAL | `--theirs`. |

### Highest-risk single change: upstream `d7ac5e6cf4` "acp_thread: Preserve waiting tool call status on updates (#58537)" — NEW since 06-08

PR description: "Reworks how `ToolCall::status` updates flow through entry updates so that 'waiting' status is preserved across update bursts." +602 lines across 6 files including the core `acp_thread.rs`. **Direct overlap with both** PR #55's 12-line streaming-reveal `EntryUpdated` emit **and** Critical Fix #6's `stopped_emitted_for_task` territory.

Two specific risks:

1. **PR #55 emit displacement**: the streaming-reveal `EntryUpdated` site may now be replaced by upstream's new tool-call-status emit. Verify the WS sync layer still receives an event when a streaming-reveal completes — Phase 15 of the E2E is the runtime gate.
2. **Critical Fix #6 invariant**: the `stopped_emitted_for_task` guard exists because the original `acp_thread.rs` had paths that could emit `Stopped` twice. `d7ac5e6cf4` adds new emit paths in the same state machine; re-verify only one `Stopped(_)` per `send()` survives. `cargo test -p acp_thread test_second_send` is the local invariant check.

After resolution: read the post-merge `run_turn` body end-to-end and trace every emit site.

### High-risk single cluster: Compaction (`e5052961af`, `9baefe701e`, `e17e272d24`, `5c90b0664f`, `0bc6c76fcf`, `0e9e8d0e68`) — NEW since 06-08

Six commits totalling ~1700 net lines, concentrated in `crates/agent/src/agent.rs`, `crates/agent_settings/`, `crates/settings_content/`, and `crates/agent_ui/src/conversation_view.rs`. Introduces the `/compact` slash command, `auto_compact` settings field, compaction UI refinements, and a fix for a compaction-cancellation race.

Three risks for Helix:

1. **WS payload schema**: compaction emits new event types and modifies `Thread` state. The Helix WS sync layer marshals `Thread` state to the API server. Inspect `external_websocket_sync/` for any `compact`/`Compact`/`compaction`-related fields the payload now carries, and document any schema change.
2. **`auto_compact` settings**: another "both sides added a field" three-way coexistence with `show_onboarding`/`auto_open_panel`/`sandbox_permissions` in agent_settings/settings_content. Trivial if independent; check.
3. **Compaction-cancel race fix (`5c90b0664f`)**: +97 lines patching a "compaction marked as cancelled" race. Critical Fixes #6/#8/#9 are in the same family. Verify the upstream race-fix doesn't reintroduce a double-`Stopped` emit or shadow Fix #6's `stopped_emitted_for_task` invariant.

### High-risk single change: upstream `620ceaaaca` "agent: Flush thread content to database on app quit (#58962)" — NEW since 06-08

+103 lines in `agent/src/agent.rs`. Adds shutdown-time persistence to `threads.db`. The Helix WS sync layer is the authoritative store; if the flush-on-quit path runs in Helix builds and stomps on or duplicates WS-managed state, the next session-restart could see split-brain.

**Decision required during the merge**:
- If the flush is harmless (writes to `threads.db` which Helix mode ignores), no action.
- If it races with WS state, gate behind `not(feature = "external_websocket_sync")` and document.

Inspect the flush path's reachability under `external_websocket_sync` and the test fixtures exercising it before deciding.

### High-risk single change: upstream `215ca2fb0b` "Typed workspace errors (#57649)" + follow-up `83aa943705` (#59185) — carried from 06-08

Migrates `Workspace::show_error` to `<E: WorkspaceError>` generic. Helix call sites that currently pass a string or `anyhow::Error` will break the build until migrated. The follow-up `83aa943705` "Fix overflow in error popup" may further tighten the API.

Migration options:
- **Option 1**: implement `WorkspaceError` for a Helix-side error type (cleanest if there's a Helix error enum).
- **Option 2**: wrap the string in a tiny ad-hoc `WorkspaceError` impl per call site (smallest diff).
- **Option 3**: use whatever convenience constructor upstream provides (preferred if it exists — minimises Helix surface).

Document the chosen approach as a "Pre-existing Breakage Repaired" entry. Build-driven discovery via `./stack build-zed dev` enumerates every site.

### Medium-risk single change: upstream `116e4bc184` "agent_ui: Inherit source agent without draft content (#58636)" — carried from 06-08

Touches the draft-inheritance path that intersects Helix PR #56 Fix 1b. Re-verify Fix 1b is still the FIRST statement of `BaseView::Uninitialized` after the merge — Phase 17 is the runtime gate.

### Medium-risk single change: upstream `27191913e9` "agent: Cumulative token usage (#58378)" — carried from 06-08

Revives `Thread::cumulative_token_usage` accumulation + persistence. Compounded by the compaction cluster's token-handling changes (`0bc6c76fcf` "Hide token usage after /compact"). Schema-drift check on WS payloads is mandatory.

### Medium-risk single change: upstream `56b71271c4` "acp: Enable ACP session usage and deletion features (#58680)" — carried from 06-08

Stabilises context windows + persistent session/delete. Confirm no default-flip the Helix `AcpConnection` impl needs to override.

### Low-risk single change: upstream `fef979dec4` "language_models: Add Anthropic-compatible provider support in settings (#50381)" — NEW since 06-08

Provider plumbing. Adjacent to Helix's enterprise-TLS-skip and built-in-agent-hiding patches; re-grep after merge.

### Low-risk single change: upstream `a32999e00b` "workspace: Update window title when switching active workspace (#58401)" — carried from 06-08

Shared `Rc<Cell<EntityId>>` between member workspaces. Re-grep `CollaboratorId::Agent` follow-focus guard.

### Low-risk single change: upstream `89cac4944d` "Improve sandbox write-path handling (#58283)" — carried from 06-08

Extends `sandbox_permissions`. "Both sides added a field" coexistence.

## Resolution Patterns Worth Re-stating

- **`HeadlessConnection` is dead code** (since 001909). Don't be misled by old porting-guide entries.
- **`ConversationView` field set grew in 002029-round-2** (added `code_span_resolver`, `last_theme_id`, `draft_prompt_persist_task`, `available_skills`, dropped `prompt_store`). `from_existing_thread()` silently breaks if upstream widens it further — every recent merge required a repair. Diff upstream `ConversationView::new()` field-by-field against post-merge `from_existing_thread()`.
- **`AcpThreadEvent::Stopped(StopReason)`** is a tuple variant — pattern matches need `Stopped(_)`. Test code is not exempt.
- **`BaseView::*` arms** must remain exhaustive — Helix UI state queries in `agent_panel.rs::new()` and `zed/src/main.rs` headless responder.
- **`ContextServerStatus::*` arms** likewise — `ClientSecretRequired { .. }` was added in 002029.
- **`title_bar`'s `external_websocket_sync` dep MUST be `optional = true`** with the matching `[features]` entry.
- **Auto-merged ≠ correct** — always grep for renamed identifiers after the merge.
- **Build via `./stack build-zed dev`** in the Helix repo (~46s–1m31s warm cache). No local Rust toolchain in this environment.
- **`go mod tidy` in `e2e-test/helix-ws-test-server/`** before E2E — the runner doesn't tidy itself.
- **GPUI events flush at end of entity update closure** (lesson from 001996 Phase 13 fix).
- **`build_application(headless: bool)` pattern** (002029-round-2) — re-verify the parameter still threads through if upstream refactors `main.rs`.
- **PR #60 retry loop** in `external_websocket_sync/src/thread_service.rs::handle_follow_up_message` — 4 attempts × 750ms backoff on `ede_diagnostic` transient. New as of 2026-06-09. Same race class as `acp_thread::AcpThread::cancel`'s existing workaround. Phase 9 of the E2E is the regression gate.

## Post-Merge Validation

### 1. Compile check
```bash
cd /home/retro/work/zed
cargo check -p zed                                          # no features (if local rust)
cargo check -p zed --features external_websocket_sync       # with Helix gate (if local rust)
cd /home/retro/work/helix
./stack build-zed dev                                       # ~46s-1m31s warm cache, produces ./zed-build/zed
```

### 2. Grep verification of critical fixes / silent drift

```bash
cd /home/retro/work/zed

# Critical fixes (Helix)
grep -n "load_session"               crates/agent/src/agent.rs                              | head
grep -n "content_only"               crates/acp_thread/src/acp_thread.rs
grep -n "drop(turn.send_task)"       crates/acp_thread/src/acp_thread.rs
grep -n "stopped_emitted_for_task"   crates/acp_thread/src/acp_thread.rs
grep -rn "unregister_thread"         crates/agent_ui/src/conversation_view.rs
grep -n "external_websocket_sync::get_thread" crates/agent_ui/src/agent_panel.rs   # Critical Fix #11

# PR #50 (serialize ACP session creation) — must coexist with _settings_subscription
grep -n "session_creation_chain"     crates/agent_servers/src/acp.rs
grep -n "_settings_subscription"     crates/agent_servers/src/acp.rs
grep -n "test_concurrent_session_creation_is_serialized" crates/agent_servers/src/acp.rs

# PR #55 (streaming-reveal EntryUpdated emit) — survival vs d7ac5e6cf4
grep -n "EntryUpdated"               crates/acp_thread/src/acp_thread.rs

# PR #56 (Fix 1a deferred UserCreatedThread + Fix 1b draft suppression)
grep -n "ensure_thread_initialized"  crates/agent_ui/src/agent_panel.rs
# Read the FULL function body — the Helix cfg-gated `return;` MUST be the FIRST statement
# of the BaseView::Uninitialized branch, before any source-agent-inheritance or
# terminal-spawn branches that 116e4bc184 or any other commit may have added.
grep -rn "defer.*UserCreatedThread\|first_user_message"  crates/external_websocket_sync/

# PR #60 retry loop (NEW as of 2026-06-09)
grep -n "ede_diagnostic\|handle_follow_up_message"   crates/external_websocket_sync/src/thread_service.rs
# Must show the 4×750ms backoff retry loop intact.

# fd26c1a113 (direct Dockerfile.ci fix)
grep -n "helix-org"                   crates/external_websocket_sync/e2e-test/Dockerfile.ci

# PR #57 (Go test-server Phase 16 counter fix)
grep -n "phase10\|Phase 10's own"    crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go

# Test-pattern drift (lesson from 001980 — checklist 41a)
grep -nE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/   # should be 0

# Upstream rename silent drift
grep -rn "ActiveView"                crates/agent_ui/src/   # only AgentPanelEvent::ActiveView*
grep -rn "set_active_view"           crates/agent_ui/src/   # should be 0
grep -rn "draft_threads\|background_threads" crates/agent_ui/src/   # should be 0
grep -rn "selected_agent_type"       crates/agent_ui/src/   # should be 0

# 215ca2fb0b typed workspace errors — every Helix show_error site must use the new signature
grep -rn "Workspace::show_error\|workspace.show_error\|\.show_error(" crates/external_websocket_sync/ crates/agent_ui/src/

# 27191913e9 cumulative token usage + compaction cluster — schema drift check on WS payloads
grep -rn "cumulative_token_usage\|TokenUsage\|compact\|Compact\|compaction" crates/external_websocket_sync/

# 620ceaaaca flush-on-quit — interaction with Helix WS-authoritative store
grep -rn "flush.*db\|on_app_quit\|shutdown" crates/agent/src/agent.rs

# 56b71271c4 acp session usage/deletion stabilisation — Helix override sound
grep -n "fn supports_delete\|fn supports_session_usage\|fn delete_session\|fn context_window" crates/agent_servers/src/acp.rs crates/agent/src/agent.rs

# Confirm AcpBetaFeatureFlag + new feature flags
grep -n "AcpBetaFeatureFlag\|enabled_for_all"  crates/feature_flags/src/flags.rs

# Helix's HELIX: bypass markers in extensions_ui
grep -n "HELIX: External agent"      crates/extensions_ui/src/extensions_ui.rs

# Carry-over fixes
grep -n "allow_multiple_instances\|headless\|build_application" crates/zed/src/main.rs
grep -n "debug-embed"                Cargo.toml
grep -n "smol::Timer"                crates/agent/src/agent.rs   # should be 0

# Helix-specific surface
grep -n "CollaboratorId::Agent"      crates/workspace/src/workspace.rs
grep -n "render_restricted_mode"     crates/title_bar/src/title_bar.rs
```

### 3. Walk the rebase checklist
Step through every numbered item in `portingguide.md` §"Rebase Checklist" (currently 44+ items, including 002029's additions on Fix 1b position and `supports_delete(&self, &App)` signature). Pay special attention to:
- Item 9 (`agent_panel.rs` cfg-gated blocks) — Fix 1b position regression risk
- Item 11 (`ConversationView` / `ConnectedServerState`) — recheck struct; every recent merge needed a repair
- Items 31, 31a, 37 (`acp_thread.rs` cancel/Stopped) — `d7ac5e6cf4` + compaction-cancel race fix risk
- The 002029 additions on Fix 1b first-statement and `supports_delete(&self, &App)` signature

### 4. Unit tests (if local Rust toolchain available)
```bash
cargo test -p external_websocket_sync          # full pass, ≤2 ignored
cargo test -p acp_thread test_second_send      # Stopped invariant (Critical Fix #6 vs d7ac5e6cf4)
cargo test -p agent_servers test_concurrent_session_creation_is_serialized   # PR #50
```

### 5. E2E test (the canonical regression check — **hard gate**)
```bash
cd /home/retro/work/helix && ./stack build-zed dev
cp ./zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary
cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test
(cd helix-ws-test-server && go mod tidy)            # per 001980 lesson — runner doesn't tidy
./run_docker_e2e.sh                                  # zed-agent only
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh    # both agents
```

All 17 phases must pass for **both** `zed-agent` and `claude`. Special gates this round:
- **Phase 9** (rapid 3-turn cancel) — explicit gate for **PR #60's retry-loop** survival against the `ede_diagnostic` race
- **Phase 13** (`cancel_current_turn` happy path — `turn_cancelled` ordering)
- **Phase 15** (streaming patches arrive incrementally — gates PR #55 surviving `d7ac5e6cf4`'s tool-call-status rewrite)
- **Phase 16** (zero spontaneous `UserCreatedThread` — gates PR #56 Fix 1a + PR #57)
- **Phase 17** (live Claude process count == real thread count — **gates PR #56 Fix 1b draft suppression survived the merge**)

If any phase fails: do **not** mark the task complete. Diagnose, fix, document in `portingguide.md`, re-run.

One retry permitted for Claude Phase 1 npm-install bootstrap flake (lesson from 001996).

## Documentation Updates (incremental — not retrospective)

After **each** non-trivial conflict resolution, append to `/home/retro/work/zed/portingguide.md`:

1. **Conflict context**: file + region + which Helix concern it touches
2. **Upstream change**: the PR or commit and what it did
3. **Resolution**: what was kept from each side and why
4. **Risk**: any follow-up regression risk

Add new top-level section `## Merge 002077 (2026-06-12)` at the top of the merge-history list, mirroring the 002029-extension round 2 structure.

**Mandatory subsections (regardless of how the merge auto-resolves)**:

1. **"`d7ac5e6cf4` Preserve waiting tool call status — PR #55 emit + Critical Fix #6 invariant"** — document where PR #55's `EntryUpdated` emit lives post-merge and confirm `stopped_emitted_for_task` still enforces exactly-once `Stopped`.
2. **"Compaction cluster (`e5052961af` et al.) — WS payload schema check"** — record whether the cluster added new payload fields and how Helix mode reacts.
3. **"`620ceaaaca` Flush-on-quit — Helix WS-authoritative store interaction"** — record the reachability analysis and whether a `not(external_websocket_sync)` gate was added.
4. **"`215ca2fb0b` Typed workspace errors — Helix `show_error` call-site migration"** — list each Helix call site and the chosen migration approach (impl `WorkspaceError` / ad-hoc wrap / upstream convenience).
5. **"`116e4bc184` Inherit source agent without draft content vs Helix PR #56 Fix 1b"** — confirm Fix 1b's first-statement position survived.
6. **"`27191913e9` + `0bc6c76fcf` Cumulative + post-compact token usage — WS schema check"** — combined token-usage schema review.
7. **"PR #60 (`27e8867c9e`/`e4c36d837c`) `ede_diagnostic` retry-loop — survival check"** — confirm the retry block in `handle_follow_up_message` is intact; document any new event path that bypasses it.

Always extend the commit-history table at the bottom with this merge's commits and any follow-up fix commits.

**Likely new rebase-checklist additions** (only if confirmed in this merge):
- "All Helix `Workspace::show_error` call sites use the new `<E: WorkspaceError>` generic signature (002077)."
- "PR #60 `handle_follow_up_message` retains the 4×750ms `ede_diagnostic` retry block; Phase 9 of the E2E is the regression gate (002077)."
- "If the compaction cluster introduced new WS payload fields, document the schema bump and confirm the Helix API server tolerates them (002077)."

## Helix Repo Bump

After the Zed branch is pushed:
1. In `/home/retro/work/helix/`, update `sandbox-versions.txt` `ZED_COMMIT=` from `79b9bfb1d60cbef5b14ba7e3992ba5e8f6eb335c` to the new merge HEAD. **This bump also ships PR #60's retry loop**, which has been on fork main since 2026-06-09 but never bumped into the sandbox.
2. Commit on a `feature/002077-merge-latest-zed` branch in helix.
3. Push the helix branch.
4. The Helix UI handles PR creation per task convention — do not open PRs from the agent.

## Open Questions (for the implementation agent to answer at runtime)

- Where does PR #55's streaming-reveal `EntryUpdated` emit end up after `d7ac5e6cf4`'s 602-line tool-call-status rewrite? Does `stopped_emitted_for_task` still enforce exactly-once `Stopped`?
- Does the compaction cluster (`e5052961af` et al.) push `compact`/`Compact`/`compaction` fields into any payload the Helix WS sync layer marshals?
- Does `620ceaaaca`'s flush-on-quit path run under `external_websocket_sync` builds? If yes, does it race the WS-authoritative store?
- How many Helix `Workspace::show_error` call sites exist, and what's the cleanest migration shape?
- Does `116e4bc184` rewrite the `ensure_thread_initialized` body or add a code path that bypasses Fix 1b?
- Does `56b71271c4` flip any default the Helix `AcpConnection` impl needs to override?
- Are any silent-drift identifiers re-appearing in the +731-line `agent_panel.rs` diff?
- Did upstream add new `BaseView` / `ContextServerStatus` variants?
- Did upstream grow `ConversationView` past its current field set since 002029-round-2?
- Did the `agent-client-protocol` schema crate add new builder patterns or `#[non_exhaustive]` markers?
- Did anyone push to fork main during the merge? (More likely now that PR #60 demonstrated active development on the WS sync layer in the meantime.)

## Notes

### Out-of-band fork pushes
The 001909, 001980, 001996, 002029, and 002029-extension merges all picked up out-of-band fixes pushed to fork main while the merge branches were open. **PR #60 landing 2026-06-09 is the most recent example.** Treat as expected — re-merge `origin/main` into the feature branch before declaring done if needed.

### `stack` is the canonical builder
There is no local Rust toolchain in this environment — all builds go via `cd /home/retro/work/helix && ./stack build-zed dev` (Docker-based, persistent cache, ~46s–1m31s warm). `cargo check` / `cargo test` items in the validation list are **best-effort**; the E2E gate is the hard contractual requirement.

### E2E phase count is 17
Phases 1–14 from 001996; 15–17 added by PR #55/#56/#57. Phase 9 is the explicit regression gate for PR #60's `ede_diagnostic` retry loop; Phase 17 is the explicit gate for PR #56 Fix 1b draft suppression.

### Branch naming
Do not reuse `feature/002059-merge-latest-zed` — that task was planned but never executed. Use `feature/002077-merge-latest-zed`.

## Out of Scope

- Changes to Helix API, frontend, or other non-Zed components unless forced by a breaking upstream change.
- New feature development beyond what conflict resolution requires.
- Rewriting or refactoring Helix-specific crates.
- Wiring upstream's `agent_skills` / `skill_creator` / `/compact` / `auto_compact` into Helix workflows.
- Implementing typed-error semantics beyond what `215ca2fb0b` forces at Helix call sites.
