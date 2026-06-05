# Design: Merge Latest Zed Upstream (002029)

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` — fork remote `origin`. There is no separate fork/origin distinction; the in-cluster gitea URL **is** the fork.
- **Upstream**: `zed-industries/zed`. The `upstream` remote is **not** configured by default in fresh clones — add it: `git remote add upstream https://github.com/zed-industries/zed.git`. (Already added in the planning environment via `git fetch upstream` — implementation agent should re-verify in its own clone.)
- **Porting guide** (canonical, living document): `/home/retro/work/zed/portingguide.md` — already exists, **762 lines** as of the start of this task. Do **not** create a separate file; update the in-repo one.
- **Helix platform repo**: `/home/retro/work/helix/` — contains `sandbox-versions.txt` with `ZED_COMMIT=b35224530f7c2ff5ead8b9cfcea23b050583d70d`. **Note**: this is 1 commit behind fork HEAD `fd26c1a113` (only the trivial Dockerfile.ci fix is missing); the `ZED_COMMIT` bump must move to the new 002029 merge HEAD.

## Current State (as of 2026-05-21)

| | Commit | Date |
|---|---|---|
| Fork HEAD | `fd26c1a113` ("fix: pull helix-org into ci container") | 2026-05-21 |
| Last upstream merge | `8841edb2b1` (task 001996, integrated upstream `8bdd78e023`) | 2026-05-11 |
| Upstream HEAD | `1399540715` ("settings_ui: Display scope in the breadcrumb (#57437)") | 2026-05-21 |
| Helix-only commits since 001996 | **15** (PRs #50, #55, #56, #57 + 1 direct CI fix `fd26c1a113`) | 10 days |
| Upstream commits to merge | **261** | 10 days |

Recent merge precedent (size → conflict count):
- 001909: 86 commits, 3 days, **1** conflict, 3 carry-over fix commits
- 001980: 172 commits, 10 days, **4** conflicts, 2 follow-up fix commits
- 001996: 127 commits, 3 days, **1** conflict (acp_thread), **2** post-merge build-fix commits (BaseView::Terminal exhaustive match, Phase 13 race fix)

This merge is **the largest by commit count of any recent merge (261 commits / 10 days, 12 501-line agent_panel.rs diff)**. Conflict count is hard to predict (likely 4–10), but the structural risk from `589dc95c87` "Restore last active agent panel entry" + `bbe23cc40b` "Bring back draft threads" against Helix PR #56's draft-suppression is the dominant variable. Both upstream PRs rewrite the exact `ensure_thread_initialized` function containing Helix Fix 1b.

## Merge Strategy

Use `git merge upstream/main` (consistent with every previous merge — preserves Helix commit history and makes conflict resolution traceable). Do **not** rebase.

```bash
cd /home/retro/work/zed
git remote add upstream https://github.com/zed-industries/zed.git   # only if missing
git fetch upstream
git checkout main && git pull origin main                            # in case main moved
git checkout -b feature/002029-merge-latest-zed
git merge upstream/main
# Resolve conflicts one at a time, updating portingguide.md as each is resolved
# Build → critical-fix grep → unit tests → E2E (with Phase 17 as the Fix-1b gate)
git push -u origin feature/002029-merge-latest-zed
# Write pull_request_zed.md + pull_request_helix.md; bump ZED_COMMIT in helix
```

### Conflict-resolution principles (carried from 001864 / 001909 / 001980 / 001996)

1. **Prefer upstream ordering** for shared changes (model lists, feature lists, match arms) — keeps the diff small and minimises future conflict surface.
2. **Keep Helix-specific code behind `#[cfg(feature = "external_websocket_sync")]`** — never let it leak into unconditional code paths.
3. **`Cargo.lock`**: always accept upstream (`git checkout --theirs`); regenerated on next `cargo build --features external_websocket_sync`.
4. **CI / `.github/workflows/`**: always accept upstream — Helix doesn't use Zed's CI.
5. **Upstream renames**: after every merge — even when files auto-merge cleanly — grep for the old identifiers in cfg-gated regions to catch silent drift.
6. **Trait-signature changes** (this merge: `supports_delete(&self)` → `supports_delete(&self, &App)`): walk all impls compile-driven; the post-merge build is the safety net.
7. **Document every non-trivial resolution in `portingguide.md` immediately**, not at the end.
8. **If a conflict is too risky to resolve**: stop, document explicitly in `portingguide.md`, and escalate to the user rather than guess.

## Likely Conflict Hot-spots

Upstream diff sizes against Helix-touched files (refreshed 2026-05-21 — `agent_panel.rs` has quadrupled since the 2026-05-18 snapshot; `agent_servers/src/acp.rs` has nearly tripled). Inspect each even if `git merge` reports "auto-merged":

| File | Upstream diff | Risk | Helix concern |
|------|---------------|------|---------------|
| `crates/agent_ui/src/agent_panel.rs` | **12 501** (was 3 156) | **VERY HIGH** | Four overlapping upstream PRs in the entry-point area: (1) `bbe23cc40b` "Bring back draft threads" (1 256 lines), (2) `589dc95c87` "Restore last active agent panel entry" (684 lines, **rewrites `ensure_thread_initialized` itself** — adds `pending_terminal_spawn` / `should_create_terminal_for_new_entry` / ACP-restoration branches before the `BaseView::Uninitialized` body), (3) `c84c22dab5` "Deprecate ACP extensions" (55 lines), (4) `e2c38b5358` "Replace Rules UI with Skills creation UI". **Helix PR #56 Fix 1b's `#[cfg(feature = "external_websocket_sync")] { return; }` guard must move to the very top of the new `ensure_thread_initialized` body**, before all the new branches. Also: thread display callback, UI state query, onboarding bypass, `acp_history_store()`, ACP auto-approve, Critical Fix #11. The `BaseView::Terminal` arm added by 001996 may need siblings if upstream added more variants. |
| `crates/agent_servers/src/acp.rs` | **1 086** (was ~400) | **HIGH** | Three concurrent upstream PRs plus Helix PR #50: (1) `23231879cd` "ACP session deletion" (+161 lines), (2) `f2df3f9e18` "ACP logout" (+108), (3) `c3951af24f` "Support additional session directories" (+489, **expanding `new_session`/`load_session`/`open_or_create_session` — exactly the methods PR #50's `session_creation_chain` wraps**). Three-way fold required. |
| `crates/agent/src/agent.rs` | **2 339** (was 1 897) | **HIGH** | Critical Fix #1 (entity-lifetime in `load_session`), `wait_for_tools_ready` (must use `cx.background_executor().timer()`, not `smol::Timer`), `supports_delete` impl on line 1838 needs new `&App` parameter. `2e70059cd9` Skills feature-flag removal drops 63 lines here. |
| `crates/agent_ui/src/conversation_view.rs` | **893** (was 720) | **HIGH** | `from_existing_thread()`, `THREAD_REGISTRY`, `is_resume`, history refresh, unregister-on-reset. Upstream `f2df3f9e18` (ACP logout) adds 90 lines here; `bbe23cc40b` adds 49; `bf423dfc45` (ACP rename), `2301e61d2a` (mermaid), `4074eabb3d` (images), `1c884d13d3` (terminal notifications) all touch it. |
| `crates/acp_thread/src/acp_thread.rs` | **404** (was 313) | MEDIUM | Critical Fixes #6/#8/#9, `content_only()`. **PR #55's 12-line streaming-reveal `EntryUpdated` emit** overlaps `2301e61d2a` (mermaid) and `495f8ba717` (checkpoint emit on edit). |
| `crates/acp_thread/src/connection.rs` | **82** | MEDIUM | **Confirmed at runtime**: upstream HEAD has `supports_delete(&self, &App)` plus new `supports_logout(&self, &App) -> bool` and `logout(&self, &mut App) -> Task<Result<()>>` default impls on `AgentConnection`. Compile-driven migration. |
| `crates/agent_ui/src/acp/thread_history.rs` | TBD | MEDIUM | 10 `supports_delete` references at runtime (impl line 362; call sites 365, 563, 574, 700, 797; builder/field 868, 879, 884–885). Each call site needs `cx` threaded after upstream's signature change. |
| `crates/extensions_ui/src/extensions_ui.rs` | **82** (almost all DELETIONS) | MEDIUM | `c84c22dab5` deletes 80 lines around the Helix `// HELIX: External agent keywords removed` (line 245) and `// HELIX: External agent upsells removed` (line 1586) bypass comments. The Helix patch may now be redundant. |
| `crates/agent_servers/src/custom.rs` | heavy cull | LOW-MED | `c84c22dab5` guts the file (116 → ~40 lines). Helix `ExternalAgent::server()` uses `CustomAgentServer::new(AgentId(...))` — confirm still compiles. |
| `crates/feature_flags/src/flags.rs` | small | LOW | `AcpBetaFeatureFlag` confirmed still present upstream (line 22). `2e70059cd9` removes `SkillsFeatureFlag`; `a7f037d94b` removes `AgentPanelTerminalFeatureFlag` + `ExperimentalSystemPromptFeatureFlag`; `800706d7a8` adds `UpdateTitleToolFeatureFlag`. Helix's `enabled_for_all()` override on `AcpBetaFeatureFlag` is unaffected. |
| `crates/external_websocket_sync/src/thread_service.rs` | (Helix-only) | LOW | Should not conflict; verify PR #56's Fix 1a `defer_user_created_thread_until_first_user_message` plumbing intact. |
| `crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go` | (Helix-only) | LOW | PR #57 Phase 16 counter fix already in fork; verify present. |
| `crates/external_websocket_sync/e2e-test/Dockerfile.ci` | (Helix-only) | LOW | `fd26c1a113` `helix-org` pull already in fork; verify present. |
| `crates/agent_settings/src/agent_settings.rs` | 0 | LOW | No upstream change in this window; Helix fields safe. |
| `crates/workspace/src/workspace.rs` | **128** (was 31) | LOW | Agent follow focus guard. |
| `crates/zed/src/main.rs` | **196** (was 67) | LOW | `--allow-multiple-instances`, `--headless` flags (all 3 `--headless` call sites). |
| `crates/title_bar/` | small | LOW | Helix connection status indicator + `optional = true` `external_websocket_sync` dep must survive (6 upstream PRs touch this crate). |
| `Cargo.toml` (workspace root) | **135** (was 101) | LOW | `rust-embed` `debug-embed` feature. |
| `Cargo.lock` | always | TRIVIAL | `--theirs`. |

### Highest-risk single change: upstream `589dc95c87` (#57150) "Restore last active agent panel entry" — NEW since 2026-05-18

Confirmed at runtime: this PR adds 684 lines to `agent_panel.rs` and **rewrites `ensure_thread_initialized` itself**. The pre-PR upstream body was a single-line dispatch to `activate_draft`; post-PR the body grows several branches (`pending_terminal_spawn`, `should_create_terminal_for_new_entry`, ACP-thread restoration) any of which can spawn agent processes before falling through to `activate_draft`.

**The Helix Fix 1b `#[cfg(feature = "external_websocket_sync")] { return; }` guard must land at the very TOP of the new function body — before all the new branches — not inside the old `if matches!(BaseView::Uninitialized)` block.** Otherwise the new terminal-spawn and ACP-restoration paths will fire in Helix mode and Phase 17 will fail.

Validation after merge: read the full `ensure_thread_initialized` body and confirm the cfg-gated `return;` is the first statement. Then run E2E and confirm Phase 17 passes for both `zed-agent` and `claude`.

### Second-highest-risk single change: upstream `bbe23cc40b` (#54292) "Bring back draft threads"

Restructures `agent_panel.rs` around a fundamentally different draft-thread lifecycle (`draft_prompt_store.rs`, retained drafts with parking, multi-draft sidebar). **Helix PR #56 just deliberately disabled the draft-spawn path under `external_websocket_sync` to fix a duplicate-Claude bug.** The patch is:

```rust
// in fn ensure_thread_initialized(...)
if matches!(self.base_view, BaseView::Uninitialized) {
    #[cfg(feature = "external_websocket_sync")]
    {
        let _ = window;
        let _ = cx;
        return;
    }
    #[cfg(not(feature = "external_websocket_sync"))]
    self.activate_draft(false, "agent_panel", window, cx);
}
```

After the merge:

1. **Grep**: `grep -n "ensure_thread_initialized\|activate_draft" crates/agent_ui/src/agent_panel.rs` — if the function or its caller has been renamed or inlined upstream, the guard needs to be re-applied at the new entry point.
2. **Functional check**: open the Helix-mode agent panel via the E2E flow and confirm no `claude` child is spawned **until** the user (or `chat_message` WS event) actually creates a thread. **Phase 17 of the E2E is the explicit hard gate** for this; if Phase 17 fails, draft suppression has been lost.
3. **Document**: write a dedicated subsection in `portingguide.md`'s new `## Merge 002029` heading explaining the upstream/Helix interaction and the exact resolution.

### High-risk single change: upstream `c3951af24f` (#57051) "Support additional session directories" — NEW since 2026-05-18

Adds **489 lines** to `crates/agent_servers/src/acp.rs`, expanding `new_session`, `load_session`, and `open_or_create_session` — the exact methods PR #50's `session_creation_chain` wraps. Three-way fold required: keep PR #50's chain field + drop-guard, but pass through any new arguments (e.g. additional path lists) upstream now expects. After resolution, `test_concurrent_session_creation_is_serialized` should remain a hard local check.

### Medium-risk single change: upstream `23231879cd` (#57004) "ACP session deletion"

Adds `connection.delete_session(...)` plumbing, in `crates/agent_servers/src/acp.rs` where Helix PR #50 just added `session_creation_chain` AND where `c3951af24f` above also expands. A three-way fold across the file is needed. Cascades the `supports_delete(&self)` → `supports_delete(&self, &App)` signature change through `crates/agent_ui/src/acp/thread_history.rs` (10 references) and `crates/agent/src/agent.rs:1838`.

### Medium-risk single change: upstream `f2df3f9e18` (#56959) "ACP logout"

Adds:
- 8 lines to `AgentConnection` trait (default `supports_logout` / `logout` impls) — no Helix override needed; defaults return `false` / `Err("Logout is not supported")`.
- 108 lines to `crates/agent_servers/src/acp.rs` — coexists with PR #50 and `c3951af24f`.
- 19 lines to `crates/agent_ui/src/agent_panel.rs` and 90 lines to `crates/agent_ui/src/conversation_view.rs` — UI surface; visually verify no logout button surfaces in Helix-mode builds.

### Medium-risk single change: upstream `c84c22dab5` (#57133) "Deprecate ACP extensions" — NEW since 2026-05-18

Removes **1 459 lines** across the agent-server extension surface (`crates/agent_servers/src/custom.rs`, `crates/project/src/agent_server_store.rs`, `crates/extensions_ui/src/extensions_ui.rs`, `crates/extension_host/`). Two Helix-relevant impacts:
1. `extensions_ui.rs` loses 80 lines — exactly the region around Helix's `// HELIX: External agent ...` bypass markers (lines 245, 1586). If upstream's deletion already removes the extension surface those markers were bypassing, the Helix patch becomes redundant. Document and drop rather than re-apply.
2. `crates/agent_servers/src/custom.rs` is gutted (116 → ~40 lines). Helix `ExternalAgent::server()` uses `CustomAgentServer::new(AgentId(...))` (porting-guide checklist 14) — confirm it still compiles.

### New upstream crates (no merge risk)

- `crates/agent_skills/` (~2 000 lines) and `crates/skill_creator/` (1 073 lines) — new upstream crates. Out-of-scope to wire into Helix; leave as-is.

## Resolution Patterns Worth Re-stating

- **`HeadlessConnection` is dead code** (since 001909). Don't be misled by old porting-guide entries.
- **`ConnectedServerState`** as of 001996 has 6 fields. Upstream may have grown new ones in this window — re-grep after merge; `from_existing_thread()` will break silently if the struct widens.
- **`AcpThreadEvent::Stopped(StopReason)`** is a tuple variant — pattern matches need `Stopped(_)`. Test code is not exempt (lesson from 001980).
- **`BaseView::Terminal { .. }` arm** was added by 001996. Upstream may have added more `BaseView` variants — the Helix UI state query in `agent_panel.rs::new()` must remain exhaustive (lesson from 001996).
- **`title_bar`'s `external_websocket_sync` dep MUST be `optional = true`** with the matching `[features]` entry.
- **Auto-merged ≠ correct** — always grep for the renamed identifier set after the merge.
- **Build via `./stack build-zed dev`** in the Helix repo (compile check + fresh binary for E2E in one shot, ~2 min warm cache). There is no local Rust toolchain in this environment.
- **`go mod tidy` in `e2e-test/helix-ws-test-server/`** before E2E — the runner doesn't tidy itself.
- **GPUI events flush at end of entity update closure** (lesson from 001996 Phase 13 fix). When ordering WebSocket events that race with synchronous `cx.emit` calls, send the externally-visible event BEFORE invoking the entity-update that emits the synchronous event.

## Post-Merge Validation

### 1. Compile check
```bash
cd /home/retro/work/zed
cargo check -p zed                                          # no features (if local rust)
cargo check -p zed --features external_websocket_sync       # with Helix gate (if local rust)
cd /home/retro/work/helix
./stack build-zed dev                                       # ~2 min warm cache, produces ./zed-build/zed
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

# PR #50 (serialize ACP session creation) — must coexist with upstream c3951af24f and 23231879cd
grep -n "session_creation_chain"     crates/agent_servers/src/acp.rs
grep -n "test_concurrent_session_creation_is_serialized" crates/agent_servers/src/acp.rs
grep -n "fn delete_session\|fn logout"   crates/agent_servers/src/acp.rs   # confirm upstream additions present too

# PR #55 (streaming-reveal EntryUpdated emit)
grep -n "EntryUpdated"               crates/acp_thread/src/acp_thread.rs   # the new emit site

# PR #56 (Fix 1a deferred UserCreatedThread + Fix 1b draft suppression)
grep -n "ensure_thread_initialized"  crates/agent_ui/src/agent_panel.rs
# Read the FULL function body — upstream 589dc95c87 adds multiple branches (pending_terminal_spawn,
# should_create_terminal_for_new_entry, ACP restoration). The Helix cfg-gated `return;` must be
# the VERY FIRST statement of the function body, before any of those branches.
grep -rn "defer.*UserCreatedThread\|first_user_message"  crates/external_websocket_sync/

# fd26c1a113 (direct Dockerfile.ci fix)
grep -n "helix-org"                   crates/external_websocket_sync/e2e-test/Dockerfile.ci

# PR #57 (Go test-server Phase 16 counter fix)
grep -n "phase10\|Phase 10's own"    crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go

# Test-pattern drift (lesson from 001980 — checklist 41a)
grep -nE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/   # should be 0

# Upstream rename silent drift
grep -rn "ActiveView"                crates/agent_ui/src/   # should only be enum variants AgentPanelEvent::ActiveViewChanged/Focused
grep -rn "set_active_view"           crates/agent_ui/src/   # should be 0
grep -rn "draft_threads\|background_threads" crates/agent_ui/src/   # should be 0 (now retained_threads + new draft_prompt_store)
grep -rn "selected_agent_type"       crates/agent_ui/src/   # should be 0 (now selected_agent)

# Trait signature migration (forced by upstream 23231879cd)
grep -rn "fn supports_delete"        crates/   # every impl must take (&self, &App) after merge
grep -rn "\.supports_delete()"       crates/   # every call site must pass cx
# Helix-specific sites to update: crates/agent_ui/src/acp/thread_history.rs (10 references),
# crates/agent/src/agent.rs:1838

# Confirm AcpBetaFeatureFlag still in place; upstream 2e70059cd9 removed SkillsFeatureFlag,
# a7f037d94b removed AgentPanelTerminalFeatureFlag + ExperimentalSystemPromptFeatureFlag
grep -n "AcpBetaFeatureFlag\|enabled_for_all"  crates/feature_flags/src/flags.rs

# Helix's HELIX: bypass markers in extensions_ui — upstream c84c22dab5 may have made them redundant
grep -n "HELIX: External agent"      crates/extensions_ui/src/extensions_ui.rs   # if gone, document the deliberate drop

# Carry-over fixes
grep -n "allow_multiple_instances"   crates/zed/src/main.rs
grep -n "headless"                   crates/zed/src/main.rs   # PR #51, all 3 call sites
grep -n "debug-embed"                Cargo.toml
grep -n "smol::Timer"                crates/agent/src/agent.rs   # should be 0

# Helix-specific surface
grep -n "CollaboratorId::Agent"      crates/workspace/src/workspace.rs
grep -n "AcpBetaFeatureFlag"         crates/feature_flags/src/flags.rs
```

### 3. Walk the rebase checklist
Step through every numbered item in `portingguide.md` §"Rebase Checklist" (currently 44 items including 41a from 001980 and the new #11). Pay special attention to:
- Item 9 (`agent_panel.rs` cfg-gated blocks) — must now also cover Fix 1b draft suppression.
- Item 11 (`ConnectedServerState` field set) — re-check struct.
- Items 31, 31a, 37 (`acp_thread.rs` cancel/Stopped territory).

### 4. Unit tests (if local Rust toolchain available)
```bash
cargo test -p external_websocket_sync          # full pass, ≤2 ignored
cargo test -p acp_thread test_second_send      # Stopped invariant (Critical Fix #6)
cargo test -p agent_servers test_concurrent_session_creation_is_serialized   # PR #50 + survival vs c3951af24f
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

All in-tree phases (1–17) must pass for **both** `zed-agent` and `claude`. The contractual minimum named in `requirements.md` includes:
- Phases 1–14 (as in 001996)
- **Phase 15** (streaming patches arrive incrementally — gates PR #55 + PR #56's streaming-cadence assertion)
- **Phase 16** (zero spontaneous `UserCreatedThread` events — gates PR #56 Fix 1a)
- **Phase 17** (live Claude process count == real thread count — **gates PR #56 Fix 1b draft suppression survived the merge**)

**Phase 17 is the explicit gate that the highest-risk Helix carry-over (PR #56's draft suppression) is intact.** If Phase 17 fails: stop, re-read `ensure_thread_initialized` / nearest equivalent, restore the cfg-gated early return.

If any phase fails: do **not** mark the task complete. Diagnose, fix, re-run.

## Documentation Updates (incremental — not retrospective)

After **each** non-trivial conflict resolution, append to `/home/retro/work/zed/portingguide.md`:

1. **Conflict context**: file + region + which Helix concern it touches
2. **Upstream change**: the PR or commit and what it did
3. **Resolution**: what was kept from each side and why
4. **Risk**: any follow-up regression risk

Always extend the commit-history table at the bottom with this merge's commits and any follow-up fix commits. Add a new top-level section `## Merge 002029 (2026-05-21)` mirroring the structure of `## Merge 001996`.

**Mandatory new subsection (regardless of how the merge auto-resolves)**: a dedicated entry under `## Merge 002029` documenting how the Helix PR #56 Fix 1b draft suppression survived against upstream `bbe23cc40b`. This is the highest-impact carry-over decision and the most likely source of subtle regression in the next merge. Future merge engineers must be able to find this without re-deriving it.

**Likely new rebase-checklist additions**:
- New item: "Check `agent_panel.rs::ensure_thread_initialized` for the `#[cfg(feature = "external_websocket_sync")] { return; }` early-return guard. After upstream `589dc95c87` 'Restore last active agent panel entry', the function body has multiple branches before `activate_draft` — the Helix guard must be the FIRST statement of the body. Phase 17 of the E2E is the only end-to-end check that this is preserved."
- New item: "All `AgentSessionList::supports_delete` impls take `(&self, &App)`; all call sites pass `cx`."
- New item (if confirmed): "Upstream `c84c22dab5` has absorbed the surface Helix `// HELIX: External agent ...` bypass markers in `extensions_ui.rs` targeted; Helix patch is now obsolete and was deliberately dropped in 002029."

## Helix Repo Bump

After the Zed branch is pushed:
1. In `/home/retro/work/helix/`, update `sandbox-versions.txt` `ZED_COMMIT=` to the new merge HEAD.
2. Commit on a `feature/002029-merge-latest-zed` branch in helix.
3. Push the helix branch.
4. The Helix UI handles PR creation per task convention — do not open PRs from the agent.
5. After both merge, the build pipeline rebuilds the Zed binary + desktop image automatically.

## Open Questions (for the implementation agent to answer at runtime)

- **Confirmed at planning time** that upstream `589dc95c87` rewrites `ensure_thread_initialized` with new branches; the Helix Fix 1b early-return must be the FIRST statement of the post-merge body. Re-confirm at merge time and document the exact post-merge location in `portingguide.md`.
- Has upstream `c3951af24f` "Support additional session directories" changed the signatures of `new_session`/`load_session`/`open_or_create_session` such that PR #50's `session_creation_chain` wrapper needs new parameters? Walk and document.
- Has upstream `23231879cd` "ACP session deletion" introduced any code path that bypasses Helix PR #50's `session_creation_chain` (e.g. a new `delete_session` path that races with `new_session`)? Walk it.
- Does upstream `f2df3f9e18` "ACP logout" introduce any Helix-mode UI element (logout button) that would surprise a Helix user? Default impl returns "Logout is not supported" — confirm the UI hides the option.
- Does upstream `c84c22dab5` "Deprecate ACP extensions" make the Helix `// HELIX: External agent ...` bypass markers in `extensions_ui.rs` redundant? If so, drop them deliberately and document.
- Are any silent-drift identifiers (`ActiveView`, `selected_agent_type`, `draft_threads`/`background_threads`, `Stopped` unit-pattern) re-appearing in the 12 501-line `agent_panel.rs` diff? Re-grep after merge.
- Did upstream grow `ConnectedServerState` past its 6 fields since 001996? Walk `from_existing_thread()` against the live struct.
- Did upstream add new `BaseView` variants beyond `AgentThread`, `Uninitialized`, `Terminal`? If yes, add arms to the Helix UI state query in `agent_panel.rs::new()` (Pre-existing-Breakage lesson from 001996).
- Has the `agent-client-protocol` schema crate added new builder patterns or `#[non_exhaustive]` markers requiring migration?
- Did anyone push to fork main during the merge? If so, `git merge origin/main` into the feature branch and re-run E2E.

## Notes

### Out-of-band fork pushes
The 001909, 001980, and 001996 merges all picked up out-of-band fixes pushed to fork main while the merge branches were open. Treat this as expected — re-merge `origin/main` into the feature branch before declaring done if needed.

### `stack` is the canonical builder
There is no local Rust toolchain in this environment — all builds go via `cd /home/retro/work/helix && ./stack build-zed dev` (Docker-based, persistent cache, ~2 min warm). `cargo check` / `cargo test` items in the validation list are **best-effort** for environments where Rust is installed; the E2E gate is the hard contractual requirement.

### E2E phase count grew from 14 to 17 since 001996
PR #55 and PR #56 added Phase 15 (streaming cadence), Phase 16 (deferred-emit regression), and Phase 17 (live Claude process count). Phase 17 in particular is the regression gate for Helix's most fragile carry-over in this merge. PR #57 fixed a Phase 16 false-positive. Do not skip Phase 17 — it's the explicit signal that PR #56 Fix 1b survived the merge.

## Out of Scope

- Changes to Helix API, frontend, or other non-Zed components unless forced by a breaking upstream change.
- New feature development beyond what conflict resolution requires.
- Rewriting or refactoring Helix-specific crates.
- Wiring upstream's new `agent_skills` crate into Helix (out of scope — let it sit).
