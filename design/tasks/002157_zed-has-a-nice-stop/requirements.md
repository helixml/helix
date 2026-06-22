# Requirements: Add Stop Button to Spec Task Chat UI

## Background

Zed's agent UI has a prominent Stop button users can click to halt the current agent turn mid-flight. Helix's `ExternalAgentDesktopViewer` already has this wired up to `RobustPromptInput`. The spec-task chat panel (`SpecTaskDetailContent`) does not — the stop button is simply absent there.

The full cancel roundtrip already exists: `POST /api/v1/sessions/{id}/cancel` → `cancel_current_turn` over the external WebSocket sync protocol → Zed stops the ACP thread turn → responds `turn_cancelled` → Helix marks the interaction interrupted. Nothing new is needed in the websocket protocol itself.

## User Stories

**As a user watching an agent run in the spec-task chat panel**, I want a Stop button so I can interrupt a runaway or wrong-direction agent turn without having to reload the page or wait for the turn to finish.

**As a user**, I want the Stop button to appear only when the agent is actively running (last interaction in `waiting` state), so it doesn't clutter the UI when the agent is idle.

**As a user**, I want the Stop button to disappear and the input to return to normal after the turn is cancelled.

## Acceptance Criteria

1. A Stop button appears in the chat input area of `SpecTaskDetailContent` when the agent has an active (waiting) interaction.
2. Clicking Stop calls `POST /api/v1/sessions/{id}/cancel` and the agent turn is interrupted.
3. After cancellation the button disappears and the input returns to the normal send state.
4. The button is not shown when the session is idle (no waiting interaction).
5. The implementation matches the existing pattern in `ExternalAgentDesktopViewer.tsx` for UI consistency.
6. The generated frontend API client exposes a `v1SessionsCancelCreate` helper so the fetch is not a raw string-interpolated call.
