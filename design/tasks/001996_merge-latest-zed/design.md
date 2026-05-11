# Design: Merge Latest Zed Upstream (001996)

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` — fork remote `origin`, default branch `main`. Note: the local clone in this environment uses an `origin` URL pointing at the in-cluster gitea (`http://api:8080/git/...`), which is the fork — there is no separate `origin`/`fork` distinction.
- **Upstream**: `zed-industries/zed` on GitHub. The `upstream` remote is **not** configured by default in fresh clones — add it: `git remote add upstream https://github.com/zed-industries/zed.git`. (The implementation agent's first session may need to add it.)
- **Porting guide** (canonical, living document): `/home/retro/work/zed/portingguide.md` — already exists, **724 lines**. Do **not** create `design/zed-porting-guide.md`; update the in-repo file.
- **Helix platform repo**: `/home/retro/work/helix/` — contains `sandbox-versions.txt` with `ZED_COMMIT=fe8f4f4e3f0fb7c0cb51e9c8028ca0c13a8252cb` (matches current fork HEAD).

## Current State (as of 2026-05-11)

| | Commit | Date |
|---|---|---|
| Fork HEAD | `fe8f4f4e3f` (PR #53 — sidebar split-brain fix) | 2026-05-08 |
| Last upstream merge | `c3e312b056` (task 001980, integrated upstream `1da60a8518`) | 2026-05-05 |
| Upstream HEAD | `8bdd78e023` ("opencode: Update Free models (#56328)") | 2026-05-10 |
| Helix-only commits since last merge | **3** (PRs #51, #52, #53) | 6 days |
| Upstream commits to merge | **127** | 3 days |

Recent merge precedent (size → conflict count):
- 001909: 86 commits, 3 days, **1** conflict, 3 carry-over fix commits
- 001980: 172 commits, 10 days, **4** conflicts, 2 follow-up fix commits
- 001864: 920 commits, 30 days, **35** conflicts, 5 follow-up fix commits

This merge is similar in commit count and time-window to 001909 (~3 days). Conflict count likely 1–6, but **the per-file diff against `agent_panel.rs` (1282 lines) is unusually large** — expect non-trivial conflict resolution there even though the file count is small.

## Merge Strategy

Use `git merge upstream/main` (consistent with every previous merge — preserves Helix commit history and makes conflict resolution traceable). Do **not** rebase.

```bash
cd /home/retro/work/zed
git remote add upstream https://github.com/zed-industries/zed.git   # only if missing
git fetch upstream
git checkout main && git pull origin main                            # in case main moved
git checkout -b feature/001996-merge-latest-zed
git merge upstream/main
# Resolve conflicts one at a time, updating portingguide.md as each is resolved
# Build → critical-fix grep → unit tests → E2E
git push -u origin feature/001996-merge-latest-zed
# Open Helix PR (sandbox-versions.txt bump), then Zed PR
```

### Conflict-resolution principles (carried from 001864 / 001909 / 001980)

1. **Prefer upstream ordering** for shared changes (model lists, feature lists, match arms) — keeps the diff small and minimises future conflict surface.
2. **Keep Helix-specific code behind `#[cfg(feature = "external_websocket_sync")]`** — never let it leak into unconditional code paths.
3. **`Cargo.lock`**: always accept upstream (`git checkout --theirs`); regenerated on next `cargo build --features external_websocket_sync`.
4. **CI / `.github/workflows/`**: always accept upstream — Helix doesn't use Zed's CI.
5. **Upstream renames**: after every merge — even when files auto-merge cleanly — grep for the old identifiers in cfg-gated regions to catch silent drift.
6. **Document every non-trivial resolution in `portingguide.md` immediately**, not at the end.
7. **If a conflict is too risky to resolve**: stop, document explicitly in `portingguide.md`, and escalate to the user rather than guess.

## Likely Conflict Hot-spots

For this merge, the upstream diff sizes against Helix-touched files are known up-front. Inspect each even if `git merge` reports "auto-merged":

| File | Upstream diff | Risk | Helix concern |
|------|---------------|------|---------------|
| `crates/agent_ui/src/agent_panel.rs` | **1282 lines** | **HIGH** | Thread display callback, UI state query, onboarding bypass, `acp_history_store()`, ACP auto-approve, agent_type serialisation, **Critical Fix #11** (PR #53 — entity-identity guard at top of `load_agent_thread`) |
| `crates/acp_thread/src/acp_thread.rs` | 546 lines | **HIGH** | `content_only()`, Critical Fixes #6/#8/#9, **upstream `0a52f80824` PR #55562 directly touches `running_turn` clearing on tx drop** — overlaps Helix cancel/Stopped fixes |
| `crates/agent_ui/src/conversation_view.rs` | 521 lines | **HIGH** | `from_existing_thread()`, `THREAD_REGISTRY`, `is_resume`, history refresh, unregister-on-reset |
| `crates/agent/src/agent.rs` | 55 lines | MEDIUM | Critical Fix #1 (entity lifetime), `wait_for_tools_ready` (uses `cx.background_executor().timer()`) |
| `crates/external_websocket_sync/src/thread_service.rs` | (Helix-only file, but recently grew) | MEDIUM | Critical Fixes #4, #5; PR #45 turn_request_id refresh; PR #46 `AgentConnectionCache`; **PR #52 `cancel_current_turn` handler** |
| `crates/external_websocket_sync/src/external_websocket_sync.rs` | (Helix-only) | MEDIUM | **PR #52 added `cancel_current_turn` routing** |
| `crates/external_websocket_sync/src/types.rs` | (Helix-only) | LOW | **PR #52 added `cancel_current_turn` command + `turn_cancelled` event types** |
| `crates/external_websocket_sync/src/websocket_sync.rs` | (Helix-only) | LOW | **PR #52 added 15-line cancel routing** |
| `crates/agent_settings/src/agent_settings.rs` | 6 lines | LOW | `show_onboarding`, `auto_open_panel` |
| `crates/workspace/src/workspace.rs` | TBD | LOW | Agent follow focus guard |
| `crates/title_bar/` | TBD | LOW | Helix connection status indicator + optional `external_websocket_sync` dep |
| `crates/zed/src/main.rs` | TBD | LOW | `--allow-multiple-instances`, `--headless` flags |
| `Cargo.toml` (workspace root) | TBD | LOW | `rust-embed` `debug-embed` feature |
| `Cargo.lock` | always | TRIVIAL | `--theirs` |

### Highest-risk single hunk: upstream `0a52f80824` (#55562)

Upstream `acp_thread: Clear running_turn when prompt task drops tx (#55562)` modifies the same code path Helix has heavily customised across:
- Critical Fix #6 (`Stopped` invariant — exactly one per `send()`)
- Critical Fix #8 (`cancel()` drops `send_task`)
- Critical Fix #9 (`stopped_emitted_for_task` guard)
- PR #52 `cancel_current_turn` (added Apr 2026 — clears active turn from a WebSocket command)

The likely interaction is that upstream's clearing of `running_turn` on tx-drop may *now* emit `Stopped` from a path Helix didn't expect, potentially conflicting with the `stopped_emitted_for_task` guard. Read the full diff of `0a52f80824` before resolving any conflict in `acp_thread.rs`. After resolution, **always** run E2E phases 8, 9, 13, and 14 — they cover this code path end-to-end.

## Resolution Patterns Worth Re-stating

- **`HeadlessConnection` is dead code** (since 001909). Don't be misled by old porting-guide entries.
- **`ConnectedServerState`** as of 001980 is stable at 6 fields: `connection`, `auth_state`, `active_id`, `threads`, `conversation`, `_connection_entry_subscription`. Re-grep after merge — upstream additions break `from_existing_thread()` silently.
- **`AcpThreadEvent::Stopped(StopReason)`** is a tuple variant — pattern matches need `Stopped(_)`. **Test code is not exempt.** Add `grep -n "AcpThreadEvent::Stopped\b\([^(]\|$\)" crates/acp_thread/src/` to silent-drift sweep (per checklist 41a, learned in 001980).
- **`title_bar`'s `external_websocket_sync` dep MUST be `optional = true`** with the matching `[features]` entry — otherwise the cfg-gated icon silently never renders.
- **Auto-merged ≠ correct** — always grep for the renamed identifier set after the merge.
- **Build via `./stack build-zed dev`** in the Helix repo (compile check + fresh binary for E2E in one shot, ~2 min warm cache).
- **`go mod tidy` in `e2e-test/helix-ws-test-server/`** before E2E — the runner doesn't tidy itself, and Helix Go deps drift between merges (per 001980 lesson).

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

# Test-pattern drift (lesson from 001980 — checklist 41a)
grep -n "AcpThreadEvent::Stopped\b\([^(]\|$\)" crates/acp_thread/src/   # should be 0

# Upstream rename silent drift (lessons from 001864/001909/001980)
grep -rn "ActiveView"                crates/agent_ui/src/   # should be 0 (renamed BaseView)
grep -rn "set_active_view"           crates/agent_ui/src/   # should be 0
grep -rn "draft_threads\|background_threads" crates/agent_ui/src/   # should be 0 (now retained_threads)
grep -rn "selected_agent_type"       crates/agent_ui/src/   # should be 0 (now selected_agent)

# Carry-over fixes (silently lost previously)
grep -n "allow_multiple_instances"   crates/zed/src/main.rs
grep -n "headless"                   crates/zed/src/main.rs   # PR #51, all 3 call sites
grep -n "debug-embed"                Cargo.toml
grep -n "smol::Timer"                crates/agent/src/agent.rs   # should be 0

# Helix-specific surface
grep -n "CollaboratorId::Agent"      crates/workspace/src/workspace.rs
grep -n "AcpBetaFeatureFlag"         crates/feature_flags/src/flags.rs

# PR #52 cancel_current_turn (new since 001980)
grep -rn "cancel_current_turn"       crates/external_websocket_sync/
grep -rn "turn_cancelled"            crates/external_websocket_sync/
```

### 3. Walk the rebase checklist
Step through every numbered item in `portingguide.md` §"Rebase Checklist" (currently 44 items, including the new 41a from 001980 and the new #11 added by PR #53). Pay special attention to items 31, 31a, 37 (the `acp_thread.rs` cancel/Stopped territory) given the overlap with upstream `0a52f80824`.

### 4. Unit tests (if local Rust toolchain available)
```bash
cargo test -p external_websocket_sync          # full pass, ≤2 ignored
cargo test -p acp_thread test_second_send      # Stopped invariant (Critical Fix #6)
cargo test -p external_websocket_sync cancel_current_turn   # PR #52 protocol tests
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

All in-tree phases must pass for **both** `zed-agent` and `claude`. The contractual phases named in `requirements.md` (Phase 1, 2, 3, 4, 8, 9, **13**, **14**) are the minimum. Phases 13 and 14 are new since 001980 (added by PR #52) and exercise the `cancel_current_turn` ↔ `turn_cancelled` round-trip.

If any phase fails: do **not** mark the task complete. Diagnose, fix, re-run.

## Documentation Updates (incremental — not retrospective)

After **each** non-trivial conflict resolution, append to `/home/retro/work/zed/portingguide.md`:

1. **Conflict context**: file + region + which Helix concern it touches
2. **Upstream change**: the PR or commit and what it did
3. **Resolution**: what was kept from each side and why
4. **Risk**: any follow-up regression risk

Always extend the commit-history table at the bottom with this merge's commits and any follow-up fix commits. Add a new top-level section `## Merge 001996 (2026-05-11)` mirroring the structure of `## Merge 001980`. If the merge is genuinely uneventful, the porting guide may need only the commit-history append — do not invent entries.

If upstream `0a52f80824` requires resolution in `acp_thread.rs`, write a dedicated subsection explaining the interaction with Critical Fixes #6/#8/#9 and PR #52's `cancel_current_turn` — this is the spec's main predicted gotcha and is exactly the sort of context future merge engineers need.

## Helix Repo Bump

After the Zed branch is pushed:
1. In `/home/retro/work/helix/`, update `sandbox-versions.txt` `ZED_COMMIT=` to the new merge HEAD.
2. Open the Helix PR **first** (per `CLAUDE.md` ordering rule).
3. Then push/open the Zed PR against fork main with the merge commit.
4. After both merge, the build pipeline rebuilds the Zed binary + desktop image automatically.

## Open Questions (for the implementation agent to answer at runtime)

- Does upstream `0a52f80824` (#55562) interact with the Helix `stopped_emitted_for_task` guard or PR #52's `cancel_current_turn`? If yes, document the resolution carefully — this is the single most likely source of subtle regression in this merge.
- Have any of the silent-drift identifiers (`ActiveView`, `selected_agent_type`, `draft_threads`, `Stopped` unit-pattern) re-appeared anywhere upstream renamed code in the 1282-line `agent_panel.rs` diff?
- Has upstream made breaking changes to `ConnectedServerState` field set since 001980? Walk `from_existing_thread()` against the live struct after merge.
- Have any of the carry-over fixes (`--allow-multiple-instances`, `--headless`, `debug-embed`, `smol → executor.timer`) been silently regressed by upstream? Re-grep after merge.
- Has the `agent-client-protocol` schema crate added new builder patterns or `#[non_exhaustive]` markers requiring migration?
- Did anyone push to fork main during the merge? If so, `git merge origin/main` into the feature branch and re-run E2E.

## Notes

### Out-of-band fork pushes
The 001909 and 001980 merges both picked up out-of-band fixes pushed to fork main while the merge branches were open. Treat this as expected — re-merge `origin/main` into the feature branch before opening the PR if needed.

### `stack` is the canonical builder
There is no local Rust toolchain in this environment for the implementation agent — all builds go via `cd /home/retro/work/helix && ./stack build-zed dev` (Docker-based, persistent cache, ~2 min warm). `cargo check` / `cargo test` items in the validation list are **best-effort** for environments where Rust is installed; the E2E gate is the hard contractual requirement.

## Out of Scope

- Changes to Helix API, frontend, or other non-Zed components unless forced by a breaking upstream change.
- New feature development beyond what conflict resolution requires.
- Rewriting or refactoring Helix-specific crates.
