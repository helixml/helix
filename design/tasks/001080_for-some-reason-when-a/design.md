# Design: Cloned Task Spec Review Button Fix

## Overview

Fix the issue where cloned tasks in `spec_review` status don't show the "Review Spec" button because `DesignDocsPushedAt` is not set during cloning.

## Current Flow (Broken)

1. User clones task with specs to target project(s)
2. `cloneTaskToProject()` creates new task with:
   - Copied specs (requirements, design, implementation plan)
   - Status: `queued_spec_generation` (if autoStart)
   - `DesignDocsPushedAt`: **nil** (not copied)
3. Orchestrator picks up task, sees specs exist, moves to `spec_review`
4. UI checks `task.status === 'spec_review' && task.design_docs_pushed_at` → **false** (nil)
5. "Review Spec" button doesn't show

## Solution

When cloning a task that has existing specs, we should:

1. **Set `DesignDocsPushedAt`** to current time on the cloned task
2. **Set initial status to `spec_review`** (skip spec generation since specs already exist)
3. **Create a design review record** so the review UI has data to display

## Implementation Approach

### Option A: Set DesignDocsPushedAt in cloneTaskToProject (Recommended)

Modify `api/pkg/server/spec_task_clone_handlers.go`:

```go
// In cloneTaskToProject(), after creating newTask:
if requirementsSpec != "" && technicalDesign != "" && implementationPlan != "" {
    now := time.Now()
    newTask.DesignDocsPushedAt = &now
    newTask.Status = types.TaskStatusSpecReview  // Skip to review since specs exist
}
```

**Pros:**
- Simple, minimal change
- Fixes the immediate problem
- Cloned tasks go directly to review (correct behavior)

**Cons:**
- Design review record not created (handled by self-healing in `listDesignReviews`)

### Option B: Also Create Design Review Record

Additionally call a helper to create the design review:

```go
// After creating the task
if newTask.DesignDocsPushedAt != nil {
    s.createDesignReviewForClonedTask(ctx, newTask)
}
```

**Pros:**
- Review record ready immediately
- No reliance on self-healing

**Cons:**
- More code
- Self-healing already handles this case

## Chosen Approach

**Option A** - Set `DesignDocsPushedAt` and status in `cloneTaskToProject()`.

The existing self-healing code in `listDesignReviews()` will create the design review record when the user first opens the review UI. This is already tested and works.

## Key Files to Modify

1. `api/pkg/server/spec_task_clone_handlers.go`
   - `cloneTaskToProject()` function
   - Set `DesignDocsPushedAt` when specs exist
   - Set status to `spec_review` instead of `queued_spec_generation`

## Testing

1. Clone a completed task (with specs) to a new project with autoStart=true
2. Verify cloned task shows "Review Spec" button immediately
3. Click "Review Spec" and verify design review loads correctly
4. Clone same task with autoStart=false, then manually start it
5. Verify it goes through spec generation properly (no specs should be skipped)

## Implementation Notes

### Changes Made

**File: `api/pkg/server/spec_task_clone_handlers.go`**

Added logic in `cloneTaskToProject()` to detect when source task has specs and handle accordingly:

```go
// Check if source task has specs - if so, we can skip spec generation
hasSpecs := source.RequirementsSpec != "" && source.TechnicalDesign != "" && source.ImplementationPlan != ""

if autoStart {
    if source.JustDoItMode {
        initialStatus = types.TaskStatusQueuedImplementation
    } else if hasSpecs {
        // Source has specs, skip directly to spec_review
        initialStatus = types.TaskStatusSpecReview
        now := time.Now()
        designDocsPushedAt = &now
    } else {
        initialStatus = types.TaskStatusQueuedSpecGeneration
    }
}
```

Also added `DesignDocsPushedAt: designDocsPushedAt` to the newTask struct.

### Key Insight

The existing self-healing code in `listDesignReviews()` (spec_task_design_review_handlers.go:109-115) will automatically create a design review record when the user opens the review UI, so no additional work needed for design review creation.

### Unit Tests Added

Created `spec_task_clone_handlers_test.go` with 4 tests:
1. `TestCloneTaskToProject_WithSpecs_SetsDesignDocsPushedAt` - verifies fix works
2. `TestCloneTaskToProject_WithoutSpecs_DoesNotSetDesignDocsPushedAt` - no regression for tasks without specs
3. `TestCloneTaskToProject_JustDoItMode_SkipsSpecReview` - JustDoItMode still goes to implementation
4. `TestCloneTaskToProject_AutoStartFalse_GoesToBacklog` - autoStart=false still goes to backlog