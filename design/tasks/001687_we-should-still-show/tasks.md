# Implementation Tasks

- [x] In `frontend/src/components/tasks/SpecTaskActionButtons.tsx`, change the PR rendering condition at line 616 from `task.status === "pull_request"` to `(task.status === "pull_request" || task.status === "done")` so PR buttons are shown on finished tasks
- [x] Type-check with `npx tsc --noEmit` (passed cleanly)
- [x] Write PR description
