# Design: Merge Latest Zed Upstream Into Helix Fork (002223)

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` — remote `origin` is the fork
  (`http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`). This in-cluster URL **is** the fork.
- **Upstream**: `zed-industries/zed`. The `upstream` remote is **not** configured in fresh
  clones — add it: `git remote add upstream https://github.com/zed-industries/zed.git`.
- **Porting guide** (canonical living doc): `/home/retro/work/zed/portingguide.md` — 1109 lines
  at task start; newest entry `## Merge 002100-extension (2026-06-18)` at line 670; "Rebase
  Checklist" at line 488.
- **Helix platform repo**: `/home/retro/work/helix/` — `sandbox-versions.txt` carries
  `ZED_COMMIT=9546054e68e2b771ac63e55821a70654684ac651` (exactly at fork HEAD).

## Current State (as of 2026-07-06)

| | Commit | Notes |
|---|---|---|
| Fork HEAD (`origin/main` == local `main`) | `9546054e68` (PR #65) | 2026-06-19 |
| Last upstream merge fence | `e45e42af6e` ("agent_ui: Use the thread title…") | absorbed in 002100-extension |
| Upstream HEAD | **UNKNOWN** — measure at runtime | — |
| Helix-only commits since fence | PR #65 only (the 002153 delta was never landed) | — |
| `upstream` remote | **not configured** | add at start |

**Precedent (size → conflict count):**
- 002029: 261 commits / 10 days → 7 manual conflicts, 3 build-fix commits
- 002077: 256 commits / 10 days → 6 trivial conflicts, 0 signature repairs
- 002100 (two rounds): 120 commits / 6 days → 2 trivial conflicts, 0 repairs
- **002223 outlook**: ~18-day window → likely the largest since 002029. Predict **2–8
  conflicts**; keep a signature-drift repair budget in mind. Consider splitting into
  rounds if the count is large (as 002100 did).

## Merge Strategy

Use `git merge upstream/main` (every prior merge used merge, not rebase — it preserves
Helix commit history and keeps conflict resolution traceable). Do **not** rebase.

```bash
cd /home/retro/work/zed
git remote add upstream https://github.com/zed-industries/zed.git   # only if missing
git fetch upstream && git fetch origin
git log --oneline e45e42af6e..upstream/main | wc -l                 # measure delta
git log --oneline upstream/main -1                                  # record upstream HEAD SHA
git checkout main && git pull origin main                           # confirm still 9546054e68
git checkout -b feature/002223-merge-latest-zed
git merge upstream/main
# Resolve conflicts one at a time, updating portingguide.md as each is resolved.
# Build → critical-fix grep → unit tests → E2E (hard gate).
git push -u origin feature/002223-merge-latest-zed
```

### Conflict-resolution principles (carried from all prior merges)

1. **Prefer upstream ordering** for shared lists / match arms (models, features).
2. **Keep Helix code behind `#[cfg(feature = "external_websocket_sync")]`** — never leak into unconditional paths.
3. **`Cargo.lock`**: always `git checkout --theirs Cargo.lock` (regenerated on next build).
4. **`.github/workflows/`**: always `--theirs` — Helix doesn't use Zed's CI.
5. **Upstream renames**: even for auto-merged files, grep for old identifiers in cfg-gated regions.
6. **Trait-signature changes**: resolve compile-driven; the `./stack build-zed dev` build is the safety net.
7. **Document every non-trivial resolution in `portingguide.md` immediately** — even a conflict-free merge needs a dated entry (this is exactly what 002153 failed to do).
8. **If a conflict is too risky**: stop, document in `portingguide.md`, escalate.

## Likely Conflict Hot-spots

Inspect all Helix-touched files after the merge even if git reports "auto-merged" —
larger window = higher silent-drift risk than 002153 anticipated.

| File | Helix concern | Risk |
|------|---------------|------|
| `crates/acp_thread/src/connection.rs` | PR #65 `fail_turn()` on `StubAgentConnection` (upstream file) | LOW-MED — keep both sides if upstream also touched `StubAgentConnection`/`AgentConnection` |
| `crates/acp_thread/src/acp_thread.rs` | Fixes #3/#6/#8/#9 + PR #55 emit | MED — re-grep `content_only`, `stopped_emitted_for_task`, `drop(turn.send_task)`, `EntryUpdated` |
| `crates/agent_ui/src/agent_panel.rs` | Fix 1b (FIRST stmt of `BaseView::Uninitialized`) + Fix #11 | MED — read full `ensure_thread_initialized` after merge; line will shift |
| `crates/agent/src/agent.rs` | Fix #1 `pending_sessions`, `wait_for_tools_ready` | LOW |
| `crates/agent_ui/src/conversation_view.rs` | `from_existing_thread()` field-set | LOW — build is the gate |
| `crates/agent_servers/src/acp.rs` | PR #50 `session_creation_chain` + `_settings_subscription` | LOW |
| `crates/extensions_ui/src/extensions_ui.rs` | 3× `// HELIX: External agent` markers | LOW — verify by grep |
| `crates/zed/src/main.rs` | `--allow-multiple-instances`, `--headless`, `build_application` | LOW |
| `crates/settings_content/src/settings_content.rs` | Helix settings fields | LOW-MED — "both-sides-added-a-field" is the recurring pattern |
| `crates/feature_flags/src/flags.rs` | `AcpBetaFeatureFlag::enabled_for_all` | LOW |
| `crates/title_bar/` | `optional = true` dep + `render_restricted_mode` early return | LOW |
| `Cargo.toml` | `rust-embed` `debug-embed`, workspace members | LOW |
| `Cargo.lock` | — | TRIVIAL (`--theirs`) |
| `crates/external_websocket_sync/*` (Helix-only) | PR #60/#63/#64/#65 surface | NONE from upstream — verify by construction |

## Resolution Patterns Worth Re-stating

- **`HeadlessConnection` is dead code** (since 001909) — ignore stale guide references.
- **`AcpThreadEvent::Stopped(StopReason)`** is a tuple variant — matches need `Stopped(_)`; test code included.
- **`BaseView::*` / `ContextServerStatus::*` arms** must stay exhaustive (Helix UI state queries + headless responder). `ContextServerStatus::ClientSecretRequired { .. }` was added in 002029.
- **`title_bar`'s `external_websocket_sync` dep MUST be `optional = true`** with the matching `[features]` entry.
- **Auto-merged ≠ correct** — always grep for renamed identifiers after the merge.
- **GPUI events flush at end of the entity update closure** (lesson from 001996).
- **PR #60 retry loop** — 4 attempts × 750ms on `ede_diagnostic`; Phase 9 is the regression gate.
- **PR #65 `TEST_WEBSOCKET_SERVICE_GUARD`** — crash-regression + reconnect tests share this mutex; `cargo test -p external_websocket_sync` must not deadlock.
- **Never use `--no-build`** when investigating E2E failures (learned 002100-extension).
- **`go mod tidy` in `e2e-test/helix-ws-test-server/`** before E2E — the runner doesn't tidy itself.

## Post-Merge Validation

### 1. Compile (the hard build gate)
```bash
cd /home/retro/work/helix && ./stack build-zed dev        # Docker-based → ./zed-build/zed
# best-effort if a local Rust toolchain exists:
# cargo check -p zed --features external_websocket_sync
```
No local Rust toolchain is assumed — `./stack build-zed dev` is canonical (warm ~46s–1m31s;
cold can exceed 16 min).

### 2. Grep verification of critical fixes / silent drift
```bash
cd /home/retro/work/zed
# PR #65
grep -n "fail_turn" crates/acp_thread/src/connection.rs
grep -n "ChatResponseError\|chat_response_error" crates/external_websocket_sync/src/types.rs crates/external_websocket_sync/src/thread_service.rs
grep -n "TEST_WEBSOCKET_SERVICE_GUARD" crates/external_websocket_sync/src/thread_service.rs
# Critical fixes
grep -n "load_session\|pending_sessions" crates/agent/src/agent.rs
grep -n "content_only\|drop(turn.send_task)\|stopped_emitted_for_task\|EntryUpdated" crates/acp_thread/src/acp_thread.rs
grep -n "ThreadMetadataStore\|load_agent_thread\|ensure_thread_initialized" crates/agent_ui/src/agent_panel.rs
grep -rn "unregister_thread" crates/agent_ui/src/conversation_view.rs
# Helix PRs
grep -n "session_creation_chain\|_settings_subscription" crates/agent_servers/src/acp.rs
grep -n "ede_diagnostic\|force_reset_session\|clear_keep_alive\|agent_ready" crates/external_websocket_sync/src/thread_service.rs
grep -n "helix-org" crates/external_websocket_sync/e2e-test/Dockerfile.ci
grep -n "HELIX: External agent" crates/extensions_ui/src/extensions_ui.rs   # expect 3
# Carry-overs / drift
grep -n "allow_multiple_instances\|headless\|build_application" crates/zed/src/main.rs
grep -n "debug-embed\|external_websocket_sync\|cloud_api_types" Cargo.toml
grep -n "smol::Timer" crates/agent/src/agent.rs                              # expect 0
grep -nE "AcpThreadEvent::Stopped\b([^(]|$)" -r crates/acp_thread/src/       # expect 0
grep -n "AcpBetaFeatureFlag\|enabled_for_all" crates/feature_flags/src/flags.rs
grep -n "render_restricted_mode" crates/title_bar/src/title_bar.rs
grep -n "CollaboratorId::Agent" crates/workspace/src/workspace.rs
```

### 3. Walk the rebase checklist
Step through every numbered item in `portingguide.md` §"Rebase Checklist" (line 488).
Special attention: Fix 1b first-statement invariant (`agent_panel.rs`), `ConversationView`
field-set drift (build-gated), `acp_thread.rs` cancel/Stopped invariant, and PR #65
`fail_turn` after any upstream `AgentConnection` change.

### 4. E2E test — canonical hard gate (external WebSocket sync crate)
```bash
cd /home/retro/work/helix && ./stack build-zed dev
cp ./zed-build/zed /home/retro/work/zed/crates/external_websocket_sync/e2e-test/zed-binary
cd /home/retro/work/zed/crates/external_websocket_sync/e2e-test
(cd helix-ws-test-server && go mod tidy)
./run_docker_e2e.sh                                  # zed-agent only
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh    # both agents — always full rebuild
```
All **17 phases** must pass for **both** `zed-agent` and `claude`. Gates: Phase 8 (Stopped
invariant), Phase 9 (PR #60 retry loop), Phase 13 (`cancel_current_turn`), Phase 15 (PR #55),
Phase 16 (PR #56 Fix 1a + PR #57), Phase 17 (PR #56 Fix 1b). If any phase fails: do **not**
mark complete or bump `ZED_COMMIT` — diagnose, fix, document in `portingguide.md`, re-run.
One retry allowed for Claude Phase 1 npm flake and Phase 9 API-latency flake.

## Documentation Updates (incremental)

After each non-trivial conflict, append to `portingguide.md`. Add `## Merge 002223
(2026-07-06)` at the top of the merge-history list, mirroring 002100. **Minimum
subsections regardless of auto-resolution:**

1. **Window summary** — "N upstream commits over ~18 days; base `e45e42af6e` → HEAD `<sha>`" (fill at runtime); note that the 002153 delta was folded into this merge.
2. **PR #65 survival check** — `fail_turn`, Error arm, `SyncEvent::ChatResponseError`, `TEST_WEBSOCKET_SERVICE_GUARD`.
3. **Helix-surface auto-merge survival check** — Fix 1b position, 3× `// HELIX:` markers, `external_websocket_sync/` untouched, all critical fixes.
4. **PR #60/#63/#64 survival check**.
5. **Cargo.toml / Cargo.lock notes** — new deps / version bumps.

If any area actually conflicted, upgrade its subsection to a "Conflicts and Resolutions"
entry with chosen resolution + rationale. Always extend the commit-history table.

## Helix Repo Bump

After the Zed branch is pushed:
1. In `/home/retro/work/helix/`, update `sandbox-versions.txt`
   `ZED_COMMIT=9546054e68e2b771ac63e55821a70654684ac651` → new merge HEAD.
2. Commit on `feature/002223-merge-latest-zed` in helix; push.
3. Helix UI opens the PR — the agent does not.

## Notes

- **`stack` is the canonical builder** — no local Rust toolchain assumed; `cargo` steps are best-effort, the E2E gate is contractual.
- **E2E phase count is 17** (1–14 from 001996; 15–17 from PR #55/#56/#57). PR #65 added a unit test, not an E2E phase. Phase 9 gates PR #60; Phase 17 gates PR #56 Fix 1b.
- **Out-of-band fork pushes**: every prior merge picked up at least one push to fork main while the branch was open — re-fetch `origin/main` before declaring done and merge if needed.
- **Branch naming**: `feature/002223-merge-latest-zed`; do not reuse an earlier name.
- **Read 002153's docs first** — they are the still-unexecuted playbook for exactly this baseline; also skim 002100/002077/002029 for conflict-pattern context.
