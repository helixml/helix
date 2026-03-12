# Requirements: Auto-Follow Mode in Zed for External WebSocket Sessions

## Problem Statement

When a message arrives from Helix via the external WebSocket sync layer, the AI agent processes it (opens files, edits code, moves cursor), but the Zed UI does **not** follow the agent's location. The user watching the desktop stream sees the editor sitting still while the agent works in the background.

This works correctly when a user types a message directly in Zed's agent panel — the UI tracks the agent's file opens and cursor movements. The difference: the normal Zed send path calls `workspace.follow(CollaboratorId::Agent)`, but the external WebSocket path never does.

## Root Cause

Zed's follow system requires an explicit `workspace.follow(CollaboratorId::Agent)` call to start tracking the agent's location. The normal UI flow triggers this in `AcpThreadView::send_impl` and related methods when `should_be_following` is true. The external WebSocket path (`thread_service.rs`) bypasses the UI layer — it injects messages directly via `AcpThread::send()` and never activates following.

Three code paths are affected:

1. **New thread creation** (`create_new_thread_sync`): Creates thread, sends message, notifies AgentPanel to display it, but never calls `workspace.follow()`.
2. **Follow-up messages** (`handle_follow_up_message`): Sends to existing thread via `thread.send()`, never calls `workspace.follow()`.
3. **Loaded threads** (`load_thread_from_agent`): Loads a persisted thread and sends a message, never calls `workspace.follow()`.

## User Stories

1. As a Helix user watching the desktop stream, when I send a message from the Helix web UI, I want to see Zed's editor automatically follow the agent as it opens and edits files, so I can observe the agent's work in real time.

2. As a Helix user sending follow-up messages, I want auto-follow to re-engage each time a new message triggers agent activity, matching the behavior of typing directly in Zed.

## Acceptance Criteria

- [ ] When a `chat_message` arrives via WebSocket and creates a new thread, Zed activates `follow(CollaboratorId::Agent)` so the editor tracks the agent's file opens and cursor movements.
- [ ] When a follow-up `chat_message` arrives for an existing thread, Zed re-activates following if `should_be_following` is true on the `AcpThreadView`.
- [ ] The follow toggle button in the agent panel correctly reflects the active following state for externally-initiated messages.
- [ ] If a user manually disables follow (clicks the crosshair toggle), externally-initiated messages respect that choice and do NOT re-enable following.
- [ ] The fix does not affect the normal (non-WebSocket) message flow — Zed-native sends continue to work identically.

## Out of Scope

- Scroll-to-bottom behavior in the chat panel (separate sticky-scroll system).
- Video streaming FPS or encoding issues.
- Any changes to the Helix API server or frontend.