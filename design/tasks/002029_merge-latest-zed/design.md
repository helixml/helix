# Design: Merge Latest Zed Upstream (002029)

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` — fork remote `origin`. There is no separate fork/origin distinction; the in-cluster gitea URL **is** the fork.
- **Upstream**: `zed-industries/zed`. The `upstream` remote is **not** configured by default in fresh clones — add it: `git remote add upstream https://github.com/zed-industries/zed.git`. (Already added in the planning environment via `git fetch upstream` — implementation agent should re-verify in its own clone.)
- **Porting guide** (canonical, living document): `/home/retro/work/zed/portingguide.md` — already exists, **762 lines** as of the start of this task. Do **not** create a separate file; update the in-repo one.
- **Helix platform repo**: `/home/retro/work/helix/` — contains `sandbox-versions.txt` with `ZED_COMMIT=b35224530f7c2ff5ead8b9cfcea23b050583d70d` (matches the `^2` of current fork HEAD `b2f2ebefb6`).

## Current State (as of 2026-05-18)

| | Commit | Date |
|---|---|---|
| Fork HEAD | `b2f2ebefb6` (PR #57 — Phase 16 counter fix) | 2026-05-14 |
| Last upstream merge | `8841edb2b1` (task 001996, integrated upstream `8bdd78e023`) | 2026-05-11 |
| Upstream HEAD | `f2df3f9e18` ("acp: Add logout support for ACP agents (#56959)") | 2026-05-17 |
| Helix-only commits since 001996 | **14** (PRs #50, #55, #56, #57) | 7 days |
| Upstream commits to merge | **158** | 7 days |

Recent merge precedent (size → conflict count):
- 001909: 86 commits, 3 days, **1** conflict, 3 carry-over fix commits
- 001980: 172 commits, 10 days, **4** conflicts, 2 follow-up fix commits
- 001996: 127 commits, 3 days, **1** conflict (acp_thread), **2** post-merge build-fix commits (BaseView::Terminal exhaustive match, Phase 13 race fix)

This merge is **larger by commit count and by per-file diff than 001996** despite a similar time window. Conflict count is hard to predict (could be 2–8), but the structural risk from `bbe23cc40b` "Bring back draft threads" against Helix PR #56's draft-suppression is the dominant variable.

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

For this merge, the upstream diff sizes against Helix-touched files are known up-front. Inspect each even if `git merge` reports "auto-merged":

| File | Upstream diff | Risk | Helix concern |
|------|---------------|------|---------------|
| `crates/agent_ui/src/agent_panel.rs` | **3156** | **VERY HIGH** | "Bring back draft threads" (`bbe23cc40b`, 1256 lines on its own) + 9 other upstream PRs. **Helix PR #56 Fix 1b's `#[cfg(feature = "external_websocket_sync")] { return; }` block inside `ensure_thread_initialized()` is the single most fragile carry-over.** Also: thread display callback, UI state query, onboarding bypass, `acp_history_store()`, ACP auto-approve, Critical Fix #11. The `BaseView::Terminal` arm added by 001996 may need siblings if upstream added more variants. |
| `crates/agent/src/agent.rs` | 1897 | **HIGH** | Critical Fix #1 (entity-lifetime in `load_session`), `wait_for_tools_ready` (must use `cx.background_executor().timer()`, not `smol::Timer`), `supports_delete` impl needs new `&App` parameter. |
| `crates/agent_ui/src/conversation_view.rs` | 720 | **HIGH** | `from_existing_thread()`, `THREAD_REGISTRY`, `is_resume`, history refresh, unregister-on-reset. Upstream `f2df3f9e18` (ACP logout) adds 90 lines here; `bbe23cc40b` adds 49; `bf423dfc45` (ACP rename), `2301e61d2a` (mermaid), `4074eabb3d` (images), `1c884d13d3` (terminal notifications) all touch it. |
| `crates/acp_thread/src/acp_thread.rs` | 313 | MEDIUM | Critical Fixes #6/#8/#9, `content_only()`. **PR #55 just added 12 lines for streaming-reveal drain** — overlaps `2301e61d2a` (mermaid) and `495f8ba717` (checkpoint emit on edit). |
| `crates/agent_servers/src/acp.rs` | (large) | **MEDIUM-HIGH** | **PR #50 added `session_creation_chain` field + chain logic on `AcpConnection`. Upstream `23231879cd` adds 161 lines of ACP session-deletion to the same file.** Likely real conflict. |
| `crates/acp_thread/src/connection.rs` | small | MEDIUM | **Trait signature change**: `AgentSessionList::supports_delete(&self)` → `supports_delete(&self, &App)`. Plus new `supports_logout`/`logout` default impls on `AgentConnection`. |
| `crates/agent_ui/src/acp/thread_history.rs` | TBD | MEDIUM | Multiple `supports_delete()` call sites; compile-driven fix expected. |
| `crates/external_websocket_sync/src/thread_service.rs` | (Helix-only) | LOW | Should not conflict; verify PR #56's Fix 1a `defer_user_created_thread_until_first_user_message` plumbing intact. |
| `crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go` | (Helix-only) | LOW | PR #57 Phase 16 counter fix already in fork; verify present. |
| `crates/agent_settings/src/agent_settings.rs` | 0 | LOW | No upstream change in this window; Helix fields safe. |
| `crates/workspace/src/workspace.rs` | 31 | LOW | Agent follow focus guard. |
| `crates/zed/src/main.rs` | 67 | LOW | `--allow-multiple-instances`, `--headless` flags. |
| `crates/title_bar/` | TBD | LOW | Helix connection status indicator + optional `external_websocket_sync` dep. |
| `Cargo.toml` (workspace root) | 101 | LOW | `rust-embed` `debug-embed` feature. |
| `Cargo.lock` | always | TRIVIAL | `--theirs`. |

### Highest-risk single change: upstream `bbe23cc40b` (#54292) "Bring back draft threads"

This is the dominant risk of this merge. Upstream is restructuring `agent_panel.rs` around a fundamentally different draft-thread lifecycle (`draft_prompt_store.rs`, retained drafts with parking, multi-draft sidebar). **Helix PR #56 just deliberately disabled the draft-spawn path under `external_websocket_sync` to fix a duplicate-Claude bug.** The patch is:

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

### Medium-risk single change: upstream `23231879cd` (#57004) "ACP session deletion"

Adds `connection.delete_session(...)` plumbing, including in `crates/agent_servers/src/acp.rs` where Helix PR #50 just added `session_creation_chain`. Resolution principle: fold both. Upstream's deletion plumbing is independent of the creation-chain field, so a clean side-by-side merge is expected, but the patch is large enough to warrant a careful three-way pick.

Also touches `crates/acp_thread/src/connection.rs` with the `supports_delete(&self, &App)` signature change. **Every existing Helix impl of `supports_delete` must be updated** (currently `crates/agent/src/agent.rs:1838` and one or two `acp/thread_history.rs` call sites). Compile-driven.

### Medium-risk single change: upstream `f2df3f9e18` (#56959) "ACP logout"

Last commit in the merge window (2026-05-17). Adds:
- 8 lines to `AgentConnection` trait (default `supports_logout` / `logout` impls) — no Helix override needed.
- 108 lines to `crates/agent_servers/src/acp.rs` — coexists with PR #50; another file to three-way pick.
- 19 lines to `crates/agent_ui/src/agent_panel.rs` and 90 lines to `crates/agent_ui/src/conversation_view.rs` — UI surface; mostly cfg-free, but the agent panel's added 19 lines may sit near Helix-modified regions.

### New upstream crates (no merge risk)

- `crates/agent_skills/` (1762 lines) — entirely new, no conflict. Helix doesn't use Skills today; leave the crate as-is. Out-of-scope to wire into Helix.

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

# PR #50 (serialize ACP session creation)
grep -n "session_creation_chain"     crates/agent_servers/src/acp.rs
grep -n "test_concurrent_session_creation_is_serialized" crates/agent_servers/src/acp.rs

# PR #55 (streaming-reveal EntryUpdated emit)
grep -n "EntryUpdated"               crates/acp_thread/src/acp_thread.rs   # the new emit site

# PR #56 (Fix 1a deferred UserCreatedThread + Fix 1b draft suppression)
grep -n "ensure_thread_initialized"  crates/agent_ui/src/agent_panel.rs
# Inside that fn there must be a cfg-gated early return; if upstream renamed the fn, find the new caller of activate_draft and re-apply.
grep -rn "defer.*UserCreatedThread\|first_user_message"  crates/external_websocket_sync/

# PR #57 (Go test-server Phase 16 counter fix)
grep -n "phase10\|Phase 10's own"    crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go

# Test-pattern drift (lesson from 001980 — checklist 41a)
grep -nE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/   # should be 0

# Upstream rename silent drift
grep -rn "ActiveView"                crates/agent_ui/src/   # should only be enum variants AgentPanelEvent::ActiveViewChanged/Focused
grep -rn "set_active_view"           crates/agent_ui/src/   # should be 0
grep -rn "draft_threads\|background_threads" crates/agent_ui/src/   # should be 0 (now retained_threads + new draft_prompt_store)
grep -rn "selected_agent_type"       crates/agent_ui/src/   # should be 0 (now selected_agent)

# Trait signature migration
grep -rn "fn supports_delete"        crates/   # every impl must take (&self, &App) after merge
grep -rn "\.supports_delete()"       crates/   # every call site must pass cx

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

Always extend the commit-history table at the bottom with this merge's commits and any follow-up fix commits. Add a new top-level section `## Merge 002029 (2026-05-18)` mirroring the structure of `## Merge 001996`.

**Mandatory new subsection (regardless of how the merge auto-resolves)**: a dedicated entry under `## Merge 002029` documenting how the Helix PR #56 Fix 1b draft suppression survived against upstream `bbe23cc40b`. This is the highest-impact carry-over decision and the most likely source of subtle regression in the next merge. Future merge engineers must be able to find this without re-deriving it.

**Likely new rebase-checklist additions**:
- New item: "Check `agent_panel.rs::ensure_thread_initialized` (or its post-upstream equivalent) for the `#[cfg(feature = "external_websocket_sync")] { return; }` early-return guard. Phase 17 of the E2E is the only end-to-end check that this is preserved."
- New item (if `supports_delete` signature change applies broadly): "All `AgentSessionList::supports_delete` impls take `(&self, &App)`; all call sites pass `cx`."

## Helix Repo Bump

After the Zed branch is pushed:
1. In `/home/retro/work/helix/`, update `sandbox-versions.txt` `ZED_COMMIT=` to the new merge HEAD.
2. Commit on a `feature/002029-merge-latest-zed` branch in helix.
3. Push the helix branch.
4. The Helix UI handles PR creation per task convention — do not open PRs from the agent.
5. After both merge, the build pipeline rebuilds the Zed binary + desktop image automatically.

## Open Questions (for the implementation agent to answer at runtime)

- Does upstream `bbe23cc40b` "Bring back draft threads" rename or restructure `ensure_thread_initialized` such that Helix's Fix 1b guard needs re-placement? **This is the single most likely source of regression in this merge.** If yes, document the new location in `portingguide.md`.
- Has upstream's `23231879cd` "ACP session deletion" introduced any code path that bypasses Helix PR #50's `session_creation_chain` (e.g. a new `delete_session` path that races with `new_session`)? Walk it.
- Does upstream `f2df3f9e18` "ACP logout" introduce any Helix-mode UI element (logout button) that would surprise a Helix user? Default impl returns "Logout is not supported" — confirm the UI hides the option when `supports_logout()` returns false.
- Are any silent-drift identifiers (`ActiveView`, `selected_agent_type`, `draft_threads`/`background_threads`, `Stopped` unit-pattern) re-appearing in the 3156-line `agent_panel.rs` diff? Re-grep after merge.
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
