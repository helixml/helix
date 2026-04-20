# Design: Merge Latest Zed Upstream into Helix Fork

## Repository Layout

- **Fork**: `helixml/zed` (local clone at `/home/retro/work/zed/`, remote `origin`)
- **Upstream**: `zed-industries/zed` (add as remote `upstream` if not present)
- **Porting guide**: `/home/retro/work/zed/portingguide.md`
- **Previous spec**: `helix-specs/design/tasks/001723_merge-latest-zed/`

## Current State

- Fork last merged with upstream: April 16, 2026 (task 001723)
- Fork HEAD: `1e07aea` (PR #41, ACP auto-approve, Apr 15)
- Upstream HEAD: `1db292d` (docs fix, Apr 19)
- Delta: ~5 days of upstream commits (small merge compared to 001723's 506-commit gap)

## Merge Strategy

Use `git merge upstream/main` — consistent with all previous merges. Not rebase.

1. Create feature branch from fork `main`
2. Add upstream remote, fetch latest
3. Merge upstream/main into feature branch
4. Resolve conflicts, verify critical fixes
5. Run tests, update porting guide
6. Open PR against `helixml/zed` main

## Upstream Changes Since Last Merge (Apr 16-19)

Analyzed from `zed-industries/zed` commit history:

| Change | Risk | Notes |
|--------|------|-------|
| Feature flag overrides (#54206) | **HIGH** | Revamps feature flag system — enum values, settings UI overrides. Our `#[cfg(feature = "external_websocket_sync")]` compile-time gates are unaffected (Cargo features, not runtime flags), but if upstream touched `feature_flags.rs` or related types our code references, conflicts are possible. |
| Fix remote projects in sidebar (#54198) | Low | Workspace/sidebar change — low overlap with Helix patches |
| Open workspaces list in sidebar (#54207) | Medium | Sidebar menu changes — could conflict if agent_panel.rs or workspace.rs touched |
| Fix tsgo LSP (#54201) | Low | LSP-types dependency bump, no overlap |
| Docs: edit prediction (#53714) | None | Documentation only |

## High-Risk Files (Known from Previous Merges)

These are the files most likely to conflict due to Helix-specific patches:

1. **`crates/agent_ui/src/agent_panel.rs`** — Thread display, UI state query callbacks, from_existing_thread calls, onboarding dismissal. Historically the most complex conflict target.
2. **`crates/agent_ui/src/conversation_view.rs`** — HeadlessConnection, from_existing_thread constructor, thread registry.
3. **`crates/acp_thread/src/acp_thread.rs`** — content_only() method, Stopped event handling, cancel() drop fix.
4. **`crates/zed/src/zed.rs`** — WebSocket sync service initialization.
5. **`crates/feature_flags/src/feature_flags.rs`** — If the feature flag overrides PR touches this, check for type changes that might affect our cfg gates.
6. **Cargo.toml files** — Feature propagation chain: `zed/Cargo.toml` -> `agent_ui/Cargo.toml` -> `title_bar/Cargo.toml`.

## Critical Fixes That Must Be Preserved

All 9 critical fixes documented in `portingguide.md`:

1. Keep NativeAgent entity alive during `load_session`
2. No duplicate WebSocket event sends
3. Strip "## Assistant" heading from synced messages
4. Follow-up to non-visible thread must notify UI
5. Flush stale pending entries when different entry starts streaming
6. AcpThread::Stopped must be emitted for every turn
7. THREAD_REGISTRY must be unregistered on entity replacement
8. Cancel must drop send_task, not await it
9. Guard normal-completion Stopped against duplicate emission

## Post-Merge Validation

```bash
# Compile check
cargo check --package zed --features external_websocket_sync

# Unit tests
cargo test -p external_websocket_sync

# E2E tests (requires ANTHROPIC_API_KEY)
cd crates/external_websocket_sync/e2e-test
./run_docker_e2e.sh
```

Verify each critical fix with grep:
```bash
# Fix 1: load_session entity lifetime
grep -n "load_session" crates/agent_ui/src/agent_panel.rs

# Fix 9: stopped_emitted_for_task guard
grep -n "stopped_emitted_for_task" crates/acp_thread/src/acp_thread.rs
```

## Learnings from Previous Merges

Carried forward from tasks 001723, 001617, 001560, 001554:

- **Two ConnectedServerState types exist**: one in `AcpServerView`, one in `ConversationView`. Both must be updated when fields change.
- **Feature propagation chain**: Adding a feature to `external_websocket_sync` requires updating multiple Cargo.toml files.
- **`from_existing_thread()` is the most fragile integration point** — depends on ConnectedServerState field consistency.
- **Type/method renames** are tracked via grep patterns in portingguide.md rebase checklist.
- **Smaller, more frequent merges reduce risk** — this merge should be simpler than 001723's 506-commit gap.
- **Always check for new upstream `ContextServerStatus` variants** — new variants can cause non-exhaustive match failures in our feature-gated code.
