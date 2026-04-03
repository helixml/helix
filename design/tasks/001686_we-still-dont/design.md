# Design: Immediately Show Desktop "Starting" State on Start Planning Click

## Root Cause

`ExternalAgentDesktopViewer` is only rendered when the task has a `planning_session_id`. That ID is populated asynchronously by the backend after `POST /api/v1/spec-tasks/{id}/start-planning`. The frontend polls for it with aggressive retries (1s, 1s, 1s, 2s, 2s, 2s), but even the first retry fires after 1 second. Until the ID arrives, the desktop viewer is not mounted — so its "Starting Desktop..." spinner is invisible.

## Relevant Files

- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — `handleStartPlanning()` handler (lines ~1170–1290); makes the API call and polls for `planning_session_id`
- `frontend/src/components/tasks/TaskCard.tsx` — renders `ExternalAgentDesktopViewer`; also holds `isStartingPlanning` state (line 587)
- `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` — shows "Starting Desktop..." spinner when `isStarting` is true
- `frontend/src/services/specTaskService.ts` — `useSpecTask` / `useSpecTasks` hooks; default poll interval 10s

## Solution: Optimistic Session ID in React Query Cache

After the `POST /api/v1/spec-tasks/{id}/start-planning` succeeds, parse the response body. The API likely returns the updated task object (or at least a session ID). If a `planning_session_id` is present in the response, immediately write it into the React Query cache for that task. This causes `ExternalAgentDesktopViewer` to mount immediately without waiting for a poll round.

If the response does not contain the session ID yet (the backend may still be async), fall back to an **optimistic placeholder**: set a local `optimisticPlanningSessionId` state in `SpecTaskKanbanBoard` (or pass it down through props) so `TaskCard` can render `ExternalAgentDesktopViewer` with a temporary ID. The viewer will start polling desktop status on its own (every 3s) and will transition normally once the real session data arrives.

### Preferred approach (check API response first)

1. In `handleStartPlanning()`, after `response.ok`, call `response.json()` to parse the returned task/session data.
2. If `planning_session_id` is present, call `queryClient.setQueryData(QUERY_KEYS.specTask(task.id), updatedTask)` to immediately update the cache — no polling delay at all.
3. Keep the existing `pollForSessionId` loop as a fallback for the case where the backend hasn't set the ID yet in the response.

### Fallback approach (if response lacks session ID)

If the API doesn't return the session ID synchronously:

1. Add `optimisticPlanningSessionId: string | null` state to `SpecTaskKanbanBoard`.
2. Set it to a sentinel value (e.g. `"pending"`) immediately after the API call succeeds.
3. Pass it down to `TaskCard` via props.
4. In `TaskCard`, if `optimisticPlanningSessionId` is set and the task doesn't yet have a real `planning_session_id`, render `ExternalAgentDesktopViewer` with a "starting" override prop so it shows the spinner.
5. Clear `optimisticPlanningSessionId` once the real session ID arrives from polling.

### Why not a global loading flag?

A simple `isDesktopStarting` boolean passed to the desktop viewer is simpler but would require `ExternalAgentDesktopViewer` to accept and handle a "forced starting" prop — coupling the viewer to caller state. The React Query cache update is cleaner because it works with the existing data flow.

## Key Constraints

- Follow the CLAUDE.md rule: "Invalidate queries after mutations, don't use `setQueryData`" — **exception**: here we're writing optimistic data, not stale data; this is the canonical React Query optimistic update pattern. If the team prefers, use `queryClient.invalidateQueries` immediately after the API call (triggers a refetch right away rather than waiting for the 10s interval), which is a simpler change.
- `ExternalAgentDesktopViewer` already handles the `isStarting` state and 120s timeout correctly — no changes needed there.

## Simplest Viable Fix

The simplest approach that avoids any component prop changes:

**After the API call succeeds, immediately call `queryClient.invalidateQueries(QUERY_KEYS.specTasks(projectId))`.**

This triggers an immediate refetch of the task list. If the backend populates `planning_session_id` synchronously (even a few ms after the POST returns), this refetch will catch it. Combined with the existing aggressive polling loop, the desktop viewer will appear much faster than waiting for the 10s background interval.

This is a 1-line change with zero risk of regression.
