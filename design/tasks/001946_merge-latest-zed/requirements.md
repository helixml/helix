# Requirements: Merge Latest Zed Upstream (001946)

## Context

The Helix fork of Zed was last merged with upstream `zed-industries/zed` via **zed PR #43** (task 001909) on **2026-04-25**, integrating upstream commits up to `e3d1876c06`. Today is **2026-04-27** — only **~2 days** of upstream activity has accumulated.

Since that merge, **4 Helix-only PRs (#44, #45, #46, #47)** have landed on fork main (8 commits ahead of the 001909 merge commit):

| Commit | PR | Description |
|---|---|---|
| `f5fab97857` | #47 | Bump context-server request timeout from 60s to 180s |
| `5dea310849` | #46 | Re-route AgentConnectionStore through AgentConnectionCache (redux) |
| `67c87da708` | #45 | external_websocket_sync: refresh turn_request_id on UserMessage NewEntry |
| `3c17c13a11` | #44 | Add trailing-edge flush timer for streaming throttle |

Current fork main HEAD: `f5fab9785759267784fa8d501d38138d203bb55a`.
`sandbox-versions.txt` `ZED_COMMIT` matches HEAD (clean state — no uncommitted bumps).

This will be **zed PR #48** in the fork. Risk profile is comparable to (or smaller than) 001909 — but note that PRs #45 and #46 touched the **external_websocket_sync crate** and **AgentConnection wiring**, which raises the chance of a conflict if upstream also moved in those areas.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to merge the latest upstream Zed into the Helix fork so that we stay current and minimize merge debt accumulation.

### 2. Helix User
> As a Helix user, I want any new upstream Zed editor improvements without losing the WebSocket sync integration that connects Zed to the Helix platform.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want the porting guide kept current with any new patterns, renames, or trait changes discovered during this merge.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD
- [ ] All Helix-specific commits (including the 4 new PRs #44–#47) are preserved and functional
- [ ] No upstream commits are skipped or cherry-picked out

### Critical Fix Preservation (9 fixes in `portingguide.md` §"Critical Fixes")
- [ ] Fix #1: `NativeAgent` entity lifetime in `load_session()` — entity cloned before async task
- [ ] Fix #2: No duplicate WebSocket sends from `thread_view.rs`
- [ ] Fix #3: `content_only()` strips "## Assistant" heading
- [ ] Fix #4: `notify_thread_display()` called for follow-ups to non-visible threads
- [ ] Fix #5: Stale pending entries flushed when different entry starts streaming
- [ ] Fix #6: Every `send()` emits exactly one `Stopped` event
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on normal completion path

### Recently Added Helix Behaviour (since 001909 — must survive merge)
- [ ] PR #44 trailing-edge flush timer for streaming throttle still in place
- [ ] PR #45 `turn_request_id` refresh on UserMessage `NewEntry` still applied
- [ ] PR #46 `AgentConnectionStore` → `AgentConnectionCache` wiring still intact
- [ ] PR #47 context-server request timeout still 180s (not reverted to 60s upstream default)

### Carry-overs from 001909 that must NOT regress
- [ ] `--allow-multiple-instances` CLI flag still present in `crates/zed/src/main.rs`
- [ ] `rust-embed` workspace dep still has `debug-embed` feature enabled
- [ ] `wait_for_tools_ready` uses `cx.background_executor().timer()` (not `smol::Timer`)

### Build & Test
- [ ] `./stack build-zed dev` succeeds and produces a fresh `./zed-build/zed` binary
- [ ] `cargo test -p external_websocket_sync` passes (unit tests) — or deferred to E2E if no local toolchain
- [ ] **E2E Docker test passes for `zed-agent` (all 12 phases)** — canonical regression check
- [ ] **E2E Docker test passes for `claude` agent (all 12 phases)** — canonical regression check
- [ ] Phase 8 (mid-stream interrupt) and Phase 9 (rapid 3-turn cancel) both pass for both agents

### Documentation
- [ ] `portingguide.md` updated with any new conflict patterns, renames, or API changes encountered during this merge
- [ ] Rebase checklist in `portingguide.md` extended with any new items
- [ ] Commit history table in `portingguide.md` extended with the new merge commit and any post-merge fixes
- [ ] Updates made **incrementally** during the merge, not retrospectively

### Process
- [ ] Feature branch `feature/001946-merge-latest-zed` created from fork main (`f5fab97857`)
- [ ] PR opened against fork main (zed PR #48 expected)
- [ ] Helix repo PR opened to bump `ZED_COMMIT` in `sandbox-versions.txt`
- [ ] Helix PR opened **first** per `CLAUDE.md` ordering rule
- [ ] **Do NOT merge if external_websocket_sync E2E tests are failing** (hard gate)
