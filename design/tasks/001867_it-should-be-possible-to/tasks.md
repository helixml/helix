# Implementation Tasks

- [ ] Add `useMarkTaskDone()` mutation hook in `frontend/src/services/specTaskWorkflowService.ts` — stop agent if running, then update status to `done` with `CompletedAt`, invalidate queries
- [ ] Add "Mark as Done" menu item with `CheckCircle` icon to the three-dot menu in `frontend/src/components/tasks/TaskCard.tsx` — show for all non-done, non-archived tasks
- [ ] Add confirmation dialog for "Mark as Done" (reuse pattern from `ArchiveConfirmDialog.tsx`, support Shift+click to skip)
- [ ] Verify the existing "Reopen" action works correctly on tasks marked done without a PR
