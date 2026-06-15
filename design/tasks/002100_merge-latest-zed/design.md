# Design: Merge Latest Zed Upstream (002100)

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` — fork remote `origin` (in-cluster gitea URL is `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`). The in-cluster URL **is** the fork.
- **Upstream**: `zed-industries/zed`. The `upstream` remote is **not configured** in fresh clones — add it: `git remote add upstream https://github.com/zed-industries/zed.git`.
- **Porting guide** (canonical, living document): `/home/retro/work/zed/portingguide.md` — exists, **966 lines** as of start of this task with the latest entry `## Merge 002077 (2026-06-12)` at line 667.
- **Helix platform repo**: `/home/retro/work/helix/` — `sandbox-versions.txt` carries `ZED_COMMIT=f82e1c676099470ecd17590878a00bd25b342f82`. This is **exactly at fork HEAD** (002077 already bumped the sandbox; no catch-up debt to clear before the merge).

## Current State (as of 2026-06-15)

| | Commit | Date |
|---|---|---|
| Fork HEAD | `f82e1c6760` ("Merge pull request #61 from helixml/feature/002077-merge-latest-zed") | 2026-06-12 |
| Last upstream merge fence | `992f395c3d` ("editor: Fix columnar selection alignment on rows with multi-byte chars (#57097)") — absorbed in 002077 | 2026-06-12 |
| Upstream HEAD | `cccc7b2d44` ("editor: Fix splitting brackets inside line comments (#59260)") | 2026-06-15 |
| Helix-only commits since 002077 | **0** | 3 days |
| Upstream commits to merge | **21** | 3 days |

Recent merge precedent (size → conflict count):
- 002029: 261 commits, 10 days, **7** manual conflicts (incl. Fix 1b position-critical), 3 build-fix commits
- 002029-extension: 287 commits, 3 days, **0** manual conflicts (`ort` auto-resolved), 1 signature-drift repair
- 002029-extension round 2: 242 commits, 8 days, **4** manual conflicts ("both sides added a field"), 3 signature-drift repairs
- 002077: 256 commits, 10 days, **6** trivial conflicts (5 workflow/deletion + 1 import-block), **0** signature-drift repairs, E2E green first try
- **002100 outlook: 21 commits, 3 days** — the smallest catch-up window of any merge in this series. Predict **0–2 trivial conflicts** (possibly only `Cargo.lock`) and **0** signature-drift repairs. No commits in this window touch `acp_thread/`, `agent/src/`, `workspace.rs`, `zed/src/main.rs`, `title_bar/`, `feature_flags/`, `agent_servers/`, `external_websocket_sync/`, `agent_settings/`, or `settings_content/`.

## Merge Strategy

Use `git merge upstream/main` (consistent with every previous merge — preserves Helix commit history and makes conflict resolution traceable). Do **not** rebase.

```bash
cd /home/retro/work/zed
git remote add upstream https://github.com/zed-industries/zed.git   # only if missing
git fetch upstream
git checkout main && git pull origin main                            # in case main moved
git checkout -b feature/002100-merge-latest-zed
git merge upstream/main
# Resolve conflicts (predicted 0–2) one at a time, updating portingguide.md as each is resolved.
# Build → critical-fix grep → unit tests → E2E (Phase 9 for PR #60, Phase 17 for Fix 1b).
git push -u origin feature/002100-merge-latest-zed
# Write pull_request_zed.md + pull_request_helix.md; bump ZED_COMMIT in helix.
```

### Conflict-resolution principles (carried from 001864 / 001909 / 001980 / 001996 / 002029 / 002077)

1. **Prefer upstream ordering** for shared changes (model lists, feature lists, match arms) — keeps the diff small and minimises future conflict surface.
2. **Keep Helix-specific code behind `#[cfg(feature = "external_websocket_sync")]`** — never let it leak into unconditional code paths.
3. **`Cargo.lock`**: always accept upstream (`git checkout --theirs`); regenerated on next `cargo build --features external_websocket_sync`.
4. **CI / `.github/workflows/`**: always accept upstream — Helix doesn't use Zed's CI.
5. **Upstream renames**: after every merge — even when files auto-merge cleanly — grep for the old identifiers in cfg-gated regions.
6. **Trait-signature changes**: walk all impls compile-driven; the post-merge build is the safety net.
7. **Document every non-trivial resolution in `portingguide.md` immediately**, not at the end. **If the merge is conflict-free, still write the entry incrementally** — start it when `git merge upstream/main` is invoked.
8. **If a conflict is too risky to resolve**: stop, document explicitly in `portingguide.md`, and escalate to the user.

## Likely Conflict Hot-spots (this window)

Upstream changes to Helix-touched files (`992f395c3d..upstream/main`, measured 2026-06-15). Inspect each even if `git merge` reports "auto-merged":

| File | Commits | +/- lines | Risk | Helix concern |
|------|---------|-----------|------|---------------|
| `crates/agent_ui/src/agent_panel.rs` | 1 | +1 / -5 | **LOW** | `1e017d04b9` "Remove dead link in agent menu (#59232)" deletes a `menu.entry("Rules Library", …)` at upstream line ~5690. Fix 1b at line 5420 is in a different function and a different branch (`BaseView::Uninitialized` of `ensure_thread_initialized`). Re-grep `ensure_thread_initialized` after the merge to confirm position. |
| `crates/extensions_ui/src/extensions_ui.rs` | 1 | +27 / -24 | **LOW** | `f39cf25c0b` "Hide agent servers from chips (#59231)" rewrites the chip filter from `.filter_map` to `.filter().map()` and adds `ExtensionProvides::AgentServers`/`Grammars` to the suppressed list at upstream line ~1738. The three Helix `// HELIX: External agent …` bypass markers live in different regions (lines 226, 248, 1518) — verify all three still present post-merge. |
| `crates/agent_ui/src/threads_archive_view.rs` | 1 | +22 / -9 | LOW | `df9c9f055e` "Match project name in archive-view search (#58214)" — Helix has no patch here; informational only. |
| `crates/agent_ui/src/completion_provider.rs` | 1 | +4 / -4 | LOW | `c7987fabf7` "Truncate long model names in config option selector (#57808)" — Helix has no patch here. |
| `crates/agent_ui/src/config_options.rs` | 1 | +8 / -1 | LOW | Same upstream commit as `completion_provider.rs` — Helix has no patch here. |
| `Cargo.toml` | small | small | LOW | `objc2` 0.6 added + `objc2-app-kit` 0.3 → 0.3.2 with feature widening + `[patch.crates-io] async-process = …` (from `d4cc8d2409`). None of these touch Helix workspace members (`cloud_api_types`, `external_websocket_sync`) or `rust-embed`'s `debug-embed` feature. Predict clean auto-merge. |
| `Cargo.lock` | always | always | TRIVIAL | `--theirs`. |
| `crates/acp_thread/` | **0** | **0** | NONE | Untouched — PR #55 emit + Critical Fixes #6/#8/#9 sit on a stable upstream base this window. |
| `crates/agent/src/` | **0** | **0** | NONE | Untouched — Critical Fix #1, `wait_for_tools_ready`, compaction state machine all stable this window. |
| `crates/workspace/src/workspace.rs` | **0** | **0** | NONE | Untouched — no further typed-error tightening; `CollaboratorId::Agent` guard stable. |
| `crates/zed/src/main.rs` | **0** | **0** | NONE | Untouched — `--headless` / `--allow-multiple-instances` / `build_application(headless: bool)` stable. |
| `crates/title_bar/` | **0** | **0** | NONE | Untouched — `optional = true` external_websocket_sync dep + `render_restricted_mode` cfg gate stable. |
| `crates/feature_flags/` | **0** | **0** | NONE | Untouched — `AcpBetaFeatureFlag` override stable. |
| `crates/agent_servers/src/acp.rs` | **0** | **0** | NONE | Untouched — PR #50 `session_creation_chain` + `_settings_subscription` stable. |
| `crates/agent_settings/`, `crates/settings_content/` | **0** | **0** | NONE | Untouched — `auto_compact` / `show_onboarding` / `auto_open_panel` / `sandbox_permissions` coexistence stable. |
| `crates/external_websocket_sync/` | **0** | **0** | NONE | Untouched both ways — no upstream churn AND no fork commits since 002077. PR #60 retry loop unchanged. |

### Specific commits worth naming

- **`f39cf25c0b` "extension_ui: Hide agent servers from chips (#59231)"** — only one of the 21 commits to materially restructure a Helix-touched file. The Helix bypass markers are in a different region; the merge should auto-resolve. Confirm by `grep -n "HELIX: External agent" crates/extensions_ui/src/extensions_ui.rs` post-merge (expect 3 hits — current lines 226, 248, 1518 may shift).
- **`1e017d04b9` "agent_ui: Remove dead link in agent menu (#59232)"** — single-hunk deletion in the agent menu near line 5690. Fix 1b at line 5420 is in `ensure_thread_initialized`'s `BaseView::Uninitialized` arm — different region, different function. The standard caution applies: re-read `ensure_thread_initialized` end-to-end after the merge to confirm Fix 1b is still the FIRST statement of `BaseView::Uninitialized`.
- **`d4cc8d2409` "Patch async-process to allow reusing their reaper (#59156)"** — adds a `[patch.crates-io]` entry pointing at a Zed Industries fork of `async-process`. Helix's `Cargo.toml` does not currently carry any `[patch.crates-io]` entry, so this should land cleanly as an upstream-only addition. Will trigger a `Cargo.lock` regeneration on next build.
- **`138139f830` "gpui_macos: Fix traffic light hitbox after repositioning (#58534)"** + **`fca2ccd403` Revert "gpui: Fix title bar clicks being delayed on macOS 27 (#58947)"** — both macOS-only `gpui_macos/window.rs` changes (+222 lines combined). No Linux build impact; not exercised by the Helix CI or the Docker E2E.

## Resolution Patterns Worth Re-stating

- **`HeadlessConnection` is dead code** (since 001909). Don't be misled by old porting-guide entries.
- **`ConversationView` field set was 15 fields after 002077.** No upstream churn in `conversation_view.rs` this window — a fifth `from_existing_thread()` signature-drift repair is unlikely, but compare field-by-field as a habit.
- **`AcpThreadEvent::Stopped(StopReason)`** is a tuple variant — pattern matches need `Stopped(_)`. Test code is not exempt.
- **`BaseView::*` arms** must remain exhaustive — Helix UI state queries in `agent_panel.rs::new()` and `zed/src/main.rs` headless responder.
- **`ContextServerStatus::*` arms** likewise — `ClientSecretRequired { .. }` was added in 002029.
- **`title_bar`'s `external_websocket_sync` dep MUST be `optional = true`** with the matching `[features]` entry.
- **Auto-merged ≠ correct** — always grep for renamed identifiers after the merge.
- **Build via `./stack build-zed dev`** in the Helix repo (~46s–1m31s warm cache; 002077 cold-cache took 8m 14s). No local Rust toolchain in this environment.
- **`go mod tidy` in `e2e-test/helix-ws-test-server/`** before E2E — the runner doesn't tidy itself. The 002077 final commit (`9f2ee85268`) was exactly this tidy.
- **GPUI events flush at end of entity update closure** (lesson from 001996 Phase 13 fix).
- **`build_application(headless: bool)` pattern** (002029-round-2) — re-verify the parameter still threads through if upstream refactors `main.rs` (no churn this window).
- **PR #60 retry loop** in `external_websocket_sync/src/thread_service.rs::handle_follow_up_message` — 4 attempts × 750ms backoff on `ede_diagnostic` transient. Phase 9 of the E2E is the regression gate. No upstream churn in this file.

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

# Critical fixes (Helix)
grep -n "load_session\|pending_sessions"      crates/agent/src/agent.rs                              | head
grep -n "content_only"                        crates/acp_thread/src/acp_thread.rs
grep -n "drop(turn.send_task)"                crates/acp_thread/src/acp_thread.rs
grep -n "stopped_emitted_for_task"            crates/acp_thread/src/acp_thread.rs
grep -rn "unregister_thread"                  crates/agent_ui/src/conversation_view.rs
grep -n "ThreadMetadataStore\|load_agent_thread" crates/agent_ui/src/agent_panel.rs   # Critical Fix #11

# PR #50 (serialize ACP session creation) — must coexist with _settings_subscription
grep -n "session_creation_chain"              crates/agent_servers/src/acp.rs
grep -n "_settings_subscription"              crates/agent_servers/src/acp.rs

# PR #55 (streaming-reveal EntryUpdated emit)
grep -n "EntryUpdated"                        crates/acp_thread/src/acp_thread.rs

# PR #56 (Fix 1a deferred UserCreatedThread + Fix 1b draft suppression)
grep -n "ensure_thread_initialized"           crates/agent_ui/src/agent_panel.rs
# Read the FULL function body — the Helix cfg-gated `return;` MUST be the FIRST statement
# of the BaseView::Uninitialized branch.
grep -rn "defer.*UserCreatedThread\|first_user_message" crates/external_websocket_sync/

# PR #60 retry loop
grep -n "ede_diagnostic\|handle_follow_up_message" crates/external_websocket_sync/src/thread_service.rs

# fd26c1a113 (Dockerfile.ci helix-org)
grep -n "helix-org"                           crates/external_websocket_sync/e2e-test/Dockerfile.ci

# PR #57 (Go test-server Phase 16 counter fix)
grep -n "phase10\|Phase 10's own"             crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go

# Helix bypass markers (line numbers may shift after f39cf25c0b)
grep -n "HELIX: External agent"               crates/extensions_ui/src/extensions_ui.rs   # expect 3 hits

# Test-pattern drift
grep -nE "AcpThreadEvent::Stopped\b([^(]|$)"  crates/acp_thread/src/   # should be 0

# Upstream rename silent drift
grep -rn "ActiveView"                         crates/agent_ui/src/   # only AgentPanelEvent::ActiveView*
grep -rn "set_active_view"                    crates/agent_ui/src/   # should be 0
grep -rn "draft_threads\|background_threads"  crates/agent_ui/src/   # should be 0
grep -rn "selected_agent_type"                crates/agent_ui/src/   # should be 0

# Carry-over fixes
grep -n "allow_multiple_instances\|headless\|build_application" crates/zed/src/main.rs
grep -n "debug-embed"                         Cargo.toml
grep -n "smol::Timer"                         crates/agent/src/agent.rs   # should be 0

# Helix-specific surface
grep -n "CollaboratorId::Agent"               crates/workspace/src/workspace.rs
grep -n "render_restricted_mode"              crates/title_bar/src/title_bar.rs
grep -n "AcpBetaFeatureFlag\|enabled_for_all" crates/feature_flags/src/flags.rs

# Workspace member sanity (no upstream churn expected, but if Cargo.toml conflicted, verify)
grep -n "external_websocket_sync\|cloud_api_types" Cargo.toml
```

### 3. Walk the rebase checklist
Step through every numbered item in `portingguide.md` §"Rebase Checklist". Pay special attention to:
- Item 9 (`agent_panel.rs` cfg-gated blocks) — Fix 1b position regression (only one upstream commit touches `agent_panel.rs`, in a different region)
- Item 11 (`ConversationView` / `ConnectedServerState`) — no `conversation_view.rs` upstream churn; expect no repair
- Items 31, 31a, 37 (`acp_thread.rs` cancel/Stopped) — no upstream churn; expect intact
- 002077 additions on Fix 1b first-statement and `supports_delete(&self, &App)` signature — no related upstream churn

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
(cd helix-ws-test-server && go mod tidy)            # per 001980/002077 lesson — runner doesn't tidy
./run_docker_e2e.sh                                  # zed-agent only
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh    # both agents
```

All 17 phases must pass for **both** `zed-agent` and `claude`. Special gates:
- **Phase 9** (rapid 3-turn cancel) — PR #60's retry-loop survival
- **Phase 13** (`cancel_current_turn` happy path)
- **Phase 15** (streaming patches arrive incrementally) — PR #55 emit
- **Phase 16** (zero spontaneous `UserCreatedThread`) — PR #56 Fix 1a + PR #57
- **Phase 17** (live Claude process count == real thread count) — PR #56 Fix 1b

If any phase fails: do **not** mark the task complete. Diagnose, fix, document in `portingguide.md`, re-run.

One retry permitted for Claude Phase 1 npm-install bootstrap flake.

## Documentation Updates (incremental — not retrospective)

After **each** non-trivial conflict resolution, append to `/home/retro/work/zed/portingguide.md`:

1. **Conflict context**: file + region + which Helix concern it touches
2. **Upstream change**: the PR or commit and what it did
3. **Resolution**: what was kept from each side and why
4. **Risk**: any follow-up regression risk

Add new top-level section `## Merge 002100 (2026-06-15)` at the top of the merge-history list, mirroring the 002077 structure.

**Minimum subsections (regardless of how the merge auto-resolves)**:

1. **Window summary** — "21 upstream commits over 3 days; smallest catch-up window in this series."
2. **Helix-surface auto-merge survival check** — per-area confirmation (`agent_panel.rs` Fix 1b position; `extensions_ui.rs` three `// HELIX:` bypass markers; `external_websocket_sync/` untouched; `acp_thread/`, `agent/src/`, `workspace.rs`, `zed/src/main.rs`, `title_bar/`, `feature_flags/`, `agent_servers/` all upstream-untouched).
3. **`1e017d04b9` Remove dead link in agent menu — Fix 1b position re-verification** — confirm Fix 1b first-statement at `BaseView::Uninitialized` survived.
4. **`f39cf25c0b` Hide agent servers from chips — Helix bypass-marker survival** — confirm three `// HELIX: External agent …` markers present (lines may have shifted from current 226/248/1518).
5. **PR #60 retry-loop survival check** — confirm `ede_diagnostic` retry block intact (no upstream churn, but the standard discipline).
6. **Cargo.toml / Cargo.lock notes** — record the `objc2`/`objc2-app-kit` bumps and the new `[patch.crates-io] async-process` entry (informational; predicted to land cleanly).

If any of items 3–5 actually conflicted (rather than auto-merging), upgrade the subsection to a "Conflicts and Resolutions" entry with the chosen resolution and rationale.

Always extend the commit-history table at the bottom with this merge's commits and any follow-up fix commits.

**Rebase-checklist additions** — only if confirmed in this merge. Predict: **none**. Do not invent entries.

## Helix Repo Bump

After the Zed branch is pushed:
1. In `/home/retro/work/helix/`, update `sandbox-versions.txt` `ZED_COMMIT=` from `f82e1c676099470ecd17590878a00bd25b342f82` to the new merge HEAD.
2. Commit on a `feature/002100-merge-latest-zed` branch in helix.
3. Push the helix branch.
4. The Helix UI handles PR creation per task convention — do not open PRs from the agent.

## Open Questions (for the implementation agent to answer at runtime)

- Did any fork-side commit land between planning (2026-06-15) and the merge? (No commits since 2026-06-12 as of planning; the window is small but check `git log f82e1c6760..origin/main` at start of work.)
- Did `f39cf25c0b` shift the line numbers of the `// HELIX: External agent …` bypass markers in `extensions_ui.rs`? (Expected yes — three markers may move slightly; verify all three still present and semantically correct.)
- Did `Cargo.toml`'s `[patch.crates-io]` block conflict? (Helix has none currently; predict clean addition.)
- Did upstream advance further between `git fetch upstream` at planning and at execution? (Likely yes — 21 → maybe 25-ish commits; re-stat at execution time.)
- Any unexpected `BaseView` / `ContextServerStatus` variant additions? (Very unlikely — no `acp_thread/`/`agent_panel.rs` core-state churn upstream this window.)

## Notes

### Out-of-band fork pushes
Every prior merge in this series (001909 / 001980 / 001996 / 002029 / 002029-extension / 002077) absorbed at least one out-of-band fix pushed to fork main while the merge branch was open. The 3-day window since 002077 has shown **zero** such pushes — fork main has been quiet. Treat as expected but unusual; re-fetch `origin/main` before declaring done.

### `stack` is the canonical builder
No local Rust toolchain in this environment — all builds go via `cd /home/retro/work/helix && ./stack build-zed dev` (Docker-based, persistent cache). `cargo check` / `cargo test` items in the validation list are **best-effort**; the E2E gate is the hard contractual requirement.

### E2E phase count is 17
Phases 1–14 from 001996; 15–17 added by PR #55/#56/#57. Phase 9 is the explicit regression gate for PR #60's `ede_diagnostic` retry loop; Phase 17 is the explicit gate for PR #56 Fix 1b draft suppression.

### Branch naming
Use `feature/002100-merge-latest-zed`. Do not reuse any earlier branch name.

## Out of Scope

- Changes to Helix API, frontend, or other non-Zed components unless forced by a breaking upstream change.
- New feature development beyond what conflict resolution requires.
- Rewriting or refactoring Helix-specific crates.
- Wiring upstream's `/compact` / `auto_compact` / `agent_skills` into Helix workflows.
- Implementing typed-error semantics — `Workspace::show_error` had no churn this window.
