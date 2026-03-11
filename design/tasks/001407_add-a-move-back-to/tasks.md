# Implementation Tasks

- [x] Add `useMoveToBacklog` hook in `specTaskWorkflowService.ts` that combines `stopAgent` + `updateSpecTask` with status `backlog`
- [x] Add "Move to Backlog" menu item in `TaskCard.tsx` overflow menu for eligible statuses (planning, review, implementation phases)
- [x] Add "Move to Backlog" button/option in `SpecTaskDetailContent.tsx` toolbar for eligible statuses
- [x] Add loading state handling for the combined stop + status update operation
- [x] Add snackbar feedback on success ("Task moved to backlog") and error
- [x] Verify build passes (`yarn build`)
- [x] Merge main to get latest inference provider fixes
- [ ] Manual test: Verify task moves from planning phase back to backlog column (blocked: helix-in-helix inference setup needs LICENSE_KEY and ANTHROPIC_BASE_URL in .env - older sessions missing these)
- [ ] Manual test: Verify task moves from implementation phase back to backlog (agent stops)
- [ ] Manual test: Verify button is not shown for backlog, queued, done, and pull_request statuses

## Notes

Testing blocked by helix-in-helix infrastructure issue:
- This session predates the startup.sh changes that auto-add LICENSE_KEY and ANTHROPIC_BASE_URL to .env
- Manually added these vars but outer-api model listing returns empty (possible routing issue)
- Code implementation is complete and builds successfully - recommend testing in fresh environment