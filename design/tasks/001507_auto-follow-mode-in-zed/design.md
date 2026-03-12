# Design: Auto-Follow Mode and Split-Brain Fix for External WebSocket Sessions

## Architecture Context

Zed's agent-follow system works via `CollaboratorId::Agent`:

1. **Agent location tracking**: When the AI agent reads/edits files, `Project::set_agent_location()` is called, which emits `Event::AgentLocationChanged`.
2. **Following activation**: `Workspace::follow(CollaboratorId::Agent)` registers a `FollowerState` that makes the workspace react to `AgentLocationChanged` events by opening the same file and scrolling to the agent's cursor.
3. **Follow intent**: `AcpThreadView::should_be_following` (defaults to `true`) records the user's intent. When a message is sent, the UI checks this flag and calls `workspace.follow()` if true.

---

## Bug 1: Auto-Follow Not Activating

### Normal Flow (Working)

```
User types in Zed → AcpThreadView::send_impl() → checks should_be_following
  → workspace.follow(CollaboratorId::Agent) → FollowerState registered
  → AcpThread::send() → Agent works → set_agent_location() → AgentLocationChanged
  → Workspace::handle_agent_location_changed() → UI follows agent
```

### External WebSocket Flow (Broken)

```
Helix sends chat_message → WebSocket → thread_service.rs
  → create_new_thread_sync() or handle_follow_up_message()
  → AcpThread::send() → Agent works → set_agent_location() → AgentLocationChanged
  → Workspace::handle_agent_location_changed() → NO FollowerState → nothing happens
```

The gap: `thread_service.rs` operates at the `AcpThread` level and never touches the `Workspace` layer to activate following. The `AgentPanel` notification handler (`ThreadDisplayNotification`) creates the UI view and focuses the panel, but also never activates following.

### Fix: Activate following in the AgentPanel notification handler

The `AgentPanel::new` handler for `ThreadDisplayNotification` already has access to the workspace and window context. After creating the `AcpServerView` and setting it as the active view, add a `workspace.follow(CollaboratorId::Agent)` call.

**Where**: `zed/crates/agent_ui/src/agent_panel.rs`, inside the `ThreadDisplayNotification` handler (~line 845), after `set_active_view` and before the `focus_panel` call.

**Logic**: Read `should_be_following` from the `AcpThreadView` (defaults to `true` for new views). If true, call `workspace.follow(CollaboratorId::Agent)`.

This handles all three broken paths (new thread, follow-up, loaded thread) because they all go through `notify_thread_display`.

For **follow-up messages to existing threads**: The handler currently early-returns if the panel is already showing the same thread entity (~line 819). For follow-ups, we also need to activate following in this early-return path (since the view already exists but follow may have lapsed after the previous generation completed).

**Alternatives considered and rejected:**

- **Option B: Activate following in thread_service.rs** — would require passing a workspace handle and window context into thread_service, crossing abstraction boundaries.
- **Option C: Activate following in from_existing_thread** — `from_existing_thread` doesn't have window context at the right time, and it conflates view creation with workspace-level behavior.

---

## Bug 2: Split-Brain — Duplicate Agent Instances

### The Problem in Detail

The split-brain occurs through this specific navigation sequence:

**Step 1**: Helix sends a message → `thread_service.rs` calls `connection.new_session()` creating **Thread-A** with **NativeAgent-A**. Wraps it in **View-A** via `from_existing_thread` (uses `HeadlessConnection`). Sets as active view. Thread-A starts generating.

**Step 2**: User clicks "View All" → `open_history()` → `set_active_view(History)`.

The `set_active_view` logic (agent_panel.rs ~line 1797):
```
// current is AgentThread (not special), new is History (special)
} else if !current_is_special && new_is_special {
    self.previous_view = Some(std::mem::replace(&mut self.active_view, new_view));
}
```
View-A is stashed in `previous_view`. Thread-A is still generating.

**Step 3**: User clicks a thread from history → `open_thread()` → `_external_thread()` → creates **View-B** with a **new** `NativeAgentServer` → **NativeAgent-B**. If resuming the same session, `load_session` creates **Thread-B**.

`set_active_view(AgentThread { View-B })`:
```
// current is History (special), new is AgentThread (not special)
if current_is_uninitialized || (current_is_special && !new_is_special) {
    self.active_view = new_view;
}
```
**`previous_view` is NOT cleared** — it still holds View-A from Step 2.

**Result**: Two NativeAgent instances running in parallel:
- **NativeAgent-A** (via Thread-A in View-A in `previous_view`): still executing tool calls, writing files
- **NativeAgent-B** (via Thread-B in View-B in `active_view`): the visible agent, responding to user commands

The user sees one agent but two are modifying the filesystem.

### Why the old thread isn't cleaned up

1. **`previous_view` leak**: When transitioning `special → non-special`, the first branch runs (`self.active_view = new_view`) but never touches `previous_view`. The stale View-A stays alive.

2. **No cancellation on view transitions**: `set_active_view` never cancels running turns on outgoing views. It only replaces references.

3. **`HeadlessConnection` can't close sessions**: View-A was created via `from_existing_thread` with `HeadlessConnection`, whose `close_session` returns an error. Even when View-A is eventually dropped and `on_release` fires `close_all_sessions`, nothing actually happens.

4. **NativeAgent-A stays alive**: The `NativeAgentConnection` created by `thread_service` holds the agent. The `Entity<AcpThread>` (Thread-A) has a strong reference in the `THREAD_REGISTRY` AND in View-A. The running turn task holds references that keep everything alive.

5. **Orphaned subscriptions**: Thread-A's persistent subscription in `thread_service` continues to fire, potentially sending duplicate WebSocket events until Thread-B's `register_thread` overwrites the registry entry (but the old subscription closure still references Thread-A's ID).

### Fix Approach

The fix has two parts:

#### Part A: Cancel running threads when switching away from an AgentThread view

In `set_active_view`, when the outgoing view is an `AgentThread`, cancel any running generation before replacing it.

**Where**: `zed/crates/agent_ui/src/agent_panel.rs`, in `set_active_view` (~line 1780), before the view replacement logic.

**Logic**:
```
if let ActiveView::AgentThread { server_view } = &self.active_view {
    if let Some(active_thread) = server_view.read(cx).active_thread() {
        active_thread.update(cx, |thread_view, cx| {
            thread_view.cancel_generation(cx);
        });
    }
}
```

This ensures that when the user navigates to History (or any other view), the running agent is stopped.

#### Part B: Clear `previous_view` when it holds a stale AgentThread

When `set_active_view` transitions from a special view (History) to a new non-special view (AgentThread), and `previous_view` holds an old AgentThread, it should be dropped — not preserved. The user has chosen a different thread; restoring the old one via "back" would be wrong.

**Where**: Same function, in the `current_is_special && !new_is_special` branch.

**Logic**: Before setting `self.active_view = new_view`, also cancel and clear `previous_view`:
```
if current_is_uninitialized || (current_is_special && !new_is_special) {
    // Cancel and drop any stale AgentThread in previous_view
    if let Some(ActiveView::AgentThread { server_view }) = &self.previous_view {
        if let Some(active_thread) = server_view.read(cx).active_thread() {
            active_thread.update(cx, |thread_view, cx| {
                thread_view.cancel_generation(cx);
            });
        }
    }
    self.previous_view = None;
    self.active_view = new_view;
}
```

#### Part C: Clean up THREAD_REGISTRY on thread entity drop

When a Thread entity in the registry is replaced by `register_thread` (same session ID, different entity), the old entity's persistent subscription should be invalidated. Currently the subscription closure captures the thread ID string and keeps firing for the old entity.

**Where**: `zed/crates/external_websocket_sync/src/thread_service.rs`, in `register_thread`.

**Logic**: When overwriting an existing entry, log a warning. The subscription issue is mitigated by Part A (cancelling the thread stops generation, so no more events fire). For a belt-and-suspenders approach, add an `unregister_thread` call in the `AcpServerView::on_release` callback for headless connections.

---

## Codebase Patterns Discovered

- `CollaboratorId::Agent` is the special collaborator ID for the AI agent (not a remote peer).
- `workspace.follow()` is idempotent — calling it when already following just re-focuses the pane (and for Agent, it skips the focus part).
- `AcpThreadView::should_be_following` is the source of truth for user intent; `is_following()` is a derived query.
- The `external_websocket_sync` crate uses global statics with `parking_lot::Mutex` for cross-component communication (callback channels). This is the Helix fork's pattern for bridging the WebSocket layer with GPUI entities.
- `notify_thread_display` is the single funnel point for showing threads in the UI from external sources.
- `set_active_view` uses a `previous_view` stash for "back" navigation from special views (History, Configuration). This stash can leak AgentThread views that hold live agent connections.
- `HeadlessConnection` is a stub that can't actually manage sessions — `close_session` is a no-op. This means headless views can't be cleaned up through the normal connection lifecycle.
- `NativeAgentConnection` (from `thread_service`) and the UI's `NativeAgentServer.connect()` create independent `NativeAgent` instances. Two connections to the same session = two agents operating independently.

## Files Modified

| File | Change |
|------|--------|
| `zed/crates/agent_ui/src/agent_panel.rs` | (Bug 1) Add `workspace.follow(CollaboratorId::Agent)` in `ThreadDisplayNotification` handler, both for new views and the early-return same-entity path |
| `zed/crates/agent_ui/src/agent_panel.rs` | (Bug 2) In `set_active_view`: cancel running generation on outgoing AgentThread views; clear stale `previous_view` when transitioning special → non-special |
| `zed/crates/external_websocket_sync/src/thread_service.rs` | (Bug 2) Add `unregister_thread` function; log warning when overwriting existing registry entry |
| `zed/crates/agent_ui/src/acp/thread_view.rs` | (Bug 2) In `from_existing_thread`'s `on_release`, call `unregister_thread` to clean up the registry |

## Testing

### Auto-follow (Bug 1)
- **Manual**: Send a message from Helix web UI, verify Zed editor follows the agent as it opens and edits files.
- **Manual**: Toggle follow off in Zed, send another Helix message, verify editor does NOT follow.
- **Manual**: Toggle follow back on, send another Helix message, verify following resumes.

### Split-brain (Bug 2)
- **Manual**: Start a long-running agent task from Helix → while generating, navigate to thread list → click the same thread → verify only one agent is running (check with `docker compose exec -T sandbox-nvidia docker logs` for duplicate tool call activity).
- **Manual**: Start a task → navigate to thread list → click a different thread → verify the first task's generation was cancelled (no more file writes from the old task).
- **Manual**: Start a task → navigate to thread list → press "back" → verify the original thread is still visible and functional (the `previous_view` restore path for "back" still works).
- **E2E**: The existing Zed WebSocket sync E2E test infrastructure can be extended with a phase that verifies `is_being_followed(CollaboratorId::Agent)` returns true after an external message triggers generation.

## Implementation Notes

### Decisions made during implementation

- **Bug 1 fix location**: Added `workspace.follow(CollaboratorId::Agent)` in two places within the `ThreadDisplayNotification` handler in `agent_panel.rs`:
  1. In the early-return path (same entity already displayed) — for follow-up messages that reuse the existing view
  2. After `set_active_view` for newly created views — for initial thread creation and loaded threads
  Both check `should_be_following` on the `AcpThreadView` before activating, respecting the user's toggle state.

- **Bug 2 cancellation is `#[cfg(feature = "external_websocket_sync")]` gated**: The split-brain only occurs with Helix's `from_existing_thread` / `HeadlessConnection` path. Normal Zed uses real `NativeAgentConnection` where `close_all_sessions` works. Gating avoids any risk to upstream behavior.

- **Cancellation only targets `previous_view`, not the outgoing `active_view`**: When the user navigates to History, the outgoing AgentThread is stashed in `previous_view` and should NOT be cancelled (user might press "back"). Cancellation only fires when `previous_view` is about to be dropped because a new non-special view is taking over.

- **`go_back` flow is safe**: `go_back` calls `self.previous_view.take()` before `set_active_view`, so `previous_view` is `None` when our cancellation code runs. No risk of cancelling a thread the user wants to restore.

### Gotchas

- **`workspace.follow()` needs window context**: The follow call must happen inside `update_in` (which provides `window`), not inside a plain `update`. The `ThreadDisplayNotification` handler already runs in a `cx.spawn_in(window, ...)` context, so `window` is available.

- **`cancel_generation` is fire-and-forget**: It stores the cancel task in `_cancel_task` on the `AcpThreadView` and returns. The actual cancellation (including tool call cleanup) happens asynchronously. This is fine for our use case — we just need to signal the stop, not wait for it.

- **`unregister_thread` in `on_release`**: The `on_release` callback on `AcpServerView` fires when the GPUI entity is dropped (all strong references gone). This happens when `previous_view` is set to `None` and the old `ActiveView::AgentThread` is dropped. The `ServerState::Connected` check ensures we only try to unregister when there's actually a connected state with an active_id.

- **Borrow checker on `set_active_view` cancellation**: `server_view.read(cx).active_thread()` returns `Option<&Entity<AcpThreadView>>` — the `read(cx)` holds an immutable borrow on `cx`. Calling `active_thread.update(cx, ...)` then tries a mutable borrow, which conflicts. Fix: `.cloned()` on the `Option<&Entity>` to get an owned `Entity` before calling `update`. This is the standard GPUI pattern — always clone the entity handle out of a `read()` before mutating.

- **Build via `./stack build-zed release`**: The dev environment doesn't have cargo installed directly, but `./stack build-zed release` runs the build inside Docker with BuildKit cache mounts. First build bootstraps rustup (~45s), subsequent builds are incremental. The diagnostics tool catches syntax errors but not borrow checker issues — always run the real build.