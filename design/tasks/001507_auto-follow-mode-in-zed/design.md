# Design: Auto-Follow Mode Fix for External WebSocket Sessions

## Architecture Context

Zed's agent-follow system works via `CollaboratorId::Agent`:

1. **Agent location tracking**: When the AI agent reads/edits files, `Project::set_agent_location()` is called, which emits `Event::AgentLocationChanged`.
2. **Following activation**: `Workspace::follow(CollaboratorId::Agent)` registers a `FollowerState` that makes the workspace react to `AgentLocationChanged` events by opening the same file and scrolling to the agent's cursor.
3. **Follow intent**: `AcpThreadView::should_be_following` (defaults to `true`) records the user's intent. When a message is sent, the UI checks this flag and calls `workspace.follow()` if true.

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

## Proposed Fix

### Option A: Activate following in the AgentPanel notification handler (Recommended)

The `AgentPanel::new` handler for `ThreadDisplayNotification` already has access to the workspace and window context. After creating the `AcpServerView` and setting it as the active view, add a `workspace.follow(CollaboratorId::Agent)` call.

**Where**: `zed/crates/agent_ui/src/agent_panel.rs`, inside the `ThreadDisplayNotification` handler (~line 845), after `set_active_view` and before the `focus_panel` call.

**Logic**: Read `should_be_following` from the `AcpThreadView` (defaults to `true` for new views). If true, call `workspace.follow(CollaboratorId::Agent)`.

This handles all three broken paths (new thread, follow-up, loaded thread) because they all go through `notify_thread_display`.

### Option B: Activate following in thread_service.rs

Call `workspace.follow()` from `create_new_thread_sync` and `handle_follow_up_message`. This would require passing a workspace handle into thread_service, which currently doesn't have one. It would also need window context for the follow call. Rejected because it crosses abstraction boundaries.

### Option C: Activate following in from_existing_thread

Have `AcpServerView::from_existing_thread` activate following during construction. Rejected because `from_existing_thread` doesn't have window context at the right time, and it conflates view creation with workspace-level behavior.

## Decision: Option A

Option A is the cleanest because:
- The `ThreadDisplayNotification` handler already has workspace + window context
- It covers all three external paths (new, follow-up, loaded) in one place
- It respects `should_be_following` from the `AcpThreadView`
- No new abstractions or plumbing needed

## Key Implementation Details

### Respecting user toggle state

For **new threads**: `should_be_following` defaults to `true` in `AcpThreadView::new()`, so following activates automatically. Correct behavior.

For **follow-up messages to existing threads**: The `ThreadDisplayNotification` handler currently early-returns if the panel is already showing the same thread entity. For follow-ups, we need to also activate following in this early-return path (since the view already exists but follow may have lapsed after the previous generation completed).

The `AcpThreadView::is_following` method checks `should_be_following` when not generating, but during generation it checks the actual `workspace.is_being_followed(CollaboratorId::Agent)`. We need to call `workspace.follow()` when the thread starts generating from an external message, bridging this gap.

### Follow-up re-engagement

When a follow-up message arrives, the thread transitions from idle to generating. The `ThreadDisplayNotification` is sent before the message is injected. The handler should call `workspace.follow()` if the existing `AcpThreadView`'s `should_be_following` is true.

### Race condition: notification before message send

`notify_thread_display` is called before `thread.send()` in both `create_new_thread_sync` and `handle_follow_up_message`. Following must be activated after the view is set up but before or at the same time as the message send, which is satisfied since the notification handler runs on the foreground thread synchronously.

## Codebase Patterns Discovered

- `CollaboratorId::Agent` is the special collaborator ID for the AI agent (not a remote peer).
- `workspace.follow()` is idempotent — calling it when already following just re-focuses the pane (and for Agent, it skips the focus part).
- `AcpThreadView::should_be_following` is the source of truth for user intent; `is_following()` is a derived query.
- The `external_websocket_sync` crate uses global statics with `parking_lot::Mutex` for cross-component communication (callback channels). This is the Helix fork's pattern for bridging the WebSocket layer with GPUI entities.
- `notify_thread_display` is the single funnel point for showing threads in the UI from external sources — making it the right place for the fix.

## Files Modified

| File | Change |
|------|--------|
| `zed/crates/agent_ui/src/agent_panel.rs` | Add `workspace.follow(CollaboratorId::Agent)` in the `ThreadDisplayNotification` handler, both for new views and the early-return (same-entity) path |

## Testing

- **Manual**: Send a message from Helix web UI, verify Zed editor follows the agent as it opens and edits files.
- **Manual**: Toggle follow off in Zed, send another Helix message, verify editor does NOT follow.
- **Manual**: Toggle follow back on, send another Helix message, verify following resumes.
- **E2E**: The existing Zed WebSocket sync E2E test infrastructure (`run_docker_e2e.sh`) can be extended with a phase that verifies `is_being_followed(CollaboratorId::Agent)` returns true after an external message triggers generation.