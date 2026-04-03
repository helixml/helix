# Implementation Tasks

- [ ] In `frontend/src/components/tasks/SpecTaskActionButtons.tsx`, change the PR rendering condition at line 616 from `task.status === "pull_request"` to `(task.status === "pull_request" || task.status === "done")` so PR buttons are shown on finished tasks
- [ ] Verify in the browser that a finished task (status "done") with associated PRs shows the "View Pull Request" button / multi-PR dropdown
- [ ] Run `cd frontend && yarn build` to confirm no TypeScript errors
