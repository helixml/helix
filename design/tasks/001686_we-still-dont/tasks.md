# Implementation Tasks

- [x] In `SpecTaskKanbanBoard.tsx` `handleStartPlanning()`, after `response.ok`, call `queryClient.invalidateQueries(QUERY_KEYS.specTasks(projectId))` to trigger an immediate refetch
- [x] Verify in the browser: clicking "Start Planning" shows the "Starting Desktop..." `CircularProgress` spinner within ~1s
- [x] Confirm the 120s "Desktop may have failed to start" timeout and Stop button still work correctly
- [x] Confirm error case: if the API call fails, no desktop viewer is shown and the error toast appears as before
