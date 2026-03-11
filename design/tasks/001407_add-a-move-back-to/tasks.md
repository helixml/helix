# Implementation Tasks

- [x] Add `useMoveToBacklog` hook in `specTaskWorkflowService.ts` that combines `stopAgent` + `updateSpecTask` with status `backlog`
- [x] Add "Move to Backlog" menu item in `TaskCard.tsx` overflow menu for eligible statuses (planning, review, implementation phases)
- [~] Add "Move to Backlog" button/option in `SpecTaskDetailContent.tsx` toolbar for eligible statuses
- [ ] Add loading state handling for the combined stop + status update operation
- [ ] Add snackbar feedback on success ("Task moved to backlog") and error
- [ ] Test: Verify task moves from planning phase back to backlog column
- [ ] Test: Verify task moves from implementation phase back to backlog (agent stops)
- [ ] Test: Verify button is not shown for backlog, queued, done, and pull_request statuses