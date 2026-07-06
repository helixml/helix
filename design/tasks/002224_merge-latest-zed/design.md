# Design: Merge Latest Zed Upstream Into Helix Fork (002224)

## Repository Layout

- **Helix fork of Zed (working repo)**: per the task constraints, `/prod/home/luke/pm/zed-upstream`
  on branch `helix-fork`; `helix` remote = `git@github.com:helixml/zed.git`; `upstream` remote
  (read-only) = `https://github.com/zed-industries/zed.git`. In the in-cluster spec system the
  equivalent is the reference clone at `/home/retro/work/zed` whose `origin` is the gitea mirror.
  **Confirm which layout applies before pushing.**
- **Porting guide (canonical, living)**: `/home/retro/work/zed/portingguide.md` — the merge
  history is at the top; in the reference clone the latest entry is `## Merge 002100-extension
  (2026-06-18)` and the "Rebase Checklist" has **44** numbered items.
- **Helix platform repo**: `sandbox-versions.txt` carries `ZED_COMMIT=` — must be bumped to the
  new merge HEAD after the Zed branch is pushed.

## Current State — Measure at Execution Time

The reference clone shows fork HEAD `9546054e68`, fence `e45e42af6e`, guide→002100-extension,
ACP `0.14.0`/schema `0.13.6`, 17 E2E phases. **This snapshot is likely stale** (see
requirements Open Questions). Before merging, in the real working repo:

```bash
cd <working-repo>
git remote get-url upstream || git remote add upstream https://github.com/zed-industries/zed.git
git fetch upstream && git fetch helix   # or origin, per layout
git checkout helix-fork && git pull     # confirm HEAD
FENCE=$(git log -1 --format=%H helix-fork)      # or the last upstream SHA per portingguide top entry
git log --oneline helix-fork..upstream/main | wc -l   # commit delta
git log --oneline -1 upstream/main                    # upstream HEAD SHA to record
grep -A2 'name = "agent-client-protocol"' Cargo.lock  # ACP version — check for bump
```

Recent merge precedent (size → conflict count), for calibration:

| Merge | Commits / window | Manual conflicts | Notes |
|---|---|---|---|
| 002029 | 261 / 10 days | 7 | 3 build-fix commits |
| 002077 | 256 / 10 days | 6 trivial | 0 signature-drift repairs |
| 002100 rd1 | 25 / 3 days | 1 trivial | `RemoteSettingsContent` both-sides field |
| 002100 rd2 | 95 / 3 days | 1 trivial | `grep_tool.rs` semantic reuse |

Expected profile for 002224 depends entirely on the measured window. A ~2-week window with
prior cadence could be 150–400+ commits and 3–8 conflicts. Scale reconnaissance accordingly.

## Merge Strategy

Use **`git merge upstream/main`** (every prior merge did this — it preserves Helix commit
history and keeps conflict resolution traceable). Do **not** rebase `helix-fork` onto upstream.

```bash
cd <working-repo>
git checkout -b feature/002224-merge-latest-zed helix-fork
# START the portingguide entry NOW (before resolving), then:
git merge upstream/main
# Resolve conflicts one at a time; update portingguide.md as each is resolved.
# Build (both feature on/off) → grep critical fixes → unit tests → E2E (17 phases, both agents).
git push -u helix feature/002224-merge-latest-zed
# Write pull_request_zed.md + pull_request_helix.md; bump ZED_COMMIT in helix; push helix branch.
```

### Conflict-resolution principles (carried from all prior merges)

1. **Prefer upstream ordering** for shared additive lists — model lists
   (`anthropic/src/anthropic.rs`), feature lists, match arms. Take upstream's ordering; graft
   Helix items back if any were Helix-only.
2. **Keep Helix code behind `#[cfg(feature = "external_websocket_sync")]`** — never let it leak
   into unconditional paths. Built-in agent hiding uses `cfg(not(feature = "..."))`.
3. **`Cargo.lock`**: accept upstream (`git checkout --theirs Cargo.lock`); it regenerates on build.
4. **`.github/workflows/` and Zed CI**: accept upstream — Helix doesn't use Zed's CI. But do
   **not** disturb `.drone.yml`'s `cargo build --locked` (Helix CI needs it; source is read-only there).
5. **Upstream renames**: after every merge — even for cleanly auto-merged files — grep for old
   identifiers in cfg-gated regions (auto-merged ≠ correct).
6. **Trait-signature / `non_exhaustive` changes**: walk all impls compile-driven; if ACP bumped,
   convert struct literals to builders and re-check `ErrorCode` arms. The build is the safety net.
7. **Document every non-trivial resolution in `portingguide.md` immediately.** Even a
   conflict-free merge gets a dated entry.
8. **If a conflict is too risky** (e.g. upstream's new session/resume UI collides with
   `from_existing_thread`): stop, document, escalate — don't silently rework the pattern.

## Likely Conflict Hot-spots (from prior merges + task constraints)

Inspect every Helix-touched file after the merge even if git reports "auto-merged".

| File | Helix concern | Risk |
|------|---------------|------|
| `crates/agent_ui/src/agent_panel.rs` | WebSocket thread display, `from_existing_thread`, ThreadDisplayNotification handler (OnboardingUpsell dismiss + NativeAgentSessionList), Fix 1b FIRST-statement invariant, Critical Fix #11 | **HIGH** |
| `crates/zed/src/zed.rs` | `initialize_agent_panel` + WebSocket init inside it | **HIGH** |
| `crates/agent_servers/src/acp.rs` | ACP builder API evolves frequently; PR #50 `session_creation_chain` + `_settings_subscription` | **HIGH** |
| `crates/agent_ui/src/acp/thread_view.rs` | `from_existing_thread`, HeadlessConnection, channel-based event forwarding; no duplicate WS sends (Fix #2) | **HIGH** |
| `crates/anthropic/src/anthropic.rs` | New models added regularly — take upstream ordering | MED |
| `crates/acp_thread/src/acp_thread.rs` | Fixes #3/#6/#8/#9 + PR #55 emit; re-grep `content_only`, `stopped_emitted_for_task`, `drop(turn.send_task)`, `EntryUpdated` | MED |
| `crates/acp_thread/src/connection.rs` | PR #65 `StubAgentConnection::fail_turn`; ACP trait additions | MED |
| `crates/external_websocket_sync/src/thread_service.rs` | Helix-only (no upstream churn) — PR #60/#63/#64/#65 surface, windowless `cx.subscribe()` | NONE (verify) |
| `crates/agent/src/agent.rs` | Fix #1 `pending_sessions`; `wait_for_tools_ready` timer | LOW |
| `crates/feature_flags/src/flags.rs` | `AcpBetaFeatureFlag::enabled_for_all() -> true` | LOW |
| `crates/zed/src/main.rs` | `--allow-multiple-instances`, `--headless`, `initialize_headless`, `build_application(headless)` | LOW |
| `assets/settings/default.json` | `trust_all_worktrees`, `show_sign_in:false`, branding/onboarding | LOW |
| `crates/settings_content/src/settings_content.rs` | Helix fields (both-sides-added-a-field historical pattern) | LOW |
| `crates/title_bar/`, `crates/extensions_ui/src/extensions_ui.rs` | `render_restricted_mode`; 3× `// HELIX:` markers; `optional = true` dep | LOW |
| `Cargo.toml` | `rust-embed debug-embed`; `external_websocket_sync` + `cloud_api_types` members | LOW |
| `.drone.yml` | `cargo build --locked` must remain | LOW (verify) |

## Resolution Patterns Worth Re-stating

- **`HeadlessConnection` is dead code** (since 001909) — don't be misled by old guide entries.
- **GPUI subscription gotcha**: `subscribe_in` is silently dropped with no window context. Use
  the channel-based pattern for UI updates; use `App::subscribe` (windowless) for WebSocket
  forwarding; keep windowless `cx.subscribe()` in `thread_service.rs` for incremental streaming.
- **`AcpThreadEvent::Stopped(StopReason)`** is a tuple variant — matches need `Stopped(_)`;
  test code is not exempt (breaks `cargo test -p acp_thread test_second_send`).
- **`BaseView::*` and `ContextServerStatus::*` arms** must stay exhaustive (Helix UI state
  queries + headless responder).
- **`title_bar`'s `external_websocket_sync` dep MUST be `optional = true`** with the matching
  `[features]` entry.
- **`from_existing_thread()` field set** is build-gated — a green build confirms no drift; zero
  drift in the last several merges.
- **`go mod tidy` in `e2e-test/helix-ws-test-server/`** before E2E — the runner doesn't tidy.
- **Never `--no-build` when investigating E2E failures** (lesson from 002100-extension).
- **Out-of-band pushes**: every prior merge picked up at least one push to fork main while the
  branch was open. Re-fetch before declaring done and merge if needed.

## Post-Merge Validation

### 1. Compile (both gates)
```bash
cargo check -p zed                                    # no features (if local rust)
cargo check -p zed --features external_websocket_sync # with Helix gate
# canonical builder when no local toolchain:
cd /home/retro/work/helix && ./stack build-zed dev    # Docker-based → ./zed-build/zed
```

### 2. Grep verification of critical fixes / silent drift
```bash
cd <working-repo>
# PR #65
grep -n "fail_turn" crates/acp_thread/src/connection.rs
grep -n "ChatResponseError\|chat_response_error" crates/external_websocket_sync/src/{types.rs,thread_service.rs}
grep -n "TEST_WEBSOCKET_SERVICE_GUARD" crates/external_websocket_sync/src/thread_service.rs
# Critical fixes
grep -n "pending_sessions\|load_session" crates/agent/src/agent.rs
grep -n "content_only\|stopped_emitted_for_task\|drop(turn.send_task)\|EntryUpdated" crates/acp_thread/src/acp_thread.rs
grep -n "ThreadMetadataStore\|load_agent_thread" crates/agent_ui/src/agent_panel.rs
grep -n "ensure_thread_initialized" crates/agent_ui/src/agent_panel.rs   # read full body: Fix 1b FIRST stmt
grep -n "OnboardingUpsell\|NativeAgentSessionList" crates/agent_ui/src/agent_panel.rs
# PR #50 / #55 / #60 / #63 / #64
grep -n "session_creation_chain\|_settings_subscription" crates/agent_servers/src/acp.rs
grep -n "ede_diagnostic\|handle_follow_up_message\|force_reset_session\|clear_keep_alive\|agent_ready" crates/external_websocket_sync/src/thread_service.rs
# Branding / flags / flags-file
grep -n "enabled_for_all" crates/feature_flags/src/flags.rs
grep -n "trust_all_worktrees\|show_sign_in" assets/settings/default.json
grep -n "allow_multiple_instances\|headless\|build_application" crates/zed/src/main.rs
grep -n "debug-embed" Cargo.toml ; grep -n "smol::Timer" crates/agent/src/agent.rs   # last: 0 hits
grep -n "render_restricted_mode" crates/title_bar/src/title_bar.rs
grep -n "HELIX: External agent" crates/extensions_ui/src/extensions_ui.rs             # expect 3
grep -n "helix-org" crates/external_websocket_sync/e2e-test/Dockerfile.ci
grep -n "locked" .drone.yml                                                            # cargo build --locked
# Test-pattern drift
grep -nE "AcpThreadEvent::Stopped\b([^(]|$)" crates/acp_thread/src/                    # should be 0
```

### 3. Walk the "Rebase Checklist" in `portingguide.md`
Step through every numbered item (44 in the reference clone). Special attention: item 9
(agent_panel cfg blocks / Fix 1b first-statement), item 11 (`ConversationView` /
`ConnectedServerState` field set), items 31/31a/37 (cancel / Stopped invariant), 39/39a
(`--allow-multiple-instances` / `--headless`), 40 (`debug-embed`), 41/41a (`smol::Timer` /
tuple-variant `Stopped`), plus PR #65 `fail_turn`.

### 4. Unit tests (where a local toolchain exists)
```bash
cargo test -p external_websocket_sync        # incl. PR #65 crash-regression + reconnect (shared guard)
cargo test -p acp_thread test_second_send    # Fix #6
cargo test -p agent_servers test_concurrent_session_creation_is_serialized  # PR #50
```

### 5. E2E (canonical regression check — HARD GATE)
```bash
cd /home/retro/work/helix && ./stack build-zed dev
cp ./zed-build/zed <working-repo>/crates/external_websocket_sync/e2e-test/zed-binary
cd <working-repo>/crates/external_websocket_sync/e2e-test
(cd helix-ws-test-server && go mod tidy)
./run_docker_e2e.sh                                # zed-agent
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh  # both — always full rebuild
```
The task also references the raw Docker invocation
(`docker build -t zed-ws-e2e -f crates/external_websocket_sync/e2e-test/Dockerfile . && docker run --rm -e ANTHROPIC_API_KEY=... -e TEST_TIMEOUT=120 zed-ws-e2e`);
`run_docker_e2e.sh` is the established wrapper. All phases must pass for both agents (17 in the
reference clone — confirm the count). Gate phases: 9 (PR #60), 15 (PR #55), 16 (PR #56 Fix 1a +
PR #57), 17 (PR #56 Fix 1b). One retry each for the Claude Phase-1 npm flake and Phase-9
API-latency flake. If any phase fails: do not complete — diagnose, fix, document, re-run.

## Documentation Updates (incremental — not retrospective)

Append to `portingguide.md` a new `## Merge 002224 (2026-07-06)` section at the top of the
merge-history list, started when `git merge` is issued. Minimum subsections regardless of
auto-resolution:

1. **Window summary** — actual commit count over the measured window + upstream HEAD SHA + fence.
2. **Conflicts and Resolutions** — per conflict: file/region, upstream change (PR/commit), what
   was kept from each side and why, follow-up risk. Or an explicit "0 conflicts, auto-merge clean".
3. **PR #65 survival check** — `fail_turn`, Error arm, `ChatResponseError`, shared test guard.
4. **Helix-surface auto-merge survival check** — Fix 1b position; 3× `// HELIX:` markers;
   `external_websocket_sync/` untouched; all Critical Fixes intact; branding/flags/settings intact.
5. **PR #60/#63/#64 survival check** — retry loop + wedge recovery + agent_ready re-emit.
6. **Cargo.toml / Cargo.lock notes** — new upstream deps, version bumps, ACP bump if any.
7. **Pre-existing Breakage Repaired** — only if a signature-drift / typed-error / new-variant fix fires.

Always extend the commit-history table at the bottom with this merge's commits + follow-up fixes.

## Helix Repo Bump

After the Zed branch is pushed: bump `sandbox-versions.txt` `ZED_COMMIT=` to the new merge HEAD
on a `feature/002224-merge-latest-zed` branch in the helix repo, push it, and let the Helix UI
create the PR (do not open PRs from the agent).

## Notes

- **`stack` is the canonical builder** in the in-cluster environment (no local Rust toolchain);
  `cargo check`/`cargo test` items are best-effort, the E2E gate is the hard requirement. In the
  maintainer's working repo a normal `cargo`/CI toolchain is available.
- **CI `--locked`**: `.drone.yml`'s `cargo build --locked` must survive the merge — the Zed
  source is mounted read-only in CI, so an unlocked build (which would try to rewrite `Cargo.lock`)
  fails. Verify after resolving `Cargo.lock`.
- **Branch naming**: `feature/002224-merge-latest-zed`. Do not reuse an earlier name.
