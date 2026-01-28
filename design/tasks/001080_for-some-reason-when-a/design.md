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