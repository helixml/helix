# Fix startup script button and add planning task indicators

## Summary
Two related fixes for task UI: (1) make the "Get AI to fix it" button use just_do_it mode so it doesn't get stuck in planning, and (2) add visual indicators for tasks waiting for agent to push specs.

## Changes
- **ProjectSettings.tsx**: Add `just_do_it_mode: true` to startup script fix button
- **TaskCard.tsx**: Add "Waiting for agent to push specs..." indicator for `spec_generation` status with 2-minute timeout warning
- **SpecTaskActionButtons.tsx**: Add "Skip Spec" button for planning tasks and "Reopen" button for completed tasks
- **specTaskWorkflowService.ts**: Add `useSkipSpec` and `useReopenTask` mutations

## Test Plan
- [ ] Click "Get AI to fix it" in project settings - should start implementation immediately
- [ ] Start planning on a task - should show "Waiting for agent to push specs..."
- [ ] Wait 2 minutes with no specs pushed - should show warning alert
- [ ] Click "Skip Spec" on planning task - should move to implementation
- [ ] Click "Reopen" on completed task - should move back to in progress
