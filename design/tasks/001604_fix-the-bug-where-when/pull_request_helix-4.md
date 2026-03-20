# Fix spectask label persistence: save all labels, not just one

## Summary

When creating a spectask with multiple labels, only one label was being saved. The bug was a race condition caused by adding labels concurrently using `Promise.all()`.

The backend `AddSpecTaskLabel` function uses a read-then-write pattern. Under PostgreSQL MVCC, all concurrent requests read the same initial state and each one overwrites the previous — only the last writer's label survived.

Fixed by switching to sequential label addition (sequential `for...of` instead of `Promise.all`), ensuring each label is fully saved before the next one starts.

## Changes

- `frontend/src/components/tasks/NewSpecTaskForm.tsx`: Replace `Promise.all(taskLabels.map(...))` with sequential `for...of` loop when adding labels after task creation
