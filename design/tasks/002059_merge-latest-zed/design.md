# Design: Merge Latest Zed Upstream (002059)

## Posture: How This Merge Relates to Still-Open 002029

Prior task 002029's PRs (`zed#58`, `helix#2480`) are still **open** as of
2026-06-01. Three options exist; the implementation agent must check the
current state of these PRs at the start of the task and choose:

| Option | When to pick | Branch | Baseline |
|---|---|---|---|
| **(a) Land 002029 first** | 002029 is reviewable; another maintainer can merge it within a day or two | `feature/002059-merge-latest-zed` cut from updated `main` | Upstream at `13e7c11768` after 002029 lands |
| **(b) Stack on 002029** | 002029 is stalled and the reviewer agrees to extend it | `feature/002059-merge-latest-zed` cut from `feature/002029-merge-latest-zed` tip | Upstream at `13e7c11768` (002029's fence) |
| **(c) Take over 002029** | 002029 is abandoned; merge new upstream onto the existing branch | reuse `feature/002029-merge-latest-zed`; close old PR or push new commits | same as (b) |

**Recommendation: (a) is preferred** — it keeps the history clean and decouples
review of the next merge from the (stale) 002029 review. If 002029 cannot be
landed promptly, fall back to (b). Do not silently abandon 002029's work; if
choosing (a), confirm with the reviewer that 002029 will land first.

Whichever option is picked, **do not reuse the branch name
`feature/002029-merge-latest-zed`** for new commits unrelated to that PR — it
exists on origin and would be confusing.

## Architecture

This is a routine merge, not a redesign. The architecture is whatever
`portingguide.md` already describes:

- **WebSocket sync layer** (`crates/external_websocket_sync/`) — Helix-only crate
  bridging the agent panel to the Helix API server.
- **Agent panel callbacks** (`crates/agent_ui/src/agent_panel.rs`) — globals
  for thread creation/display/open/UI-state-query.
- **ACP cancel protocol** (`crates/acp_thread/src/acp_thread.rs`) — Helix
  Critical Fixes #6/#8/#9 plus PR #52.
- **Session creation chain** (`crates/agent_servers/src/acp.rs`) — PR #50.
- **Draft-thread suppression** under `external_websocket_sync` — PR #56 Fix 1b,
  position-critical in `ensure_thread_initialized`.
- **Headless mode** (`crates/zed/src/main.rs`) — `--headless`, `--allow-multiple-instances`.
- **Eleven Critical Fixes**, of which #10 (180s context-server timeout) is
  **retired** as of `e60a1b2789`; the other ten remain active.

Full surface and per-file change list lives in `/home/retro/work/zed/portingguide.md`.
Read it end-to-end before starting.

## Key Decisions

### 1. Branch and PR layout
- Zed branch: `feature/002059-merge-latest-zed` on `helixml/zed`.
- Helix branch: same name on `helixml/helix`.
- Open Helix companion PR (bumps `ZED_COMMIT` in `sandbox-versions.txt`) **before**
  the Zed PR merges, per the convention from 001980's CLAUDE.md citation.

### 2. Live porting-guide updates
After each conflict is resolved (or each pre-existing breakage is repaired),
add an entry to `portingguide.md` in the merge's `## Merge 002059 (YYYY-MM-DD)`
section **and commit it**. Do not batch porting-guide updates at the end.
Convention from 002029: dedicate at least one commit titled `porting guide` and
one titled `docs porting entry`.

### 3. Conflict resolution defaults
- **`Cargo.lock`**: `--theirs` (regenerates on next `cargo build`).
- **`.github/workflows/*`**: accept upstream (Helix doesn't use Zed CI).
- **Helix-specific Rust code**: keep Helix; absorb any compatible upstream
  improvements. Only retire a Helix patch if upstream genuinely subsumes the
  intent (verify, don't assume — see the 002029 `extensions_ui.rs` bypass-markers
  example where they were *almost* retired wrongly).
- **Critical Fixes are not immortal**: if a Helix fix is now wrong (Fix #10
  precedent), retire it and document why in the porting guide.

### 4. Hard gates
- `cargo test` — zero failures.
- E2E: **all 17 phases × 2 personalities (`zed-agent`, `claude`)** must pass.
  Specific gates: Phase 13 (turn_cancelled ordering), Phase 15 (PR #55), Phase
  16 (PR #56 Fix 1a + PR #57), Phase 17 (PR #56 Fix 1b).
- Phase 17 failure means **restore the cfg-gated early return at the top of
  `ensure_thread_initialized`'s `BaseView::Uninitialized` branch** before
  declaring done. Do not mark the task complete with Phase 17 failing.

## Recurring Conflict Hotspots (from 001980/001996/002029)

Expect conflicts or silent-drift breakage in these files. Re-grep after the merge.

| File | Why it conflicts |
|---|---|
| `crates/acp_thread/src/acp_thread.rs` | `run_turn` cancel/Stopped state machine — Critical Fixes #6/#8/#9 + PR #52. Strict-superset resolution. |
| `crates/agent_ui/src/agent_panel.rs` | Callbacks, `ActiveView`/`BaseView`/`ContextServerStatus` non-exhaustive matches, Fix 1b position, Critical Fix #11 signature drift. |
| `crates/agent_ui/src/conversation_view.rs` | `from_existing_thread()` — silent-break magnet whenever upstream extends `ThreadView::new` args or `ConnectedServerState` fields. |
| `crates/external_websocket_sync/src/thread_service.rs` | Event ordering races (GPUI flushes at end of update closure); Phase 13 hard gate. |
| `crates/agent/src/agent.rs` | `NativeAgent` restructure, `load_session` entity lifetime, trait signatures (e.g. `supports_delete(&self, &App)`). |
| `crates/acp_thread/src/connection.rs` | `AgentConnection` trait additions. |
| `crates/agent_servers/src/acp.rs` | PR #50 `session_creation_chain` wrap — upstream `SessionDirectories` work expanded heavily in 002029. |
| `crates/zed/src/main.rs` | `--headless` (3 call sites + `initialize_headless()`) and `--allow-multiple-instances` repeatedly lost in older merges. |
| `crates/agent_settings/src/agent_settings.rs` | Helix fields collide with upstream removals (e.g. `new_thread_location` in #55575). |
| `crates/workspace/src/workspace.rs` | `CollaboratorId::Agent` focus guard in `follow()`/`update_follower_items()`. |
| `crates/title_bar/Cargo.toml` + `src/title_bar.rs` | `external_websocket_sync` optional dep; `render_restricted_mode` cfg-gated branch. |
| `crates/extensions_ui/src/extensions_ui.rs` | `// HELIX: External agent ...` bypass markers ~lines 245 and 1586. |
| `crates/project/src/agent_server_store.rs` | `reregister_agents` destructure pattern. |

## Patches to Preserve

All of the following are still load-bearing as of `e60a1b2789`. Full
descriptions in research notes; this is the checklist.

- PRs: **#50** (session_creation_chain), **#55** (EntryUpdated emit), **#56 Fix 1a**
  (deferred UserCreatedThread), **#56 Fix 1b** (draft suppression — position-critical),
  **#57** (Phase 16 counter exclusion), direct **`fd26c1a113`** (Dockerfile.ci helix-org pull).
- **Critical Fixes #1–#9, #11** (#10 retired and stays retired).
- The entire `external_websocket_sync` crate.
- `AcpBetaFeatureFlag::enabled_for_all() -> true`.
- Helix CLI flags `--headless` (with `initialize_headless()` and all call sites)
  and `--allow-multiple-instances`.
- All entries in the "Helix Patches" list of the research synthesis — see
  prior task `002029_merge-latest-zed/design.md` and `portingguide.md` for the
  full enumerated set.

## Patches to Re-evaluate (Not Pre-emptively Retire)

- `extensions_ui.rs` `// HELIX:` bypass markers — re-verify against current upstream
  (002029 confirmed still needed despite `c84c22dab5` reshape; do same check here).
- `AcpBetaFeatureFlag::enabled_for_all()` override — only retire if upstream
  has matured the ACP session list/resume UI to the point where the override
  is no longer needed.
- `title_bar/Cargo.toml` `feature_flags.workspace` dep — 002029 dropped it once;
  re-evaluate if upstream re-added a consumer.

## Silent-Drift Greps (must be zero after merge, in `agent_ui/src/` and `agent/src/agent.rs`)

- `ActiveView`, `set_active_view`, `draft_threads`, `background_threads`,
  `selected_agent_type`, `smol::Timer`.
- `Stopped` not followed by `(` (must be `Stopped(_)` tuple-pattern, including in tests).
- Re-grep `ConnectedServerState` field set; mirror into `from_existing_thread()`
  if upstream widened the struct (002029 added: `code_span_resolver`,
  `last_theme_id`, `draft_prompt_persist_task`, `available_skills`).

## Build & Test

- Build: `./stack build-zed dev` (warm ~46s–1m31s, cold ~6m35s).
- E2E: run `crates/external_websocket_sync/e2e-test/run_e2e.sh`. Before running,
  `go mod tidy` in `crates/external_websocket_sync/e2e-test/helix-ws-test-server/` —
  the runner does **not** tidy itself (lesson from 001980).
- E2E flake to ignore: claude-agent Phase 1 npm-install bootstrap can time out
  with 0 events received; re-run is the correct response.

## Risks and Notes

- **MAJOR**: 002029's stale PRs may carry conflict-resolution patterns that
  002059 will re-encounter. Prefer landing 002029 first so the porting guide
  on `main` has the 002029 narrative as reference.
- **Draft-threads philosophical conflict** (upstream `bbe23cc40b` "Bring back
  draft threads" vs Helix PR #56 Fix 1b) — if upstream continues doubling down
  in this space, the suppression may need structural rethinking rather than a
  mechanical port. Flag and escalate rather than guess.
- **Fix 1b position** must remain the **first** statement of
  `BaseView::Uninitialized` branch in `ensure_thread_initialized` — before
  `pending_terminal_spawn`, `should_create_terminal_for_new_entry`, ACP-restoration
  branches. Phase 17 is the gate.
- **Out-of-band fork pushes are routine** — `git merge origin/main` into the
  feature branch and re-run E2E whenever they happen.
- **Cargo.lock**: always `--theirs`.
- **`.github/workflows/*`**: always accept upstream.
