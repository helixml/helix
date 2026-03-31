# Design

## Key Files

- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — derives `activeSessionId` from task; passes to `useSandboxState`
- `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` — `useSandboxState()` hook maps session metadata to sandbox states
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — `handleStartPlanning()` calls the API and updates local task state
- `api/pkg/server/spec_driven_task_handlers.go` — `/api/v1/spec-tasks/{taskId}/start-planning` endpoint

## The Bug

In `SpecTaskDetailContent.tsx`:
```ts
const activeSessionId = selectedThreadSessionId || task?.planning_session_id;
// useSandboxState("") → "absent" state → shows stopped desktop
```

When the task is queued but `planning_session_id` is not yet set, `useSandboxState` receives an empty string and returns the absent/stopped state.

## Chosen Fix: Optimistic "Starting" State in the Frontend

When the task status is `queued_spec_generation` (or `spec_generation`) but `planning_session_id` is not yet populated, treat `isDesktopStarting` as `true` without waiting for the session poll.

**In `SpecTaskDetailContent.tsx`**, augment the sandbox state:
```ts
const isQueuedForPlanning = task?.status === "queued_spec_generation" || task?.status === "spec_generation";
const effectiveIsStarting = isDesktopStarting || (isQueuedForPlanning && !activeSessionId);
```

Use `effectiveIsStarting` wherever `isDesktopStarting` is used to decide which desktop UI to show.

This is the minimal change: no backend changes needed, no new polling, no race conditions. Once `planning_session_id` is set and the session state comes back as "starting", the flag naturally resolves to `true` via the normal path.

## Alternative Considered

**Return `session_id` from the start-planning API immediately** — would require the backend to synchronously create the session before returning, changing the async design. More invasive.

## Key Pattern Note

`useSandboxState` is driven entirely by the session data polled from the backend. To avoid a flash, any frontend-only "intent" must be tracked separately and merged with the hook's result. The task status fields (`queued_spec_generation`, `spec_generation`) are the right signal because they are set synchronously when the button is clicked.
