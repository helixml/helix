# Show desktop "Starting" spinner immediately when Start Planning is clicked

## Summary

When the user clicks "Start Planning", the "Starting Desktop..." spinner now appears immediately after the API call succeeds, rather than waiting up to 10 seconds for the next background poll interval.

## Changes

- `SpecTaskKanbanBoard.tsx`: import `useQueryClient` from `@tanstack/react-query` and call `queryClient.invalidateQueries({ queryKey: ["spec-tasks"] })` right after `POST /api/v1/spec-tasks/{id}/start-planning` succeeds

This triggers an immediate React Query refetch. Once `planning_session_id` is populated on the task (which happens shortly after the POST), `ExternalAgentDesktopViewer` mounts and shows its spinner without delay. The existing aggressive polling loop stays in place as a safety net.
