# Implementation Tasks: Navigate to Task Detail Page After Creating a Spec Task

- [x] Update `handleTaskCreated` in `frontend/src/pages/SpecTasksPage.tsx` to navigate to `project-task-detail` with the new task id (kanban/audit views)
- [x] In the same handler, keep workspace mode in the workspace by navigating to `project-specs` with `tab=workspace` and `openTask=<id>`
- [x] Drop the now-pointless `setFocusTaskId` / `setRefreshTrigger` calls from that handler
- [x] Update `handleTaskCreated` in `frontend/src/pages/SpecTaskDetailPage.tsx` to navigate to the newly created task's detail page
- [x] Leave `NewSpecTaskForm` and `TabsView` creation behaviour unchanged (verify by reading, not by editing)
- [x] Remove the dead `focusTaskId` / `focusStartPlanning` plumbing across `SpecTasksPage.tsx`, `SpecTaskKanbanBoard.tsx`, and `TaskCard.tsx` (only if Open Question 3 is confirmed)
- [x] Run `cd frontend && yarn build` and confirm no TypeScript errors
- [x] Test in the inner Helix: create from kanban → lands on detail page; Back returns to the board
- [x] Test in the inner Helix: create from a task detail page → lands on the new task's detail page
- [~] Test in the inner Helix: create from the workspace side panel → stays in workspace with the task open as a tab; create from the workspace create-tab → unchanged
- [ ] Test in the inner Helix: create a task with an attachment → attachment present on the destination page (upload not cut short)
- [ ] Confirm `Onboarding.test.tsx` and the rest of the frontend test suite still pass
- [ ] Commit with a conventional message (`feat(frontend): navigate to task detail after creating a spec task`) and open the PR
