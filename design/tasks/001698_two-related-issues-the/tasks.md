# Implementation Tasks

## Fix 1: Startup Script Button → Just Do It Mode

- [ ] Update `createSpecTaskMutation` type in ProjectSettings.tsx to include `just_do_it_mode?: boolean`
- [ ] Add `just_do_it_mode: true` to the mutation call (line ~1038)
- [ ] Update success message to reflect immediate implementation (not "Created task")

## Fix 2: Planning Task Waiting Indicator

- [ ] In TaskCard.tsx, find where `spec_generation` status is rendered
- [ ] Add waiting indicator matching PR pattern (CircularProgress + text)
- [ ] Add 30-second timeout check using `status_updated_at`
- [ ] Show warning Alert after timeout: "Agent hasn't pushed specs yet..."
- [ ] Test: create a task, start planning, verify indicator appears

## Fix 3: Skip Spec Button (Optional)

- [ ] Add "Skip Spec" button in SpecTaskActionButtons.tsx for `spec_generation` status
- [ ] Style as `variant="outlined"` (non-primary)
- [ ] Backend: Add handler to set `status = queued_implementation` and `just_do_it_mode = true`
- [ ] Frontend: Add mutation to call skip-spec endpoint
- [ ] Test: verify task moves to implementation and can still create PR

## Fix 4: Return to Backlog Button (Optional)

- [ ] Add "Return to Backlog" button in SpecTaskActionButtons.tsx for `spec_review` status
- [ ] Style as `variant="outlined"` (non-primary)
- [ ] Backend: Add handler to set `status = backlog`
- [ ] Frontend: Add mutation to call return-to-backlog endpoint
- [ ] Test: verify task moves back to backlog and can restart planning
