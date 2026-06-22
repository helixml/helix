# Add stop button to spec task chat panel

## Summary

Zed has a stop button users can click to interrupt the current agent turn. The `ExternalAgentDesktopViewer` component already had this wired up. This PR adds the same button to the spec-task chat panel (`SpecTaskDetailContent`).

The full cancel roundtrip already existed end-to-end:
- Backend: `POST /api/v1/sessions/{id}/cancel` → `cancelCurrentTurnIfActive` → sends `cancel_current_turn` over the external WebSocket sync
- Zed: receives it, calls `stop_turn` on the active ACP thread, responds with `turn_cancelled`
- Backend: marks the interaction as `interrupted`

The only missing piece was the frontend wiring in `SpecTaskDetailContent`.

## Changes

- **`api/pkg/server/session_handlers.go`**: Add proper swagger `@Summary`/`@Router` annotations to `cancelSessionTurn` (was missing, so the endpoint didn't appear in codegen). Also fixed a misplaced swagger block — the `stopExternalAgentSession` annotations were sitting above `cancelSessionTurn`.
- **`frontend/src/api/api.ts`** (generated): Regenerated via `./stack update_openapi` — adds `v1SessionsCancelCreate`.
- **`frontend/src/components/tasks/SpecTaskDetailContent.tsx`**:
  - Add `refetchInterval: 3000` to existing `useGetSession` hook so the session state stays fresh
  - Compute `isAgentBusy` from last interaction `state === 'waiting'`
  - Add `handleCancelTurn` callback that calls `api.getApiClient().v1SessionsCancelCreate()`
  - Pass `onCancel` and `isAgentBusy` to `RobustPromptInput` — this is what actually shows the stop button

## How it works

`RobustPromptInput` renders a `StopCircle` icon button (warning colour) whenever both `isAgentBusy && onCancel` are truthy. Clicking it fires `handleCancelTurn` → `POST /api/v1/sessions/{id}/cancel`. The agent receives the cancel over WebSocket, stops the current turn, and the interaction state transitions from `waiting` → `interrupted`, which clears `isAgentBusy` and hides the button.
