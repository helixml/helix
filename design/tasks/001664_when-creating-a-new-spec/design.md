# Design: Immediate Cache Invalidation on Spec Task Creation

## Current Flow

In `NewSpecTaskForm.tsx` (lines 279–362), after task creation:

```
1. await v1SpecTasksFromPromptCreate(...)   ← task created on server
2. await addLabelMutation (loop per label)  ← labels added (slow if multiple)
3. queryClient.invalidateQueries(["spec-tasks"])  ← cache finally cleared
```

The invalidation at step 3 may arrive 0–N seconds late (N = number of labels × label API roundtrip). If the component unmounts or an error is thrown in the label loop, invalidation is skipped entirely, leaving the list stale until the 10-second polling interval fires.

## Fix

Move `queryClient.invalidateQueries({ queryKey: ["spec-tasks"] })` to immediately after step 1 (task creation success), before the label loop. Keep a second invalidation after labels complete to reflect the labeled state.

```
1. await v1SpecTasksFromPromptCreate(...)
2. queryClient.invalidateQueries(["spec-tasks"])  ← MOVED HERE
3. await addLabelMutation (loop per label)
4. queryClient.invalidateQueries(["spec-tasks"])  ← keep for label refresh
```

This is a one-line move within `NewSpecTaskForm.tsx`. No architectural changes needed.

## Notes

- The task list query (`useSpecTasks`) polls every 10 seconds — invalidation bypasses the wait.
- Query key `["spec-tasks"]` is a broad match that invalidates all filtered list variants via React Query key prefix matching.
- The `specTaskService.ts` query key definitions are already correct; only the call site order needs changing.
