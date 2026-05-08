# Design: Merge Latest Zed Upstream (001947)

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` (fork remote `origin`, default branch `main`)
- **Upstream**: `zed-industries/zed` on GitHub — added as `upstream` remote (verify URL with `git remote -v`; if missing run `git remote add upstream https://github.com/zed-industries/zed.git`)
- **Porting guide** (canonical): `/home/retro/work/zed/portingguide.md` — already exists; do **not** create a new one at `design/zed-porting-guide.md`
- **Helix platform repo**: `/home/retro/work/helix/` — contains `sandbox-versions.txt` with `ZED_COMMIT=`

## Current State (as of 2026-04-27)

| | Commit | Date |
|---|---|---|
| Fork HEAD | `f5fab97857` ("bump context-server timeout 60s → 180s") | 2026-04-26/27 |
| Last upstream merge | `8428a4399d` (PR #43, task 001909) — integrated upstream `e3d1876c06` | 2026-04-25 |
| Upstream HEAD | TBD — fetch first | — |
| Helix-only commits since last merge | **4** (PRs #44, #45, #46, #47) | 2 days |
| Upstream commits to merge | TBD (expected very small — ~2 days) | — |

Recent merge precedent (smallest → largest):
- 001909: 86 commits, 3 days, **1** conflict — required 3 carry-over fix commits
- 001864: 920 commits, 30 days, **35** conflicts — required 5 follow-up fix commits
- 001723: 506 commits, 25 conflicts, 3 follow-up commits

This merge is expected to be smaller than 001909.

## Merge Strategy

Use `git merge upstream/main` (consistent with every previous merge — preserves Helix commit history and makes conflict resolution traceable). Do **not** rebase.

```bash
cd /home/retro/work/zed
git remote -v   # verify `upstream` points at zed-industries/zed
git fetch upstream
git checkout -b feature/001947-merge-latest-zed
git merge upstream/main
# Resolve conflicts (one at a time, updating portingguide.md as we go)
# Build + critical-fix grep + unit tests + E2E
# Push, open PR
```

### Conflict-resolution principles (carried from 001864 and 001909)

1. **Prefer upstream ordering** for shared changes (model lists, feature lists, match arms) — keeps the diff small and minimises future conflict surface.
2. **Keep Helix-specific code behind `#[cfg(feature = "external_websocket_sync")]`** — never let it leak into unconditional code paths.
3. **`Cargo.lock`**: always accept upstream (`git checkout --theirs`); regenerated on next `cargo build --features external_websocket_sync`.
4. **CI / `.github/workflows/`**: always accept upstream — Helix doesn't use Zed's CI.
5. **Upstream renames**: after every merge — even when files auto-merge cleanly — grep for the old identifiers (`ActiveView`, `set_active_view`, `selected_agent_type`, `draft_threads`, `background_threads`, etc.) to catch silent drift in cfg-gated regions.
6. **Document every non-trivial resolution in `portingguide.md` immediately**, not at the end.

## Likely Conflict Hot-spots

Even on a tiny merge, inspect each of these even if `git merge` reports "auto-merged":

| File | Risk | Helix changes at risk |
|------|------|-----------------------|
| `crates/agent_ui/src/agent_panel.rs` | **HIGH** | Thread display callback, UI state query, onboarding bypass, split-brain detection, auto-follow, ACP auto-approve, `acp_history_store()` accessor, agent_type serialisation |
| `crates/agent_ui/src/conversation_view.rs` | **HIGH** | `from_existing_thread()`, `THREAD_REGISTRY`, `is_resume`, history refresh, unregister-on-reset, `UserCreatedThread` HashMap insertion |
| `crates/acp_thread/src/acp_thread.rs` | **MEDIUM** | `content_only()`, `cancel()` drop fix, `stopped_emitted_for_task` guard, `Stopped(StopReason)` tuple, trailing-edge flush timer (PR #44) |
| `crates/acp_thread/src/connection.rs` | **MEDIUM** | `wait_for_tools_ready()` trait method |
| `crates/agent/src/agent.rs` | **MEDIUM** | Critical Fix #1 (entity lifetime), `wait_for_tools_ready` (uses `cx.background_executor().timer()` since 001909) |
| `crates/external_websocket_sync/src/thread_service.rs` | **LOW** | Critical Fixes #4, #5 — self-contained crate, conflicts rare |
| `crates/workspace/src/workspace.rs` | **LOW** | Agent follow focus guard `!matches!(leader_id, CollaboratorId::Agent)` |
| `crates/title_bar/` | **LOW** | Helix connection status indicator + optional `external_websocket_sync` dep |
| `crates/zed/Cargo.toml`, `agent_ui/Cargo.toml`, `title_bar/Cargo.toml` | **LOW** | Feature propagation chain |
| `Cargo.lock` | **TRIVIAL** | Always `--theirs` |
| `crates/zed/src/main.rs` | **LOW** | `--allow-multiple-instances` CLI flag (silently lost in 001864 — re-grep) |
| `Cargo.toml` (workspace root) | **LOW** | `rust-embed` `debug-embed` feature (also silently lost previously) |

## Resolution Patterns Worth Re-stating

- **`HeadlessConnection` is dead code** (since 001909). Current `from_existing_thread()` reuses `thread.read(cx).connection().clone()`. Don't be misled by old porting-guide entries.
- **`ConnectedServerState`** has 6 fields as of 001909: `connection`, `auth_state`, `active_id`, `threads`, `conversation`, `_connection_entry_subscription`. No `history` field. `from_existing_thread()` must list every field — upstream additions break it silently (compiles fine right up until the compiler tells you a field is missing).
- **`AcpThreadEvent::Stopped(StopReason)`** is a tuple variant — pattern matches need `Stopped(_)`.
- **`title_bar`'s `external_websocket_sync` dep MUST be `optional = true`** with the matching `[features]` entry — otherwise the cfg-gated icon silently never renders.
- **Auto-merged ≠ correct** — always grep for the renamed identifier set after the merge.
- **Build via `./stack build-zed dev`** in the Helix repo (gives both compile check and a fresh binary for E2E in one shot, ~2m).

## Post-Merge Validation

### 1. Compile check (in Docker, fast)
```bash
cd /home/retro/work/helix
./stack build-zed dev   # ~2 min, produces ./zed-build/zed
```
Also run a no-feature check: `cargo check -p zed` (proves nothing leaked outside the cfg gate).

### 2. Grep verification of critical fixes
```bash
cd /home/retro/work/zed

grep -n "load_session"               crates/agent/src/agent.rs                              | head
grep -n "content_only"               crates/acp_thread/src/acp_thread.rs
grep -n "drop(turn.send_task)"       crates/acp_thread/src/acp_thread.rs
grep -n "stopped_emitted_for_task"   crates/acp_thread/src/acp_thread.rs
grep -rn "unregister_thread"         crates/agent_ui/src/conversation_view.rs
grep -n "CollaboratorId::Agent"      crates/workspace/src/workspace.rs
grep -n "allow_multiple_instances"   crates/zed/src/main.rs
grep -n "debug-embed"                Cargo.toml
```

### 3. Walk the rebase checklist
Step through every numbered item in `portingguide.md` §"Rebase Checklist". If upstream changed any referenced field/trait/file, fix it now — don't defer.

### 4. Unit tests
```bash
cargo test -p external_websocket_sync          # expect 37 pass, ≤2 ignored
cargo test -p acp_thread test_second_send      # Stopped invariant (Critical Fix #6)
```

### 5. E2E test (the canonical regression check — hard gate)
```bash
cd /home/retro/work/helix && ./stack build-zed dev
cp ./zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary
cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test
./run_docker_e2e.sh                                # zed-agent
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh  # both agents
```

The contract from this spec: **all four phases must pass** —
- Phase 1: correct thread ID, entry count ≥ 2
- Phase 2: same thread ID, entry count increased
- Phase 3: different thread ID, entry count ≥ 2
- Phase 4: message to non-visible thread completes with no thread-load error

The in-tree test currently has 12 phases (Phases 8–9 sensitive to Critical Fixes #6/#8/#9; Phase 11 spectask routing; Phase 12 reconnect). All 12 must pass; the four named above are the contractual minimum.

If any phase fails: do **not** mark the task complete. Diagnose and fix.

## Documentation Updates (incremental — not retrospective)

After **each** non-trivial conflict resolution, append to `/home/retro/work/zed/portingguide.md`:

1. **Conflict context**: file + region + which Helix concern it touches
2. **Upstream change**: what PR or commit and what it did
3. **Resolution**: what was kept from each side and why
4. **Risk**: any follow-up regression risk

If the merge is genuinely uneventful, the porting guide may need only a commit-history append. Do **not** invent updates — keep the guide accurate. Always extend the commit-history table at the bottom.

## Helix Repo Bump

After the Zed branch is pushed:
1. In `/home/retro/work/helix/`, update `sandbox-versions.txt` `ZED_COMMIT=` to the new merge HEAD.
2. Open the Helix PR **first** (per `CLAUDE.md` ordering rule).
3. Then push/open the Zed PR.
4. After both merge, the build pipeline rebuilds the Zed binary + desktop image automatically.

## Open Questions (for the implementation agent to answer at runtime)

- How many upstream commits are we behind? (Run `git fetch upstream && git log --oneline upstream/main ^main | wc -l`.)
- Has upstream progressed on ACP session list / resume UI? If so, assess whether the Helix override (`AcpBetaFeatureFlag::enabled_for_all() -> true`) is still needed or can be removed.
- Has the `agent-client-protocol` schema crate added new builder patterns or `#[non_exhaustive]` markers requiring migration?
- Have any of the Helix carry-over fixes from 001909 (allow-multiple-instances, debug-embed, smol → executor.timer) been silently regressed by upstream? (Re-grep after merge.)

## Notes on the Out-of-Band Merge Pattern

The 001909 merge picked up an out-of-band fix (`d7be64fad1`) pushed to fork main while the merge branch was open. Today fork main has 4 such commits (PRs #44–#47) ahead of the last merge. The merge branch starts from `f5fab97857` so all 4 are baked in from the start — no out-of-band re-merge is required unless someone pushes to fork main during this work.

## Out of Scope

- Changes to Helix API, frontend, or other non-Zed components unless forced by a breaking upstream change.
- New feature development beyond what conflict resolution requires.
- Rewriting or refactoring Helix-specific crates.
