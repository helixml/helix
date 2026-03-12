# Requirements: Auto-Follow Mode in Zed for External WebSocket Sessions

## Problem Statement

Two related bugs affect the Helix-Zed integration:

### Bug 1: Auto-follow not activating for external messages

When a message arrives from Helix via the external WebSocket sync layer, the AI agent processes it (opens files, edits code, moves cursor), but the Zed UI does **not** follow the agent's location. The user watching the desktop stream sees the editor sitting still while the agent works in the background.

This works correctly when a user types a message directly in Zed's agent panel — the UI tracks the agent's file opens and cursor movements. The difference: the normal Zed send path calls `workspace.follow(CollaboratorId::Agent)`, but the external WebSocket path never does.

### Bug 2: Split-brain — duplicate agent instances after view switching

When a user navigates to the thread list (history view) and then back into a specific thread, two separate agent instances end up running in parallel. One agent continues writing files from the previous turn while the other responds to a stop request. This manifests as:

- Files being modified by an invisible agent after the user pressed stop
- Conflicting edits from two agents on the same worktree
- The visible agent panel showing "stopped" while background work continues

## Root Causes

### Auto-follow (Bug 1)

Zed's follow system requires an explicit `workspace.follow(CollaboratorId::Agent)` call to start tracking the agent's location. The normal UI flow triggers this in `AcpThreadView::send_impl` and related methods when `should_be_following` is true. The external WebSocket path (`thread_service.rs`) bypasses the UI layer — it injects messages directly via `AcpThread::send()` and never activates following.

Three code paths are affected:

1. **New thread creation** (`create_new_thread_sync`): Creates thread, sends message, notifies AgentPanel to display it, but never calls `workspace.follow()`.
2. **Follow-up messages** (`handle_follow_up_message`): Sends to existing thread via `thread.send()`, never calls `workspace.follow()`.
3. **Loaded threads** (`load_thread_from_agent`): Loads a persisted thread and sends a message, never calls `workspace.follow()`.

### Split-brain (Bug 2)

The split-brain occurs through this sequence:

1. Helix sends a message → `thread_service` creates **Thread-A** via `connection.new_session()` (with its own `NativeAgent` instance), wraps it in **View-A** via `from_existing_thread`, and sets it as the active view. Thread-A starts generating.
2. User clicks "View All" → `open_history()` → `set_active_view(History)`. The `set_active_view` logic stores View-A in `previous_view` (because it transitions from non-special → special).
3. User clicks a specific thread from history → `open_thread()` → `_external_thread()` creates a **new** `AcpServerView` (**View-B**) with a **new** `NativeAgentServer` → new `NativeAgent` connection. If the same thread is selected, `load_session` creates **Thread-B** — a separate `Entity<AcpThread>` for the same session.
4. `set_active_view(AgentThread { View-B })` sets the new active view. But **`previous_view` still holds View-A** (it's never cleared in this transition path: History → AgentThread takes the first branch which only sets `self.active_view = new_view`).

The result:
- **Thread-A** is still alive — held by View-A in `previous_view`, with its `NativeAgent` connection running
- **Thread-B** is the new visible thread — held by View-B in `active_view`, with a different `NativeAgent` connection
- Thread-A's running turn is never cancelled (nothing calls `thread.cancel()` during view transitions)
- Thread-A's `NativeAgent` continues executing tool calls (file writes, terminal commands) on the same worktree
- `THREAD_REGISTRY` gets overwritten with Thread-B, but Thread-A's persistent subscription from `thread_service` remains active, potentially sending duplicate WebSocket events

Contributing factors:
- `HeadlessConnection::close_session` is a no-op (returns error), so `close_all_sessions` on View-A's drop does nothing
- `set_active_view` never cancels running turns on the outgoing view
- `previous_view` is not cleared when transitioning from a special view (History) to a non-special view (AgentThread) via a new thread selection

## User Stories

1. As a Helix user watching the desktop stream, when I send a message from the Helix web UI, I want to see Zed's editor automatically follow the agent as it opens and edits files, so I can observe the agent's work in real time.

2. As a Helix user sending follow-up messages, I want auto-follow to re-engage each time a new message triggers agent activity, matching the behavior of typing directly in Zed.

3. As a Helix user, when I navigate to the thread list and back to a thread, I expect only one agent instance to be active at a time. If the previous agent was mid-generation, it should be cancelled before a new instance takes over.

## Acceptance Criteria

### Auto-follow

- [ ] When a `chat_message` arrives via WebSocket and creates a new thread, Zed activates `follow(CollaboratorId::Agent)` so the editor tracks the agent's file opens and cursor movements.
- [ ] When a follow-up `chat_message` arrives for an existing thread, Zed re-activates following if `should_be_following` is true on the `AcpThreadView`.
- [ ] The follow toggle button in the agent panel correctly reflects the active following state for externally-initiated messages.
- [ ] If a user manually disables follow (clicks the crosshair toggle), externally-initiated messages respect that choice and do NOT re-enable following.
- [ ] The fix does not affect the normal (non-WebSocket) message flow — Zed-native sends continue to work identically.

### Split-brain prevention

- [ ] When the active view changes away from an AgentThread (e.g., to History), any running generation on the outgoing thread is cancelled.
- [ ] When a new AgentThread view replaces an old one (directly or via History → thread selection), the old view's thread is cancelled before the new one starts.
- [ ] `previous_view` is properly cleaned up when a new non-special view replaces a special view, preventing stale View references from keeping old threads alive.
- [ ] After navigating History → select thread, only one `NativeAgent` instance is active for that session.
- [ ] The `THREAD_REGISTRY` entry and persistent WebSocket subscription are consistent — no orphaned subscriptions sending events for dead threads.

## Out of Scope

- Scroll-to-bottom behavior in the chat panel (separate sticky-scroll system).
- Video streaming FPS or encoding issues.
- Any changes to the Helix API server or frontend.
- Refactoring the `previous_view` mechanism entirely (we fix the specific leak, not redesign navigation).