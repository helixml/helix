# Design: Merge Latest Zed Upstream (001980)

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` — fork remote `origin`, default branch `main`
- **Upstream**: `zed-industries/zed` on GitHub. The `upstream` remote is **not** configured today (`git remote -v` shows only `origin`). The implementation agent must add it: `git remote add upstream https://github.com/zed-industries/zed.git`
- **Porting guide** (canonical, living document): `/home/retro/work/zed/portingguide.md` — already exists. Do **not** create `design/zed-porting-guide.md`; update the in-repo file
- **Helix platform repo**: `/home/retro/work/helix/` — contains `sandbox-versions.txt` with `ZED_COMMIT=`

## Current State (as of 2026-05-05)

| | Commit | Date |
|---|---|---|
| Fork HEAD | `f5fab97857` ("bump context-server timeout 60s → 180s") | 2026-04-26/27 |
| Last upstream merge | `8428a4399d` (PR #43, task 001909) — integrated upstream `e3d1876c06` | 2026-04-25 |
| Upstream HEAD | TBD — fetch first | — |
| Helix-only commits since last merge | **4** (PRs #44, #45, #46, #47) | 8 days |
| Upstream commits to merge | TBD (~10 days of activity) | — |
| Skipped intermediate plans | 001946, 001947 (planned 2026-04-27, never executed) | — |

Recent merge precedent (size → conflict count):
- 001909: 86 commits, 3 days, **1** conflict, 3 carry-over fix commits
- 001864: 920 commits, 30 days, **35** conflicts, 5 follow-up fix commits
- 001723: 506 commits, 25 conflicts, 3 follow-up commits

This merge is expected to fall **between 001909 and 001864** in size. Roughly 250–500 commits is a plausible upper bound; the implementation agent should confirm by counting.

## Merge Strategy

Use `git merge upstream/main` (consistent with every previous merge — preserves Helix commit history and makes conflict resolution traceable). Do **not** rebase.

```bash
cd /home/retro/work/zed
git remote add upstream https://github.com/zed-industries/zed.git   # only if missing
git fetch upstream
git checkout -b feature/001980-merge-latest-zed   # from fork main (f5fab97857 or newer)
git merge upstream/main
# Resolve conflicts one at a time, updating portingguide.md as each is resolved
# Build → critical-fix grep → unit tests → E2E
git push -u origin feature/001980-merge-latest-zed
# Open Helix PR (sandbox-versions.txt bump), then Zed PR
```

### Conflict-resolution principles (carried from 001864 / 001909 / 001947)

1. **Prefer upstream ordering** for shared changes (model lists, feature lists, match arms) — keeps the diff small and minimises future conflict surface.
2. **Keep Helix-specific code behind `#[cfg(feature = "external_websocket_sync")]`** — never let it leak into unconditional code paths.
3. **`Cargo.lock`**: always accept upstream (`git checkout --theirs`); regenerated on next `cargo build --features external_websocket_sync`.
4. **CI / `.github/workflows/`**: always accept upstream — Helix doesn't use Zed's CI.
5. **Upstream renames**: after every merge — even when files auto-merge cleanly — grep for the old identifiers in cfg-gated regions to catch silent drift.
6. **Document every non-trivial resolution in `portingguide.md` immediately**, not at the end.
7. **If a conflict is too risky to resolve**: stop, document explicitly in `portingguide.md`, and escalate to the user rather than guess.

## Likely Conflict Hot-spots

Even on a small merge, inspect each of these even if `git merge` reports "auto-merged":

| File | Risk | Helix changes at risk |
|------|------|-----------------------|
| `crates/agent_ui/src/agent_panel.rs` | **HIGH** | Thread display callback, UI state query, onboarding bypass, split-brain detection, auto-follow, ACP auto-approve, `acp_history_store()` accessor, agent_type serialisation |
| `crates/agent_ui/src/conversation_view.rs` | **HIGH** | `from_existing_thread()`, `THREAD_REGISTRY`, `is_resume`, history refresh, unregister-on-reset, `UserCreatedThread` HashMap insertion |
| `crates/acp_thread/src/acp_thread.rs` | **MEDIUM** | `content_only()`, `cancel()` drop fix, `stopped_emitted_for_task` guard, `Stopped(StopReason)` tuple variant, trailing-edge flush timer (PR #44) |
| `crates/acp_thread/src/connection.rs` | **MEDIUM** | `wait_for_tools_ready()` trait method |
| `crates/agent/src/agent.rs` | **MEDIUM** | Critical Fix #1 (entity lifetime), `wait_for_tools_ready` (uses `cx.background_executor().timer()` since 001909) |
| `crates/external_websocket_sync/src/thread_service.rs` | **LOW–MEDIUM** | Critical Fixes #4, #5; PR #45 turn_request_id refresh; PR #46 `AgentConnectionCache` wiring — touched recently, raises chance of conflict |
| `crates/workspace/src/workspace.rs` | **LOW** | Agent follow focus guard `!matches!(leader_id, CollaboratorId::Agent)` |
| `crates/title_bar/` | **LOW** | Helix connection status indicator + optional `external_websocket_sync` dep |
| `crates/zed/Cargo.toml`, `agent_ui/Cargo.toml`, `title_bar/Cargo.toml` | **LOW** | Feature propagation chain |
| `crates/zed/src/main.rs` | **LOW** | `--allow-multiple-instances` CLI flag (silently lost in 001864 — re-grep) |
| `Cargo.toml` (workspace root) | **LOW** | `rust-embed` `debug-embed` feature (also silently lost previously) |
| `Cargo.lock` | **TRIVIAL** | Always `--theirs` |

## Resolution Patterns Worth Re-stating

- **`HeadlessConnection` is dead code** (since 001909). Current `from_existing_thread()` reuses `thread.read(cx).connection().clone()`. Don't be misled by old porting-guide entries.
- **`ConnectedServerState`** had 6 fields as of 001909: `connection`, `auth_state`, `active_id`, `threads`, `conversation`, `_connection_entry_subscription`. No `history` field. `from_existing_thread()` must list every field — upstream additions break it silently.
- **`AcpThreadEvent::Stopped(StopReason)`** is a tuple variant — pattern matches need `Stopped(_)`.
- **`title_bar`'s `external_websocket_sync` dep MUST be `optional = true`** with the matching `[features]` entry — otherwise the cfg-gated icon silently never renders.
- **Auto-merged ≠ correct** — always grep for the renamed identifier set after the merge.
- **Build via `./stack build-zed dev`** in the Helix repo (compile check + fresh binary for E2E in one shot, ~2 min).

## Post-Merge Validation

### 1. Compile check
```bash
cd /home/retro/work/zed
cargo check -p zed                                          # no features
cargo check -p zed --features external_websocket_sync       # with Helix gate
cd /home/retro/work/helix
./stack build-zed dev                                       # ~2 min, produces ./zed-build/zed
```

### 2. Grep verification of critical fixes / silent drift

```bash
cd /home/retro/work/zed

# Critical fixes
grep -n "load_session"               crates/agent/src/agent.rs                              | head
grep -n "content_only"               crates/acp_thread/src/acp_thread.rs
grep -n "drop(turn.send_task)"       crates/acp_thread/src/acp_thread.rs
grep -n "stopped_emitted_for_task"   crates/acp_thread/src/acp_thread.rs
grep -rn "unregister_thread"         crates/agent_ui/src/conversation_view.rs

# Renamed-identifier silent drift (lessons from 001864/001909)
grep -rn "ActiveView"                crates/agent_ui/src/   # should be 0 (renamed BaseView)
grep -rn "set_active_view"           crates/agent_ui/src/   # should be 0
grep -rn "draft_threads\|background_threads" crates/agent_ui/src/   # should be 0 (now retained_threads)
grep -rn "selected_agent_type"       crates/agent_ui/src/   # should be 0 (now selected_agent)

# Carry-over fixes from 001909 (silently lost previously)
grep -n "allow_multiple_instances"   crates/zed/src/main.rs
grep -n "debug-embed"                Cargo.toml
grep -n "smol::Timer"                crates/agent/src/agent.rs   # should be 0

# Helix-specific surface
grep -n "CollaboratorId::Agent"      crates/workspace/src/workspace.rs
grep -n "AcpBetaFeatureFlag"         crates/feature_flags/src/flags.rs
```

### 3. Walk the rebase checklist
Step through every numbered item in `portingguide.md` §"Rebase Checklist". If upstream changed any referenced field/trait/file, fix it now — don't defer.

### 4. Unit tests
```bash
cargo test -p external_websocket_sync          # expect 37 pass, ≤2 ignored
cargo test -p acp_thread test_second_send      # Stopped invariant (Critical Fix #6)
```

### 5. E2E test (the canonical regression check — **hard gate**)
```bash
cd /home/retro/work/helix && ./stack build-zed dev
cp ./zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary
cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test
./run_docker_e2e.sh                                # zed-agent only
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh  # both agents
```

All in-tree phases must pass for **both** `zed-agent` and `claude`. The contractual phases named in `requirements.md` (Phase 1, 2, 3, 4, 8, 9) are the minimum.

If any phase fails: do **not** mark the task complete. Diagnose, fix, re-run.

## Documentation Updates (incremental — not retrospective)

After **each** non-trivial conflict resolution, append to `/home/retro/work/zed/portingguide.md`:

1. **Conflict context**: file + region + which Helix concern it touches
2. **Upstream change**: the PR or commit and what it did
3. **Resolution**: what was kept from each side and why
4. **Risk**: any follow-up regression risk

Always extend the commit-history table at the bottom with this merge's commits and any follow-up fix commits. If the merge is genuinely uneventful, the porting guide may need only the commit-history append — do not invent entries.

## Helix Repo Bump

After the Zed branch is pushed:
1. In `/home/retro/work/helix/`, update `sandbox-versions.txt` `ZED_COMMIT=` to the new merge HEAD.
2. Open the Helix PR **first** (per `CLAUDE.md` ordering rule).
3. Then push/open the Zed PR against fork main with the merge commit.
4. After both merge, the build pipeline rebuilds the Zed binary + desktop image automatically.

## Open Questions (for the implementation agent to answer at runtime)

- How many upstream commits are we behind? (`git fetch upstream && git log --oneline upstream/main ^main | wc -l`.)
- Has upstream made breaking changes to the ACP protocol or agent panel APIs since `e3d1876c06`?
- Has upstream progressed on ACP session list / resume UI? If so, assess whether the Helix override `AcpBetaFeatureFlag::enabled_for_all() -> true` is still needed or can be retired.
- Has the `agent-client-protocol` schema crate added new builder patterns or `#[non_exhaustive]` markers requiring migration?
- Have any of the 001909 carry-over fixes (`--allow-multiple-instances`, `debug-embed`, `smol → executor.timer`) been silently regressed by upstream? (Re-grep after merge.)
- Have PRs #44–#47's surface-area files (`acp_thread.rs`, `agent_panel.rs`, `external_websocket_sync/`, `context_server*`) been touched by upstream in ways that complicate the merge?

## Notes

### Two-merge skip
Plans 001946 and 001947 were written 2026-04-27 and never executed. Their `requirements.md` and `design.md` remain accurate descriptions of the fork state at the time and a useful precedent for resolution patterns. The implementation agent for 001980 should read both, plus `portingguide.md`, before starting.

### Out-of-band fork pushes
The 001909 merge picked up an out-of-band fix (`d7be64fad1`) pushed to fork main while the merge branch was open. PRs #44–#47 are already baked into `f5fab97857`. If anyone pushes to fork main during this work, `git merge origin/main` into the feature branch and re-run E2E.

## Out of Scope

- Changes to Helix API, frontend, or other non-Zed components unless forced by a breaking upstream change.
- New feature development beyond what conflict resolution requires.
- Rewriting or refactoring Helix-specific crates.
