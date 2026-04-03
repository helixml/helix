# Design: Immediately Show Desktop "Starting" State on Start Planning Click

## Root Cause

`ExternalAgentDesktopViewer` is only rendered when the task has a `planning_session_id`. That ID is populated asynchronously by the backend after `POST /api/v1/spec-tasks/{id}/start-planning`. The frontend polls for it with aggressive retries (1s, 1s, 1s, 2s, 2s, 2s), but even the first retry fires after 1 second. Until the ID arrives, the desktop viewer is not mounted — so its "Starting Desktop..." spinner is invisible.

## Relevant Files

- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — `handleStartPlanning()` handler (lines ~1170–1290); makes the API call and polls for `planning_session_id`
- `frontend/src/components/tasks/TaskCard.tsx` — renders `ExternalAgentDesktopViewer`; also holds `isStartingPlanning` state (line 587)
- `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` — shows "Starting Desktop..." spinner when `isStarting` is true
- `frontend/src/services/specTaskService.ts` — `useSpecTask` / `useSpecTasks` hooks; default poll interval 10s

## Solution

In `handleStartPlanning()` in `SpecTaskKanbanBoard.tsx`, after `response.ok`, immediately call:

```ts
queryClient.invalidateQueries(QUERY_KEYS.specTasks(projectId))
```

This triggers an immediate refetch of the task list rather than waiting for the 10s background poll interval. The backend populates `planning_session_id` shortly after the POST returns, so this refetch will catch it and cause `ExternalAgentDesktopViewer` to mount and show its "Starting Desktop..." spinner without a perceptible delay.

The existing aggressive `pollForSessionId` loop stays in place as a safety net for cases where the first refetch still doesn't see the ID.

`ExternalAgentDesktopViewer` already handles the `isStarting` state and 120s timeout correctly — no changes needed there.
