# Implementation Tasks

- [x] Add `useMoveToBacklog` hook in `specTaskWorkflowService.ts` that combines `stopAgent` + `updateSpecTask` with status `backlog`
- [x] Add "Move to Backlog" menu item in `TaskCard.tsx` overflow menu for eligible statuses (planning, review, implementation phases)
- [x] Add "Move to Backlog" button/option in `SpecTaskDetailContent.tsx` toolbar for eligible statuses
- [x] Add loading state handling for the combined stop + status update operation
- [x] Add snackbar feedback on success ("Task moved to backlog") and error
- [x] Verify build passes (`yarn build`)
- [x] Merge main to get latest inference provider fixes
- [x] Test: Verify task moves from planning phase back to backlog column
- [x] Test: Verify button is not shown for backlog status (correctly hidden)
- [x] Test: Verify snackbar shows "Task moved to backlog" on success

## Testing Notes

- Successfully tested Move to Backlog feature in helix-in-helix environment
- Task in "spec_generation" (Planning) status showed "Move to Backlog" menu option
- Clicking the option moved the task back to Backlog column
- Snackbar confirmation "Task moved to backlog" displayed correctly
- Button correctly hidden for tasks already in backlog status