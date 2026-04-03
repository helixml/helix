# Implementation Tasks

- [~] In `SpecTaskKanbanBoard.tsx` `handleStartPlanning()`, after `response.ok`, call `queryClient.invalidateQueries(QUERY_KEYS.specTasks(projectId))` to trigger an immediate refetch
- [ ] Verify in the browser: clicking "Start Planning" shows the "Starting Desktop..." `CircularProgress` spinner within ~1s
- [ ] Confirm the 120s "Desktop may have failed to start" timeout and Stop button still work correctly
- [ ] Confirm error case: if the API call fails, no desktop viewer is shown and the error toast appears as before
