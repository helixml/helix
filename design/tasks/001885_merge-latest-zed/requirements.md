# Requirements: Merge Latest Zed Upstream

## Context

The Helix fork of Zed was last merged with upstream zed-industries/zed on **March 23, 2026** (fork commit `1f96cb8812`, upstream commit `8b822f9e10` — "Fix regression preventing new predictions from being previewed in subtle mode #51887"). Since then:

- **~920 upstream commits** have accumulated (March 23 – April 22, 2026)
- **89 Helix-specific commits** exist on the fork (PRs #29–#41), including critical fixes for FIFO ordering, cancel-drop semantics, and split-brain detection
- This is the **largest upstream merge to date** — previous largest was 506 commits (task 001723)

## User Stories

### 1. Platform Engineer (performing the merge)
> As a platform engineer, I want to merge the latest upstream Zed into the Helix fork so that we stay current with upstream improvements and minimize future merge debt.

### 2. Helix User
> As a Helix user, I want the latest Zed editor features (code lens support, top-down thread generation, parallel agents, worktree picker improvements) without losing the WebSocket sync integration that connects Zed to the Helix platform.

### 3. Future Merge Engineer
> As an engineer performing the next upstream merge, I want an updated porting guide documenting all new conflict patterns, renames, and API changes discovered during this merge.

## Acceptance Criteria

### Merge Completeness
- [ ] Fork main includes all upstream commits through current upstream HEAD (~920 commits)
- [ ] All 89 Helix-specific commits are preserved and functional
- [ ] No upstream commits are skipped or cherry-picked out

### Critical Fix Preservation (9 fixes)
- [ ] Fix #1: `NativeAgent` entity lifetime in `load_session()` — entity cloned before async task
- [ ] Fix #2: No duplicate WebSocket sends from `thread_view.rs`
- [ ] Fix #3: `content_only()` strips "## Assistant" heading
- [ ] Fix #4: `notify_thread_display()` called for non-visible thread follow-ups
- [ ] Fix #5: Stale pending entries flushed when different entry starts streaming
- [ ] Fix #6: Every `send()` emits exactly one `Stopped` event (test: `cargo test -p acp_thread test_second_send`)
- [ ] Fix #7: `THREAD_REGISTRY` unregistration on entity replacement
- [ ] Fix #8: `cancel()` drops `send_task` instead of awaiting it
- [ ] Fix #9: `stopped_emitted_for_task` guard on normal completion path

### API Compatibility
- [ ] `HeadlessConnection` updated for new `AgentConnection::prompt` signature (`UserMessageId` now required, not `Option`)
- [ ] `from_existing_thread()` updated for any new `ConnectedServerState` fields
- [ ] Session ref-counting changes in `NativeAgent` integrated with Critical Fix #1
- [ ] All `HeadlessConnection` methods track current `AgentConnection` trait

### Build & Test
- [ ] `cargo check --package zed --features external_websocket_sync` compiles cleanly
- [ ] `cargo test -p external_websocket_sync` passes (unit tests)
- [ ] `cargo test -p acp_thread test_second_send` passes (Stopped invariant)
- [ ] E2E Docker test passes for `zed-agent` (all 10 phases)
- [ ] E2E Docker test passes for `claude` agent (all 10 phases)

### Documentation
- [ ] `portingguide.md` updated with all new conflict patterns, renames, and API changes
- [ ] Rebase checklist in porting guide updated with any new items
- [ ] `sandbox-versions.txt` in helix repo updated with new ZED_COMMIT hash

### Process
- [ ] PR opened against fork main with merge commit
- [ ] Zed binary rebuilt and E2E test re-run with fresh binary
