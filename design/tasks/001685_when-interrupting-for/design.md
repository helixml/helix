# Design: Interrupt Message Actually Interrupts Zed

## Root Cause

The Helix↔Zed WebSocket protocol (defined in `zed-4/crates/external_websocket_sync/PROTOCOL_SPEC.md`) has no cancel/interrupt command. `processInterruptPrompt()` in `helix-4/api/pkg/server/prompt_history_handlers.go:208` sends only a `chat_message` command — Zed has no way to know it should stop its current work first.

## Architecture

### New Protocol Command: `cancel_current_turn`

Add a new Helix→Zed command to the WebSocket protocol:

```json
{
  "type": "cancel_current_turn",
  "data": {
    "request_id": "<request_id of the turn to cancel>"
  }
}
```

`request_id` identifies which in-progress turn to cancel. If the turn is already finished, Zed ignores the command.

### New Protocol Event: `turn_cancelled`

Zed replies with this event once cancellation is complete (or if there was nothing to cancel):

```json
{
  "event_type": "turn_cancelled",
  "data": {
    "request_id": "<same request_id>",
    "status": "cancelled" | "noop"
  }
}
```

### Flow: Interrupt a Queued Message

```
User promotes queued message → interrupt mode
  ↓
Frontend syncs via POST /api/v1/prompt-history/sync (interrupt=true)
  ↓
Backend processInterruptPrompt() called
  ↓
Helix sends cancel_current_turn with current interaction's request_id
  ↓
Zed stops ACP agent turn, sends turn_cancelled
  ↓
Helix marks interrupted interaction as state=interrupted
  ↓
Helix sends chat_message with new interrupt prompt
  ↓
Zed starts new turn, streams response normally
```

### Helix Backend Changes

**`helix-4/api/pkg/server/prompt_history_handlers.go`** (`processInterruptPrompt`):
1. Before calling `sendQueuedPromptToSession`, look up the current `waiting` interaction for the session.
2. If one exists, call a new helper `sendCancelToExternalAgent(sessionID, requestID)` and await `turn_cancelled` (with a short timeout, e.g. 3s).
3. Mark the interrupted interaction as `state=error` (or a new `interrupted` state) in the database.
4. Then call `sendQueuedPromptToSession` as before.

**`helix-4/api/pkg/server/websocket_external_agent_sync.go`**:
- Add `sendCancelToExternalAgent()`: sends `cancel_current_turn` command over the existing WebSocket connection.
- Add handler for incoming `turn_cancelled` event: resolves the pending cancel await and updates interaction state.

### Zed Changes

**`zed-4/crates/external_websocket_sync/src/websocket_sync.rs`**:
- Add `cancel_current_turn` to the command match block.
- Handler: look up the active request by `request_id`; if found, cancel the ACP task (Rust task cancellation or abort handle). Send `turn_cancelled` event back.

**`zed-4/crates/external_websocket_sync/PROTOCOL_SPEC.md`**:
- Document `cancel_current_turn` command and `turn_cancelled` event.

### Helix Frontend: Cancel Button

Add a "Cancel" button in `RobustPromptInput.tsx`, positioned next to the send button. It is only visible when the session has an active `waiting` interaction (i.e. Zed is working).

**Behavior**:
1. User clicks Cancel → frontend calls a new API endpoint `POST /api/v1/sessions/{id}/cancel` (or reuses the existing cancel infrastructure from `sendCancelToExternalAgent`).
2. Button shows a spinner/loading state while awaiting completion.
3. Once the backend confirms cancellation (interaction state changes to `interrupted` via the existing WebSocket session update stream), the button disappears.

**Placement**: Right next to or replacing the send button when Zed is active. Use the `StopCircle` or `Square` icon from lucide-react (consistent with the existing `StopIcon` pattern in `Session.tsx` DesktopControls at line 74).

**Visibility condition**: The `RobustPromptInput` already receives `sessionId` as a prop. Use the existing streaming context / session state to determine if there's a `waiting` interaction. The component already has a `disabled` prop — the cancel button should be independent of it.

### Interaction State

Add `interrupted` as a terminal `InteractionState` in `helix-4/api/pkg/types/types.go`. The frontend should render interrupted interactions distinctly (e.g. "Interrupted" label instead of spinner or error icon). This is a minor UI change.

### Helix Backend: Cancel API Endpoint

Add `POST /api/v1/sessions/{id}/cancel` — a thin endpoint that:
1. Finds the current `waiting` interaction for the session.
2. Calls `sendCancelToExternalAgent(sessionID, requestID)`.
3. Returns immediately (202 Accepted). The interaction state update flows to the frontend via the existing WebSocket session update stream.

This endpoint is used by both the cancel button and internally by `processInterruptPrompt`.

## Key Decisions

- **Synchronous cancel with timeout**: Helix waits up to 3s for `turn_cancelled` before proceeding. This prevents a race where the new `chat_message` arrives at Zed before the old turn has actually stopped.
- **`cancel_current_turn` is idempotent**: If Zed has no active turn for that `request_id`, it immediately returns `turn_cancelled` with `status: noop`.
- **No protocol version bump needed**: New command type; Zed instances that don't recognize it just ignore it (existing behavior for unknown commands). They should be deployed together anyway.
- **`request_id` tracking**: Helix already tracks `requestID` per interaction in the websocket sync code (`websocket_external_agent_sync.go:2523`). Use this to target the right turn.

## Codebase Patterns

- Commands from Helix→Zed are `ExternalAgentCommand{Type, Data}` (`helix-4/api/pkg/types/types.go:2168`).
- Zed matches command types in `websocket_sync.rs:400` — add a new arm.
- Helix handles Zed events in `websocket_external_agent_sync.go` — add handler for `turn_cancelled`.
