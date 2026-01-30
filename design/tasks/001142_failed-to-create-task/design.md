# Design: Allow Multiple Tasks on Same Branch

## Overview

Remove the validation that prevents creating a new task when the selected branch already has an active task. This is a simple bug fix - the validation is overly restrictive.

## Current Behavior (Broken)

**Code location:** `api/pkg/services/spec_driven_task_service.go` lines 152-170

```go
// VALIDATION: Check for active tasks on the same branch
// This prevents multiple agents working on the same branch which causes confusion
if branchMode == types.BranchModeExisting && req.WorkingBranch != "" {
    existingTasks, err := s.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
        ProjectID:  req.ProjectID,
        BranchName: req.WorkingBranch,
    })
    // ... returns error if any active task found
}
```

This blocks legitimate use cases where a user wants to:
- Start fresh with a new agent conversation on the same branch
- Create a task for a different aspect of work on the same branch
- Resume work with updated requirements

## Solution

Delete the validation block entirely. Multiple tasks on the same branch are fine - each task has its own session, specs, and state.

## Files to Modify

- `api/pkg/services/spec_driven_task_service.go` - Remove the validation block (lines ~152-170)

## Testing

1. Select "Continue existing" with a branch that has an active task → task created successfully
2. Verify existing task on that branch is unchanged
3. Verify both tasks work independently