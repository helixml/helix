# Merge upstream Zed into fork (001909)

## Summary

Merges upstream `zed-industries/zed` into the Helix fork. Brings in 86 upstream commits accumulated between Apr 22 and Apr 25, 2026 (range `62bd61a679..e3d1876c06`). The smallest merge to date — only 3 days of upstream activity since the previous merge (PR #42).

## Conflicts

Just **one conflict**, in `crates/agent/src/agent.rs`:
- Upstream PR #53603 ("Remove smol as a dependency from a bunch of crates") changed `NativeAgentSessionList` to use `async_channel` instead of `smol::channel`. Helix had marked the constructor `pub fn new`. Took upstream's version (`fn new` + `async_channel::unbounded()`) — the `pub` was unused outside `agent.rs`, and the field types are now `async_channel`.

All other high-risk Helix-touched files (`agent_panel.rs`, `conversation_view.rs`, `acp_thread.rs`, `connection.rs`, `workspace.rs`, the feature-flagged `Cargo.toml`s) auto-merged cleanly.

## Verification

- All 9 critical fixes verified by grep (entity lifetime, no duplicate WS sends, content_only, notify_thread_display, stale-pending flush, Stopped-emission guards, unregister-on-reset, drop-not-await, stopped_emitted_for_task)
- No silent drift from upstream renames (`ActiveView`/`set_active_view`/`draft_threads`/`background_threads` all clean)
- `wait_for_tools_ready` (Helix addition) preserved in `agent.rs:1723`
- Full E2E test (10 phases × 2 agents) — see CI

## Porting Guide

Updated `portingguide.md` with:
- Note that `HeadlessConnection` referenced in older sections only exists in dead code (`crates/agent_ui/src/acp/thread_view.rs` is not in `mod` tree). The current `from_existing_thread()` reuses the existing thread's connection.
- Note that `ConnectedServerState` no longer has a `history` field (porting guide had stale reference).
- Append commit history with this merge.

Release Notes:

- N/A
