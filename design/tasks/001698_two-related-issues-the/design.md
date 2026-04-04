# Design: Fix Startup Script Button & Planning Task Status Indicators

## Overview

Two related fixes:
1. Make the startup script fix button use "just do it" mode
2. Add "waiting for specs" indicator to planning tasks (matching PR waiting pattern)

## Key Files

| File | Change |
|------|--------|
| `frontend/src/pages/ProjectSettings.tsx` | Add `just_do_it_mode: true` to mutation |
| `frontend/src/components/tasks/TaskCard.tsx` | Add spec waiting indicator (copy PR pattern) |
| `api/pkg/server/spec_driven_task_handlers.go` | Add skip-spec endpoint (optional) |

## Solution 1: Startup Script Button → Just Do It

**File:** `/frontend/src/pages/ProjectSettings.tsx` (lines 1036-1043)

Current mutation call:
```tsx
createSpecTaskMutation.mutate({
  prompt: `...`,
  branch_mode: "new",
  base_branch: "main",
})
```

Add just_do_it_mode:
```tsx
createSpecTaskMutation.mutate({
  prompt: `...`,
  branch_mode: "new",
  base_branch: "main",
  just_do_it_mode: true,  // <-- ADD THIS
})
```

Also update the mutation function signature to accept `just_do_it_mode`.

## Solution 2: Planning Status Indicator

**Pattern to copy:** TaskCard.tsx lines 1390-1422 shows "Waiting for agent to push branch..." for PR phase.

**Apply same pattern to planning phase:** When `status === "spec_generation"`:

```tsx
// In renderPlanningPhase() or similar
const specGenerationStartedAt = task.spec_generation_started_at
  ? new Date(task.spec_generation_started_at).getTime()
  : 0;
const secondsSinceStart = specGenerationStartedAt 
  ? (Date.now() - specGenerationStartedAt) / 1000 
  : 0;
const isWaitingTooLong = secondsSinceStart > 120; // 2 minutes

return isWaitingTooLong ? (
  <Alert severity="warning" sx={{ py: 0.5 }}>
    <Typography variant="caption">
      Agent hasn't pushed specs yet. Please check if the agent is having trouble.
    </Typography>
  </Alert>
) : (
  <Box>
    <CircularProgress size={20} />
    <Typography variant="caption">
      Waiting for agent to push specs...
    </Typography>
  </Box>
);
```

**Note:** May need to add `spec_generation_started_at` timestamp field to track when spec generation began, or use `status_updated_at`.

## Solution 3: Skip Spec Button (Optional)

Add a non-primary "Skip Spec" button for tasks in `spec_generation` status.

**Frontend:** In SpecTaskActionButtons.tsx, add:
```tsx
{task.status === "spec_generation" && (
  <Button variant="outlined" size="small" onClick={handleSkipSpec}>
    Skip Spec
  </Button>
)}
```

**Backend:** Either reuse existing endpoint with a flag, or add new endpoint:
- PUT `/api/v1/spec-tasks/{id}/skip-spec`
- Sets `status = queued_implementation` and `just_do_it_mode = true`

## Solution 4: Reopen Completed Task Button (Optional)

Add a non-primary "Reopen" button for tasks in `done`/completed status. Use case: task was prematurely detected as finished but user wants to continue working.

**Frontend:** In SpecTaskActionButtons.tsx, add:
```tsx
{task.status === "done" && (
  <Button variant="outlined" size="small" onClick={handleReopen}>
    Reopen
  </Button>
)}
```

**Backend:** Either reuse existing endpoint or add:
- PUT `/api/v1/spec-tasks/{id}/reopen`
- Sets `status = implementation`

## Decision: Use status_updated_at vs new field

**Recommendation:** Use existing `status_updated_at` field rather than adding new timestamp. It gets updated when task enters `spec_generation`, so it works for our timeout check.

## Implementation Notes

- Used existing `v1SpecTasksUpdate` endpoint for Skip Spec and Reopen - no new backend endpoints needed
- Added `status_updated_at` to `SpecTaskWithExtras` interface to fix TypeScript error
- Skip Spec sets both `status = queued_implementation` and `just_do_it_mode = true` in a single update
- Reopen simply sets `status = implementation` to move task back to in progress
- Both Skip Spec and Reopen buttons use `variant="outlined"` for non-primary styling
