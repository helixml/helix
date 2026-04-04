# Implementation Tasks

## Fix 1: Startup Script Button → Just Do It Mode

- [x] Update `createSpecTaskMutation` type in ProjectSettings.tsx to include `just_do_it_mode?: boolean`
- [x] Add `just_do_it_mode: true` to the mutation call (line ~1038)
- [x] Update success message to reflect immediate implementation (not "Created task")

## Fix 2: Planning Task Waiting Indicator

- [x] In TaskCard.tsx, find where `spec_generation` status is rendered
- [x] Add waiting indicator matching PR pattern (CircularProgress + text)
- [x] Add 2-minute timeout check using `status_updated_at`
- [x] Show warning Alert after timeout: "Agent hasn't pushed specs yet..."
- [x] Test: TypeScript compiles without errors

## Fix 3: Skip Spec Button (Optional)

- [x] Add "Skip Spec" button in SpecTaskActionButtons.tsx for `spec_generation` status
- [x] Style as `variant="outlined"` (non-primary)
- [x] Backend: Uses existing v1SpecTasksUpdate endpoint with status + just_do_it_mode
- [x] Frontend: Add useSkipSpec mutation in specTaskWorkflowService.ts
- [x] Test: TypeScript compiles without errors

## Fix 4: Reopen Completed Task Button (Optional)

- [x] Add "Reopen" button in SpecTaskActionButtons.tsx for `done` status
- [x] Style as `variant="outlined"` (non-primary)
- [x] Backend: Uses existing v1SpecTasksUpdate endpoint with status
- [x] Frontend: Add useReopenTask mutation in specTaskWorkflowService.ts
- [x] Test: TypeScript compiles without errors
