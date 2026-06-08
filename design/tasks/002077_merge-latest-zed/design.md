# Design: Merge Latest Zed Upstream (002077)

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` — fork remote `origin` (in-cluster gitea URL is `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`). The in-cluster URL **is** the fork; there is no separate origin/fork distinction.
- **Upstream**: `zed-industries/zed`. The `upstream` remote is **not configured** in fresh clones — add it: `git remote add upstream https://github.com/zed-industries/zed.git`. (Was added in the planning environment to compute divergence; implementation agent re-verifies in its own clone.)
- **Porting guide** (canonical, living document): `/home/retro/work/zed/portingguide.md` — exists, **892 lines** as of the start of this task with the latest entry `## Merge 002029-extension round 2 (2026-06-02)` at line 750. Do **not** create a separate file; update the in-repo one.
- **Helix platform repo**: `/home/retro/work/helix/` — `sandbox-versions.txt` carries `ZED_COMMIT=79b9bfb1d60cbef5b14ba7e3992ba5e8f6eb335c`. **Aligned with fork main** — the `ZED_COMMIT` bump just moves to the new 002077 merge HEAD.

## Current State (as of 2026-06-08)

| | Commit | Date |
|---|---|---|
| Fork HEAD | `79b9bfb1d6` ("Merge pull request #58 from helixml/feature/002029-merge-latest-zed") | 2026-06-02 |
| Last upstream merge fence | `9d50bab893` ("git_ui: Add total diff stats to git panel (#58018)") — 002029-extension round 2 | 2026-06-02 |
| Upstream HEAD | `3f5705b985` ("extension_ci: Bump extension CLI version to `9ee3c50` (#58785)") | 2026-06-08 |
| Helix-only commits since 002029 | **0** (clean baseline — first time since at least 001980) | 6 days |
| Upstream commits to merge | **139** | 6 days |

Recent merge precedent (size → conflict count):
- 002029: 261 commits, 10 days, **7** manual conflicts (incl. Fix 1b position-critical), 3 build-fix commits
- 002029-extension: 287 commits, 3 days, **0** manual conflicts (`ort` strategy auto-resolved), 1 signature-drift repair (`code_span_resolver`)
- 002029-extension round 2: 242 commits, 8 days, **4** manual conflicts (all "both sides added a field"), 3 signature-drift repairs

**002077 outlook**: 139 commits over 6 days, with the largest upstream churn in `agent_panel.rs` (+612), `agent.rs` (+257), and `acp_thread.rs` (+184). Expect **2–6 manual conflicts** and **2–4 signature-drift repairs** — closest in shape to 002029-extension round 2. The dominant variables are:

1. `215ca2fb0b` "Typed workspace errors" — compile-driven migration of every Helix `Workspace::show_error` call site.
2. `116e4bc184` "Inherit source agent without draft content" — Fix 1b position must be re-verified.
3. `27191913e9` "Cumulative token usage accumulation" — schema-drift check on WS sync payloads.
4. `56b71271c4` "Enable ACP session usage and deletion features" — feature-flag/trait-default check.

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
# Build → critical-fix grep → unit tests → E2E (Phase 17 as the Fix-1b gate).
git push -u origin feature/002077-merge-latest-zed
# Write pull_request_zed.md + pull_request_helix.md; bump ZED_COMMIT in helix.
```

### Conflict-resolution principles (carried from 001864 / 001909 / 001980 / 001996 / 002029)

1. **Prefer upstream ordering** for shared changes (model lists, feature lists, match arms) — keeps the diff small and minimises future conflict surface.
2. **Keep Helix-specific code behind `#[cfg(feature = "external_websocket_sync")]`** — never let it leak into unconditional code paths.
3. **`Cargo.lock`**: always accept upstream (`git checkout --theirs`); regenerated on next `cargo build --features external_websocket_sync`.
4. **CI / `.github/workflows/`**: always accept upstream — Helix doesn't use Zed's CI.
5. **Upstream renames**: after every merge — even when files auto-merge cleanly — grep for the old identifiers in cfg-gated regions to catch silent drift.
6. **Trait-signature changes** (this merge: `Workspace::show_error<E: WorkspaceError>`): walk all impls compile-driven; the post-merge build is the safety net.
7. **Document every non-trivial resolution in `portingguide.md` immediately**, not at the end.
8. **If a conflict is too risky to resolve**: stop, document explicitly in `portingguide.md`, and escalate to the user rather than guess.

## Likely Conflict Hot-spots

Upstream diff sizes against Helix-touched files (`9d50bab893..upstream/main`, measured 2026-06-08). Inspect each even if `git merge` reports "auto-merged":

| File | Commits | +/- lines | Risk | Helix concern |
|------|---------|-----------|------|---------------|
| `crates/agent_ui/src/agent_panel.rs` | 6 | +612 / -52 | **MEDIUM** | `116e4bc184` "Inherit source agent without draft content (#58636)" — touches the activate-draft/draft-inheritance path. Fix 1b's `#[cfg(feature = "external_websocket_sync")] { return; }` must remain the FIRST statement of `BaseView::Uninitialized`. Critical Fix #11 entity-identity guard (now `thread_id`-based after 002029) must survive. Callbacks, UI state query, `acp_history_store()`, onboarding bypass, ACP auto-approve all preserved. |
| `crates/agent/src/agent.rs` | 3 | +257 / -80 | MEDIUM | `27191913e9` "Cumulative token usage accumulation (#58378)" — revives `Thread::cumulative_token_usage` field and adds accumulation+persistence. Critical Fix #1 (entity-lifetime in `load_session`) and `wait_for_tools_ready` (must use `cx.background_executor().timer()`, not `smol::Timer`) preserved. |
| `crates/acp_thread/src/acp_thread.rs` | 4 | +184 / -14 | MEDIUM | Critical Fixes #6/#8/#9 (cancel/Stopped state machine) preserved; PR #55 streaming-reveal `EntryUpdated` emit preserved. `de744e744c` "Correctly handle file links in markdown and agent threads (#56024)" may touch the entry-update path — inspect for conflict against PR #55's emit site. |
| `crates/agent_ui/src/conversation_view.rs` | 4 | +80 / -3 | LOW-MED | `from_existing_thread()` signature-drift magnet (repaired in 002029, 002029-extension, and 002029-extension round 2 — expect a fourth round). Helix-only constructor must mirror upstream's `ConversationView::new()` field-by-field, including any new struct fields and any `ThreadView::new` arg additions. |
| `crates/workspace/src/workspace.rs` | 4 | small | **MEDIUM** | `215ca2fb0b` "Typed workspace errors (#57649)" migrates `Workspace::show_error` to `<E: WorkspaceError>` generic. Every Helix call site (likely in `external_websocket_sync/src/thread_service.rs` and possibly `agent_panel.rs`) breaks the build until migrated. `a32999e00b` "Update window title when switching active workspace (#58401)" adds a shared `Rc<Cell<EntityId>>` between member workspaces — verify the `CollaboratorId::Agent` follow-focus guard is unaffected. |
| `crates/agent_settings/src/agent_settings.rs` + `crates/settings_content/src/agent.rs` | 1 | small | LOW-MED | `89cac4944d` "Improve sandbox write-path handling (#58283)" extends the `sandbox_permissions` plumbing absorbed in 002029-round-2. Expected to be a "both sides added a field" trivial three-way coexistence with Helix's `show_onboarding` / `auto_open_panel`. |
| `crates/agent_servers/src/acp.rs` | 2 | +4 / -91 | LOW | Small upstream cleanup. PR #50 `session_creation_chain` + `_settings_subscription` should survive cleanly. `56b71271c4` "Enable ACP session usage and deletion features (#58680)" — confirm no feature-flag default the Helix `AcpConnection` impl now needs to override. |
| `crates/zed/src/main.rs` | 2 | small | LOW | `--allow-multiple-instances`, `--headless` flags + `build_application(headless: bool)` signature (002029-round-2). Re-verify both call sites still pass the right `headless` value. |
| `crates/title_bar/` | 4 | small | LOW | `external_websocket_sync = { workspace = true, optional = true }` dep + cfg-gated `render_restricted_mode` early-return preserved. |
| `crates/extensions_ui/src/extensions_ui.rs` | 1 | small | LOW | `215ca2fb0b` "Typed workspace errors" likely touches this file too. `// HELIX: External agent ...` bypass markers at ~221, ~243, ~1513 — re-grep. |
| `crates/feature_flags/src/flags.rs` | 0 | none | TRIVIAL | No upstream churn this window; `AcpBetaFeatureFlag::enabled_for_all() -> true` safe. |
| `crates/external_websocket_sync/` | (Helix-only) | n/a | LOW | Should not conflict. Verify PR #56 Fix 1a, PR #55, PR #57 plumbing intact. **Likely site for new `WorkspaceError` impl wrappers** if `215ca2fb0b` migration requires them. |
| `crates/workspace/src/workspace.rs` (focus guard) | 4 | small | LOW | `CollaboratorId::Agent` must not steal keyboard focus — preserved across `follow()` / `update_follower_items()`. |
| `Cargo.toml` (workspace root) | n/a | small | LOW | `rust-embed` `debug-embed` feature preserved. |
| `Cargo.lock` | always | always | TRIVIAL | `--theirs`. |

### Highest-risk single change: upstream `215ca2fb0b` "Typed workspace errors (#57649)"

Migrates `Workspace::show_error` to a generic `<E: WorkspaceError>` taking a trait-bound type instead of `&anyhow::Error`. The trait provides "methods to show a proper error message but most importantly means to help with providing actions given certain errors." Helix has call sites in (at minimum) `external_websocket_sync/src/thread_service.rs` and possibly `agent_panel.rs` that currently pass a string or `anyhow::Error`. **Each call site needs migration**:

- Option 1: implement `WorkspaceError` for a Helix-side error type (cleanest if there's a Helix error enum already).
- Option 2: wrap the string in a tiny ad-hoc `WorkspaceError` impl per call site (smallest diff).
- Option 3: use whatever convenience constructor upstream provides on `WorkspaceError` for ad-hoc strings (preferred if it exists — minimises Helix surface).

Document the chosen approach as a "Pre-existing Breakage Repaired" entry. Build-driven discovery: `./stack build-zed dev` will surface every site.

### Medium-risk single change: upstream `116e4bc184` "agent_ui: Inherit source agent without draft content (#58636)"

PR description: "when you had an empty draft and created a worktree we weren't respecting agent selection. It was because of an early ? return on an option." The fix removes/restructures that early return. Risk for Helix: this is in the same family of code paths as PR #56 Fix 1b — both manipulate the draft/activate-draft flow. **Re-read `ensure_thread_initialized`'s full body after the merge** and confirm:

1. The `#[cfg(feature = "external_websocket_sync")] { return; }` is still the FIRST statement of `BaseView::Uninitialized`.
2. No new code path bypasses Fix 1b to reach `connection.new_session()` or `activate_draft` under `external_websocket_sync`.
3. Phase 17 of the E2E passes for both `zed-agent` and `claude` — this is the runtime gate.

### Medium-risk single change: upstream `27191913e9` "agent: Cumulative token usage (#58378)"

Revives `Thread::cumulative_token_usage` (dead code since the agent2 rewrite) with accumulation logic on `request_completed`/`tool_finished` paths and persistence to `threads.db`. The risk is schema drift: if the Helix WS sync layer marshals `Thread` state into a payload that now contains `cumulative_token_usage`, downstream consumers may see a new key. Inspect:

```bash
grep -rn "cumulative_token_usage" crates/external_websocket_sync/
grep -rn "TokenUsage\|token_usage" crates/external_websocket_sync/
```

If the WS payload is touched, document the schema bump in `portingguide.md` and confirm the Helix API server tolerates the additional field.

### Medium-risk single change: upstream `56b71271c4` "acp: Enable ACP session usage and deletion features (#58680)"

Stabilizes context windows + persistent session/delete for ACP agents. Likely flips a feature-flag default or trait default. Walk:

```bash
grep -n "supports_delete\|supports_session_usage\|context_window" crates/agent_servers/src/acp.rs
```

Confirm Helix's `AcpConnection` impl either picks up the new defaults transparently or that any override Helix needs is in place. The 002029 trait migration to `supports_delete(&self, &App)` is presumed unchanged; verify.

### Low-risk single change: upstream `a32999e00b` "workspace: Update window title (#58401)"

Adds a shared `Rc<Cell<EntityId>>` between member workspaces in a multi-workspace window for tracking the active workspace. Helix's `CollaboratorId::Agent` follow-focus guard sits in the same file but in a different function. Re-grep after merge to confirm.

### Low-risk single change: upstream `89cac4944d` "Improve sandbox write-path handling (#58283)"

Extends the `sandbox_permissions` field plumbing absorbed in 002029-round-2 (where it coexisted with Helix's `show_onboarding` / `auto_open_panel`). Expected to be a trivial three-way "both sides added a field" merge. Inspect both `agent_settings/src/agent_settings.rs` and `settings_content/src/agent.rs`.

## Resolution Patterns Worth Re-stating

- **`HeadlessConnection` is dead code** (since 001909). Don't be misled by old porting-guide entries.
- **`ConnectedServerState` field set grew in 002029** (added `code_span_resolver`, `last_theme_id`, `draft_prompt_persist_task`, `available_skills`, dropped `prompt_store`). `from_existing_thread()` will silently break if upstream widens it further — every recent merge required a repair. Diff the upstream `ConversationView::new()` field-by-field against the post-merge `from_existing_thread()` body.
- **`AcpThreadEvent::Stopped(StopReason)`** is a tuple variant — pattern matches need `Stopped(_)`. Test code is not exempt (lesson from 001980).
- **`BaseView::*` arms** must remain exhaustive — if upstream added new variants this window, the Helix UI state query in `agent_panel.rs::new()` and `zed/src/main.rs` headless responder both need new arms. Build-driven discovery.
- **`ContextServerStatus::*` arms** likewise — `ClientSecretRequired { .. }` was added in 002029; check for further variants.
- **`title_bar`'s `external_websocket_sync` dep MUST be `optional = true`** with the matching `[features]` entry.
- **Auto-merged ≠ correct** — always grep for the renamed identifier set after the merge.
- **Build via `./stack build-zed dev`** in the Helix repo (compile check + fresh binary for E2E in one shot, ~46s–1m31s warm cache). There is no local Rust toolchain in this environment.
- **`go mod tidy` in `e2e-test/helix-ws-test-server/`** before E2E — the runner doesn't tidy itself.
- **GPUI events flush at end of entity update closure** (lesson from 001996 Phase 13 fix). When ordering WebSocket events that race with synchronous `cx.emit` calls, send the externally-visible event BEFORE invoking the entity-update that emits the synchronous event.
- **`build_application(headless: bool)` pattern** (from 002029-round-2) — if upstream refactors `main.rs` further, the `headless` parameter must continue to thread through.

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

# PR #50 (serialize ACP session creation) — must coexist with _settings_subscription (002029-round-2)
grep -n "session_creation_chain"     crates/agent_servers/src/acp.rs
grep -n "_settings_subscription"     crates/agent_servers/src/acp.rs
grep -n "test_concurrent_session_creation_is_serialized" crates/agent_servers/src/acp.rs

# PR #55 (streaming-reveal EntryUpdated emit)
grep -n "EntryUpdated"               crates/acp_thread/src/acp_thread.rs

# PR #56 (Fix 1a deferred UserCreatedThread + Fix 1b draft suppression)
grep -n "ensure_thread_initialized"  crates/agent_ui/src/agent_panel.rs
# Read the FULL function body. The Helix cfg-gated `return;` MUST be the FIRST statement
# of the BaseView::Uninitialized branch, before any source-agent-inheritance or
# terminal-spawn branches that 116e4bc184 or any other commit may have added.
grep -rn "defer.*UserCreatedThread\|first_user_message"  crates/external_websocket_sync/

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

# 215ca2fb0b typed workspace errors — every Helix show_error site must compile against the new signature
grep -rn "Workspace::show_error\|workspace.show_error\|\.show_error(" crates/external_websocket_sync/ crates/agent_ui/src/

# 27191913e9 cumulative token usage — schema drift check on WS payloads
grep -rn "cumulative_token_usage\|TokenUsage" crates/external_websocket_sync/

# 56b71271c4 acp session usage/deletion stabilisation — confirm Helix override (if any) is sound
grep -n "fn supports_delete\|fn supports_session_usage\|fn delete_session\|fn context_window" crates/agent_servers/src/acp.rs crates/agent/src/agent.rs

# Confirm AcpBetaFeatureFlag still in place
grep -n "AcpBetaFeatureFlag\|enabled_for_all"  crates/feature_flags/src/flags.rs

# Helix's HELIX: bypass markers in extensions_ui
grep -n "HELIX: External agent"      crates/extensions_ui/src/extensions_ui.rs

# Carry-over fixes
grep -n "allow_multiple_instances\|headless" crates/zed/src/main.rs
grep -n "build_application"          crates/zed/src/main.rs   # 002029-round-2 signature
grep -n "debug-embed"                Cargo.toml
grep -n "smol::Timer"                crates/agent/src/agent.rs   # should be 0

# Helix-specific surface
grep -n "CollaboratorId::Agent"      crates/workspace/src/workspace.rs
grep -n "render_restricted_mode"     crates/title_bar/src/title_bar.rs
```

### 3. Walk the rebase checklist
Step through every numbered item in `portingguide.md` §"Rebase Checklist" (currently 44+ items, including 002029's new entries on Fix 1b position and `supports_delete(&self, &App)` signature). Pay special attention to:
- Item 9 (`agent_panel.rs` cfg-gated blocks) — Fix 1b position regression risk from `116e4bc184`
- Item 11 (`ConnectedServerState` field set) — recheck struct; every recent merge needed a repair
- Items 31, 31a, 37 (`acp_thread.rs` cancel/Stopped territory)
- The 002029 additions: Fix 1b first-statement check; `supports_delete(&self, &App)` signature

### 4. Unit tests (if local Rust toolchain available)
```bash
cargo test -p external_websocket_sync          # full pass, ≤2 ignored
cargo test -p acp_thread test_second_send      # Stopped invariant (Critical Fix #6)
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

All 17 phases must pass for **both** `zed-agent` and `claude`. The contractual minimum:
- Phases 1–14 (as in 001996)
- **Phase 15** (streaming patches arrive incrementally — gates PR #55)
- **Phase 16** (zero spontaneous `UserCreatedThread` events — gates PR #56 Fix 1a + PR #57)
- **Phase 17** (live Claude process count == real thread count — **gates PR #56 Fix 1b draft suppression survived the merge**)

**Phase 17 is the explicit gate that the highest-risk Helix carry-over (PR #56's draft suppression) is intact.** If Phase 17 fails: stop, re-read `ensure_thread_initialized`, restore the cfg-gated early return as the FIRST statement of `BaseView::Uninitialized`.

If any phase fails: do **not** mark the task complete. Diagnose, fix, re-run.

One retry permitted for Claude Phase 1 npm-install bootstrap flake (lesson from 001996).

## Documentation Updates (incremental — not retrospective)

After **each** non-trivial conflict resolution, append to `/home/retro/work/zed/portingguide.md`:

1. **Conflict context**: file + region + which Helix concern it touches
2. **Upstream change**: the PR or commit and what it did
3. **Resolution**: what was kept from each side and why
4. **Risk**: any follow-up regression risk

Always extend the commit-history table at the bottom with this merge's commits and any follow-up fix commits. Add a new top-level section `## Merge 002077 (2026-06-08)` at the top of the merge-history list, mirroring the 002029-extension round 2 structure (Divergence at start / Manual conflicts / Pre-existing Breakage Repaired / Ancillary upstream notes / Validation).

**Mandatory subsections (regardless of how the merge auto-resolves)**:

1. **"`215ca2fb0b` Typed workspace errors — Helix `show_error` call-site migration"** — list each Helix call site, the chosen migration approach (impl `WorkspaceError` / ad-hoc wrap / use upstream convenience), and the rationale.
2. **"`116e4bc184` Inherit source agent without draft content vs Helix PR #56 Fix 1b"** — confirm Fix 1b's first-statement position survived; document any code-path change that required moving the guard.
3. **"`27191913e9` Cumulative token usage — WS sync payload schema check"** — record whether the WS payload was affected and, if so, what the schema bump is.
4. **"`56b71271c4` Stabilised ACP session usage/deletion — Helix `AcpConnection` impl review"** — record whether any Helix override needed updating.

**Likely new rebase-checklist additions** (only if confirmed in this merge):
- "All Helix `Workspace::show_error` call sites use the new `<E: WorkspaceError>` generic signature." (002077)
- "Verify PR #56 Fix 1b is the FIRST statement of `BaseView::Uninitialized` even after `116e4bc184` source-agent-inheritance refactor." (002077, additive to the 002029 entry)

## Helix Repo Bump

After the Zed branch is pushed:
1. In `/home/retro/work/helix/`, update `sandbox-versions.txt` `ZED_COMMIT=` from `79b9bfb1d60cbef5b14ba7e3992ba5e8f6eb335c` to the new merge HEAD.
2. Commit on a `feature/002077-merge-latest-zed` branch in helix.
3. Push the helix branch.
4. The Helix UI handles PR creation per task convention — do not open PRs from the agent.
5. After both merge, the build pipeline rebuilds the Zed binary + desktop image automatically.

## Open Questions (for the implementation agent to answer at runtime)

- How many Helix `Workspace::show_error` call sites exist, and what's the cleanest migration shape? (Implement `WorkspaceError` for a Helix error type, or ad-hoc wrap per call site?)
- Does `27191913e9`'s cumulative token usage accumulation push `cumulative_token_usage` into any payload the Helix WS sync layer marshals? If so, does the Helix API server tolerate the additional field?
- Does `116e4bc184` "Inherit source agent without draft content" rewrite the `ensure_thread_initialized` body or add a code path that bypasses Fix 1b? If so, where must the guard move to?
- Does `56b71271c4` flip any default that the Helix `AcpConnection` impl needs to override (`supports_session_usage`, `supports_delete`, etc.)?
- Are any silent-drift identifiers re-appearing in the +612-line `agent_panel.rs` diff? Re-grep after merge.
- Did upstream add new `BaseView` or `ContextServerStatus` variants beyond the current set? If yes, add arms to the Helix UI state queries (Pre-existing Breakage lesson from 001996 / 002029).
- Did upstream grow `ConversationView` past its current field set since 002029-round-2 (which added/dropped fields)? Walk `from_existing_thread()` against the live struct.
- Did the `agent-client-protocol` schema crate add new builder patterns or `#[non_exhaustive]` markers requiring migration?
- Did anyone push to fork main during the merge? If so, `git merge origin/main` into the feature branch and re-run E2E.

## Notes

### Out-of-band fork pushes
The 001909, 001980, 001996, 002029, and 002029-extension merges all picked up out-of-band fixes pushed to fork main while the merge branches were open. Treat this as expected — re-merge `origin/main` into the feature branch before declaring done if needed. **For 002077 the baseline is unusually clean (zero Helix-only commits since 002029)**, so out-of-band pushes are less likely but still possible.

### `stack` is the canonical builder
There is no local Rust toolchain in this environment — all builds go via `cd /home/retro/work/helix && ./stack build-zed dev` (Docker-based, persistent cache, ~46s–1m31s warm). `cargo check` / `cargo test` items in the validation list are **best-effort** for environments where Rust is installed; the E2E gate is the hard contractual requirement.

### E2E phase count is 17 (unchanged since 002029)
PR #55 + PR #56 added Phases 15–17; PR #57 fixed a Phase 16 false-positive. Phase 17 is the explicit regression gate for Fix 1b draft suppression. Do not skip Phase 17 — Phase 17 failing means the suppression has been lost.

### Branch naming
Do not reuse `feature/002059-merge-latest-zed` — that task was planned but never executed (no branch was ever pushed to origin); the helix-specs directory exists but the branch name slot is free. Nevertheless, **use `feature/002077-merge-latest-zed`** for this task to keep the task-id → branch-name mapping intact for the UI.

## Out of Scope

- Changes to Helix API, frontend, or other non-Zed components unless forced by a breaking upstream change.
- New feature development beyond what conflict resolution requires.
- Rewriting or refactoring Helix-specific crates.
- Wiring upstream's `agent_skills` / `skill_creator` into Helix workflows (out of scope — let it sit).
- Implementing typed-error semantics beyond what `215ca2fb0b` forces at Helix call sites (no proactive migration of Helix internal error types to `WorkspaceError`).
