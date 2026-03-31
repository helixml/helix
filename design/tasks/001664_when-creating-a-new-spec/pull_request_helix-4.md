# Invalidate React Query cache immediately on spec task creation

## Summary

New spec tasks now appear in the list instantly after creation instead of after a ~5 second delay.

Previously, `queryClient.invalidateQueries(["spec-tasks"])` was called only after all label mutations completed. If labels were selected (each requires an API roundtrip) or if the component unmounted before reaching that line, the list stayed stale until the next 10-second polling cycle — making it look like the submission had failed.

## Changes

- `frontend/src/components/tasks/NewSpecTaskForm.tsx`: added a `queryClient.invalidateQueries({ queryKey: ["spec-tasks"] })` call immediately after `v1SpecTasksFromPromptCreate` succeeds, before the label mutation loop. The second invalidation (after labels) is kept so the list also reflects label data once those finish.
