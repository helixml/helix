# Design: Add Stop Button to Spec Task Chat UI

## What already exists

The full cancel roundtrip is already implemented — nothing new is needed in the websocket sync protocol:

- **Backend handler**: `POST /api/v1/sessions/{id}/cancel` → `cancelSessionTurn` (registered in `server.go:1064`) → fires `cancelCurrentTurnIfActive` in a goroutine.
- **`cancelCurrentTurnIfActive`** (`prompt_history_handlers.go:343`): looks up the active `request_id` via the context mappings, calls `sendCancelToExternalAgent`, waits up to 3 s for `turn_cancelled` acknowledgement.
- **WebSocket message**: sends `cancel_current_turn` command with `request_id` over the existing external agent sync WebSocket (`websocket_external_agent_sync.go:2074`).
- **Zed side**: `external_websocket_sync` crate handles `cancel_current_turn` in `websocket_sync.rs:405`, routes via `thread_service.rs`, responds with `turn_cancelled`. The Zed `stop_turn` method in `active_thread.rs:615` is what actually stops the ACP thread.
- **`RobustPromptInput`** (`frontend/src/components/common/RobustPromptInput.tsx:113-116`): already accepts `onCancel?: () => void` and `isAgentBusy?: boolean`. When both are provided it renders a Stop button (line 1671).
- **`ExternalAgentDesktopViewer`** (`frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx:172-188`): the reference implementation — polls session with `useGetSession`, computes `isAgentBusy` from last interaction state, calls raw `fetch` to `/api/v1/sessions/${sessionId}/cancel`, passes both props to `RobustPromptInput`.

## What is missing

`SpecTaskDetailContent.tsx` renders `RobustPromptInput` (around line 2792) without passing `onCancel` or `isAgentBusy`. The stop button is therefore never shown.

## Changes required

### 1. Frontend — `SpecTaskDetailContent.tsx`

Mirror the `ExternalAgentDesktopViewer` pattern:

```tsx
// Poll session to detect active (waiting) interaction
const { data: sessionForCancel } = useGetSession(activeSessionId, {
  enabled: !!activeSessionId,
  refetchInterval: 3000,
});
const isAgentBusy = useMemo(() => {
  const interactions = sessionForCancel?.data?.interactions;
  if (!interactions || interactions.length === 0) return false;
  return interactions[interactions.length - 1].state === 'waiting';
}, [sessionForCancel?.data?.interactions]);

const handleCancelTurn = useCallback(async () => {
  try {
    await api.getApiClient().v1SessionsCancelCreate(activeSessionId);
  } catch (error: any) {
    snackbar.error(error?.message || "Failed to cancel");
  }
}, [activeSessionId]);
```

Then pass to `RobustPromptInput`:
```tsx
onCancel={handleCancelTurn}
isAgentBusy={isAgentBusy}
```

### 2. Frontend — generated API client (`frontend/src/api/api.ts`)

Add `v1SessionsCancelCreate` — a `POST /api/v1/sessions/{id}/cancel` wrapper — so callers don't hand-roll fetch strings. Run `make generate` (or the equivalent swagger-codegen step) after ensuring the OpenAPI spec includes the endpoint. The backend swagger comment block is on `cancelSessionTurn` in `session_handlers.go` but is currently missing `@Router` and summary annotations — add them before re-generating.

## Key files

| File | Change |
|------|--------|
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Add `isAgentBusy`, `handleCancelTurn`, wire to `RobustPromptInput` |
| `api/pkg/server/session_handlers.go` | Add `@Summary`, `@Router` swagger annotations to `cancelSessionTurn` |
| `frontend/src/api/api.ts` | Add `v1SessionsCancelCreate` (via regeneration or manual addition) |

## Notes for implementors

- The `cancelCurrentTurnIfActive` function relies on `requestToSessionMapping` / `requestToInteractionMapping` (correlation maps in the sync server). The June-2026 design doc (`design/2026-06-19-acp-v2-and-websocket-sync-rewrite-strategy.md`) plans to reroute these on `acp_thread_id`, but the cancel path works today — don't block this ticket on that refactor.
- If `activeSessionId` is undefined (no active session selected), disable the wiring entirely — `isAgentBusy` should be `false` and `onCancel` undefined so the stop button does not appear.
