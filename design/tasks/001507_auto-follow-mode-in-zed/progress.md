# Progress: Auto-Follow Mode & Split-Brain Fix

## Date: 2026-03-13

## Status: Code merged, awaiting manual testing

## PRs

| Repo | PR | Branch | Status |
|------|-----|--------|--------|
| `helixml/zed` | [#20](https://github.com/helixml/zed/pull/20) | `feature/001507-auto-follow-mode-in-zed` | **Merged** (2026-03-13) |
| `helixml/helix` | [#1899](https://github.com/helixml/helix/pull/1899) | — | **Merged** (2026-03-13) — pins new Zed commit in `sandbox-versions.txt` |

## Problem

Two related bugs when Helix sends messages to Zed via the external WebSocket sync layer.

### Bug 1: Auto-follow mode not activating for external messages

When messages arrive from Helix, the code path in `thread_service.rs` injects messages via `AcpThread::send()` and never calls `workspace.follow(CollaboratorId::Agent)`. This means the editor doesn't scroll to follow the agent's file edits — the user has to manually navigate to see what the agent is doing.

**Root cause**: The `ThreadDisplayNotification` handler in `AgentPanel::new` creates the UI view and focuses the panel but never activates following. Compare with the user-initiated path in `AcpThreadView::send_impl()` which explicitly calls `workspace.follow()`.

### Bug 2: Split-brain — duplicate agent instances after view switching

A specific navigation sequence causes two `NativeAgent` instances to run in parallel on the same worktree:

1. Helix sends message → `thread_service` creates Thread-A with NativeAgent-A, wraps in View-A via `from_existing_thread` (uses `HeadlessConnection`)
2. User clicks "View All" → `set_active_view(History)` stashes View-A in `previous_view`
3. User clicks a thread from history → creates new View-B with NativeAgent-B
4. `set_active_view(AgentThread { View-B })` sets active view but **`previous_view` is never cleared** in the `special → non-special` branch
5. Thread-A's `NativeAgent` continues executing tool calls (file writes, terminal commands) on the same worktree

**Contributing factors**:
- `HeadlessConnection::close_session` is a no-op (returns error)
- `set_active_view` never cancels running turns on outgoing views
- Memory leak: the `special → non-special` transition only sets `self.active_view = new_view` without clearing `previous_view`

## Solution

### Bug 1 fix (`agent_panel.rs`)

Added `workspace.follow(CollaboratorId::Agent)` in the `ThreadDisplayNotification` handler at two points:

1. **New view creation path** (~line 878): After `set_active_view`, read `should_be_following` from the active thread view and call `workspace.follow()` if true.
2. **Early-return same-entity path** (~line 822): When the panel already shows the same thread entity, re-engage following for follow-up messages (follow may have lapsed after the previous generation completed).

Both paths are conditional on `should_be_following` (defaults to `true`), so the user's toggle is respected.

### Bug 2 fix

Three changes across three files:

**`agent_panel.rs` — `set_active_view` cancellation:**
- In the `special → non-special` branch: Cancel any running generation in `previous_view` before clearing it, then set `self.previous_view = None`.
- In the `else` branch (direct thread replacement): Same cancellation before clearing `previous_view`.
- Both gated behind `#[cfg(feature = "external_websocket_sync")]` since the split-brain only occurs with Helix's `HeadlessConnection` path.

**`thread_service.rs` — Registry cleanup:**
- Added `unregister_thread()` function to remove entries from `THREAD_REGISTRY`.
- Enhanced `register_thread()` to log warnings when overwriting existing entries with different entities (diagnostic for future split-brain issues).

**`thread_view.rs` — `on_release` cleanup:**
- In `from_existing_thread`'s `on_release` callback: calls `unregister_thread` to clean up the registry when a headless view is dropped.

## Files Changed

| File | Lines | Change |
|------|-------|--------|
| `crates/agent_ui/src/agent_panel.rs` | +64 | Bug 1: follow activation. Bug 2: `set_active_view` cancellation |
| `crates/external_websocket_sync/src/thread_service.rs` | +29, -1 | `unregister_thread()`, diagnostic logging in `register_thread()` |
| `crates/agent_ui/src/acp/thread_view.rs` | +8 | `on_release` cleanup in `from_existing_thread` |

## Build Verification

- [x] `./stack build-zed release` passes (after fixing two E0502 borrow checker errors)
- [x] `./stack build-ubuntu` passes — new Zed binary packaged into desktop image
- [x] Zed PR #20 merged to main
- [x] Helix PR #1899 merged to main — new Zed commit pinned in `sandbox-versions.txt`

## Remaining: Manual Testing

All code is merged. The fix needs to be deployed and manually verified. A fresh desktop image build is required so new sessions pick up the Zed binary containing the fix.

### Test prerequisites
- [ ] Verify `sandbox-versions.txt` points to a Zed commit at or after `6c5ac28` (the merge commit of PR #20)
- [ ] Build a desktop image with the new Zed binary (`./stack build-zed release && ./stack build-ubuntu`)
- [ ] Start a fresh session so it picks up the new image

### Bug 1: Auto-follow tests
- [ ] Send message from Helix web UI → verify Zed editor follows agent (opens files, scrolls to cursor)
- [ ] Send follow-up message to existing thread → verify follow re-activates
- [ ] Toggle follow OFF → send Helix message → verify editor does NOT follow
- [ ] Toggle follow ON → send another Helix message → verify following resumes
- [ ] Type a message directly in Zed agent panel → verify normal follow behavior unchanged

### Bug 2: Split-brain tests
- [ ] Start long-running task → navigate to thread list → click same thread → verify only one agent running (no ghost file writes)
- [ ] Start task → navigate to thread list → click different thread → verify first task cancelled
- [ ] Start task → navigate to thread list → press "back" → verify original thread restored
- [ ] Start task → navigate to thread list → click same thread → send stop → verify it actually stops

### Unit tests (if Rust toolchain available in VM)
- [ ] `cargo test -p external_websocket_sync` passes
- [ ] `cargo test -p agent_ui` passes

## Design Decisions

1. **Bug 2 cancellation is `#[cfg(feature = "external_websocket_sync")]` gated** since the split-brain only occurs with Helix's `HeadlessConnection` path. Upstream Zed doesn't have this issue because `NativeAgentServer.connect()` reuses the same agent instance.

2. **Cancellation only targets `previous_view`**, not the outgoing `active_view` when navigating to History — the user might press "back" to return to it.

3. **`go_back` flow is safe**: it calls `self.previous_view.take()` before `set_active_view`, so `previous_view` is `None` when the cancellation code runs.

4. **`unregister_thread` in `on_release`**: We clean up the registry when the headless view entity is dropped, not when `set_active_view` replaces it. This is more robust because it catches all drop paths (explicit cancellation, GPUI cleanup, etc.).

## Key Codebase Patterns Discovered

- `CollaboratorId::Agent` is the special collaborator ID for the AI agent (not a remote peer).
- `workspace.follow()` is idempotent — safe to call when already following.
- `AcpThreadView::should_be_following` defaults to `true`; `is_following()` is a derived query.
- `notify_thread_display` is the single funnel point for showing threads in the UI from external sources.
- `NativeAgentConnection` (from `thread_service`) and the UI's `NativeAgentServer.connect()` create **independent** `NativeAgent` instances — two connections to the same session = two agents operating independently.
- GPUI borrow checker gotcha: when calling `server_view.read(cx).active_thread()`, must `.cloned()` the returned `Option<&Entity<T>>` before calling `.update(cx, ...)` to avoid E0502 conflicting borrows.