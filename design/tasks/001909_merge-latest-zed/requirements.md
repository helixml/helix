# Requirements: Merge Latest Zed Upstream

## Context

The Helix fork of Zed was last merged with upstream `zed-industries/zed` via PR #42 (merge commit `980a6f1dbc`, integrating upstream `62bd61a679` "Update docs for creating multi-root projects #54655") on **April 22, 2026** as part of task 001864. Today is **April 25, 2026** — only 3 days of upstream activity has accumulated.

Current fork main HEAD: `9f0475c6c2` ("fix: drop stale display_name reference in [ACP_SPAWN] log") — exactly **1 Helix commit** ahead of the previous merge.

This is the **smallest upstream merge to date**. Risk profile is correspondingly low, but the same care must be taken to:
- Preserve all 9 Critical Fixes documented in `portingguide.md`
- Update `portingguide.md` with any new conflict patterns / API changes encountered
- Verify the external_websocket_sync E2E test still passes (this is the canonical regression check)

The previous merge (001864) hit **35 conflicted files** and required 4 follow-up fix commits (`badd75c350`, `466f9e57a6`, `5000ca904d`, `ba7e97aea6`, `350de991de`) plus a stale-reference fix (`9f0475c6c2`). The same areas remain fragile.

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to merge the latest upstream Zed into the Helix fork so that we stay current with upstream improvements and minimize future merge debt.

### 2. Helix User
> As a Helix user, I want any new upstream Zed editor improvements without losing the WebSocket sync integration that connects Zed to the Helix platform.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want the porting guide kept current with any new patterns, renames, or trait changes discovered during this merge.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD
- [ ] All Helix-specific commits are preserved and functional
- [ ] No upstream commits are skipped or cherry-picked out

### Critical Fix Preservation (9 fixes in `portingguide.md` §"Critical Fixes")
- [ ] Fix #1: `NativeAgent` entity lifetime in `load_session()` — entity cloned before async task
- [ ] Fix #2: No duplicate WebSocket sends from `thread_view.rs` (only `UserCreatedThread` + `ThreadTitleChanged`)
- [ ] Fix #3: `content_only()` strips "## Assistant" heading
- [ ] Fix #4: `notify_thread_display()` called for follow-ups to non-visible threads
- [ ] Fix #5: Stale pending entries flushed when different entry starts streaming
- [ ] Fix #6: Every `send()` emits exactly one `Stopped` event (`cargo test -p acp_thread test_second_send`)
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on normal completion path

### Build & Test
- [ ] `cargo check --package zed --features external_websocket_sync` compiles cleanly
- [ ] `cargo test -p external_websocket_sync` passes (unit tests)
- [ ] `cargo test -p acp_thread test_second_send` passes (Stopped invariant)
- [ ] Fresh Zed binary built via `./stack build-zed dev` (or `release`)
- [ ] E2E Docker test passes for `zed-agent` (all 10 phases)
- [ ] E2E Docker test passes for `claude` agent (all 10 phases)

### Documentation
- [ ] `portingguide.md` updated with any new conflict patterns, renames, or API changes encountered
- [ ] Rebase checklist in `portingguide.md` updated with any new items
- [ ] Commit history table in `portingguide.md` extended with the new merge commit and any post-merge fixes
- [ ] `sandbox-versions.txt` in helix repo updated with new `ZED_COMMIT=` hash

### Process
- [ ] Feature branch `feature/001909-merge-latest-zed` created from fork main
- [ ] PR opened against fork main with merge commit
- [ ] Helix repo PR opened to bump `ZED_COMMIT` in `sandbox-versions.txt`
