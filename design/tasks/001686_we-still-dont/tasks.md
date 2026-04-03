# Implementation Tasks

- [ ] In `SpecTaskKanbanBoard.tsx` `handleStartPlanning()`, after `response.ok`, parse the response body with `response.json()` and check if it contains `planning_session_id`
- [ ] If `planning_session_id` is in the API response, immediately update the React Query cache with `queryClient.setQueryData` so `TaskCard` renders `ExternalAgentDesktopViewer` without waiting for a poll
- [ ] If the API response does not include `planning_session_id`, immediately call `queryClient.invalidateQueries` on the spec tasks query to trigger a refetch right away (instead of waiting for the 10s background interval)
- [ ] Verify in the browser: clicking "Start Planning" shows the "Starting Desktop..." `CircularProgress` spinner within ~1s (not after a multi-second poll delay)
- [ ] Confirm the 120s "Desktop may have failed to start" timeout and Stop button still work correctly
- [ ] Confirm error case: if the API call fails, no desktop viewer is shown and the error toast appears as before
