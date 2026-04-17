# Implementation Tasks

- [~] Add `last_push_at?: string` to `SpecTaskForActions` interface in `frontend/src/components/tasks/SpecTaskActionButtons.tsx`
- [ ] Add `hasPushed` derived boolean (`!!task.last_push_at`) near the existing `isDirectPush` logic
- [ ] Disable both the "Open PR" / "Accept" and "Reject" buttons in the **inline** variant when `!hasPushed`, with tooltip "Waiting for agent to push code..."
- [ ] Disable both the "Open PR" / "Accept" and "Reject" buttons in the **full-size** variant when `!hasPushed`, with same tooltip
- [ ] Ensure callers of `SpecTaskActionButtons` pass `last_push_at` from the task object (check `SpecTaskKanbanBoard.tsx` and any other call sites)
- [ ] Test: verify button is disabled when task enters implementation with no push, and enables after agent pushes
