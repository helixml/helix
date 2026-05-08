# Merge upstream Zed into fork (001909)

## Summary

Merges upstream `zed-industries/zed` into the Helix fork. Brings in 86 upstream commits accumulated between Apr 22 and Apr 25, 2026 (range `62bd61a679..e3d1876c06`). The smallest upstream merge to date — only 3 days of activity since the previous merge (PR #42).

Also incorporates an out-of-band fix pushed to fork main during the merge work (`d7be64fad1`: "stop empty message_completed loop after Zed restart + Helix-mode UI cleanup").

## Conflicts

Only **one merge conflict** during `git merge upstream/main`, in `crates/agent/src/agent.rs`:
- Upstream PR #53603 ("Remove smol as a dependency from a bunch of crates") changed `NativeAgentSessionList` to use `async_channel` instead of `smol::channel`. Helix had marked the constructor `pub fn new`. Took upstream's version (`fn new` + `async_channel::unbounded()`) — the `pub` was unused outside `agent.rs`, and the field types in the auto-merged region were already updated to `async_channel`.

A second trivial conflict on `Cargo.lock` when re-merging fork main — took theirs (regenerated on next build).

All high-risk Helix-touched files (`agent_panel.rs`, `conversation_view.rs`, `acp_thread.rs`, `connection.rs`, `workspace.rs`, the feature-flagged `Cargo.toml`s) auto-merged cleanly.

## Post-merge fixes

Three carry-over fixes were needed to get the build + E2E working:

1. **`wait_for_tools_ready` smol → `cx.background_executor().timer()`** (`6ccf3010a6`)
   Upstream PR #53603 dropped the `smol` workspace dep from the agent crate. Helix's `wait_for_tools_ready()` used `smol::Timer::after`, breaking the build. Switched to the canonical GPUI pattern used elsewhere in the agent crate.

2. **Restored `--allow-multiple-instances` CLI flag** (`16f2b82053`)
   This Helix-specific flag (originally added in `4cae6d90f7`) was silently lost during the 001864 merge. Without it the e2e-test runner can't launch Zed (`error: unexpected argument '--allow-multiple-instances'`).

3. **Restored `debug-embed` feature on `rust-embed` workspace dep** (`c7a26c9144`)
   Originally added by `9ca797706f` (Oct 2025), lost in a subsequent merge. Without it, dev builds panic on startup with `settings/default.json` because RustEmbed tries to read assets from the source tree at runtime — which doesn't exist inside the e2e container or anywhere outside the build directory.

## Verification

- All 9 critical fixes verified by grep (entity lifetime, no duplicate WS sends, content_only, notify_thread_display, stale-pending flush, Stopped-emission guards, unregister-on-reset, drop-not-await, stopped_emitted_for_task)
- No silent drift from upstream renames (`ActiveView`/`set_active_view`/`draft_threads`/`background_threads` all clean)
- `wait_for_tools_ready` (Helix addition) preserved
- **Full E2E test PASSED for both `zed-agent` and `claude` rounds — all 12 phases**

## Porting guide

Updated `portingguide.md` with:
- 3 new rebase checklist items (#39 allow_multiple_instances, #40 rust-embed debug-embed feature, #41 smol→executor.timer pattern)
- Note that the e2e test now has 12 phases (up from 10 in the old guide)
- Commit history extended with all 6 new commits

Release Notes:

- N/A
