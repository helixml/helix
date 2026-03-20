# Implementation Tasks

- [x] In `frontend/src/components/tasks/NewSpecTaskForm.tsx` around line 339, replace `Promise.all(taskLabels.map(...))` with a sequential `for...of` loop that awaits each `addLabelMutation.mutateAsync` call
- [x] Verify fix by creating a task with 3+ labels and confirming all labels appear on the saved task
- [x] Run `cd frontend && yarn build` to confirm no TypeScript errors
