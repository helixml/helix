# Implementation Tasks

- [x] Add `last_push_at?: string` to `SpecTaskForActions` interface in `frontend/src/components/tasks/SpecTaskActionButtons.tsx`
- [x] Add `hasPushed` derived boolean (`!!task.last_push_at`) near the existing `isDirectPush` logic
- [x] Disable both the "Open PR" / "Accept" and "Reject" buttons in the **inline** variant when `!hasPushed`, with tooltip "Waiting for agent to push code..."
- [x] Disable both the "Open PR" / "Accept" and "Reject" buttons in the **full-size** variant when `!hasPushed`, with same tooltip
- [x] Ensure callers of `SpecTaskActionButtons` pass `last_push_at` from the task object (TaskCard.tsx + SpecTaskDetailContent.tsx x2)
- [x] Test: frontend build passes; E2E not possible (no API stack running locally) — WARNING: NOT tested in browser
