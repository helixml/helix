# Implementation Tasks

- [~] In `frontend/src/components/tasks/SpecTaskDetailContent.tsx` (~line 464): remove `task.status !== "done"` and `task.status !== "pull_request"` from the `canMoveToBacklog` condition
- [ ] In `frontend/src/components/tasks/TaskCard.tsx` (~line 610): remove `task.phase !== "completed"`, `task.phase !== "pull_request"`, `task.status !== "done"`, and `task.status !== "pull_request"` from the `canMoveToBacklog` condition
- [ ] Verify `yarn build` passes with no type errors
- [ ] Manual test: open a task in `pull_request` status → confirm "Move to Backlog" button appears in detail view and Kanban card menu → click it → confirm task moves to Backlog column
- [ ] Manual test: open a task in `done` status → confirm "Move to Backlog" appears → click it → confirm task moves to Backlog column
- [ ] Manual test: confirm archived tasks still do NOT show the "Move to Backlog" option
- [ ] Manual test: confirm queued tasks (`queued_implementation`, `queued_spec_generation`, `spec_approved`) still do NOT show "Move to Backlog"