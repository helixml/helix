# Design: Continue Existing Branch Should Resume Active Task

## Overview

This is a bug fix in the task creation flow. When a user selects "Continue existing" mode and picks a branch that already has an active task, the system should redirect them to that task instead of failing with an error.

## Current Architecture

**Flow today (broken):**
1. User selects "Continue existing" mode
2. User picks a branch (e.g., `feature/001124-fix-the-project-startup`)
3. Frontend calls `POST /api/v1/spec-tasks` with `branch_mode: "existing"` and `working_branch: "feature/..."`
4. Backend checks for active tasks on that branch
5. Backend returns error: "branch already has an active task"
6. User sees error and is stuck

**Code location:** `api/pkg/services/spec_driven_task_service.go` lines 152-170

## Proposed Solution

Change the behavior from "error on conflict" to "redirect to existing task".

### Option A: Backend Returns Existing Task (Recommended)

Instead of returning an error, return the existing task with a flag indicating it was found rather than created.

**Backend changes (`spec_driven_task_service.go`):**
```go
// In CreateTaskFromPrompt, when we find an active task on the branch:
if !isTaskInactive(existingTask) {
    // Return the existing task instead of error
    return existingTask, nil  // Or wrap in a response struct with "found_existing: true"
}
```

**Frontend changes (`NewSpecTaskForm.tsx` and `SpecTasksPage.tsx`):**
- Check response for existing task indicator
- Show toast: "Resuming existing task: {task name}"
- Navigate to task detail page

### Option B: Frontend Pre-Check (Alternative)

Frontend queries for active tasks on the branch before submitting.

**Pros:** Simpler backend, no API change  
**Cons:** Extra API call, race condition possible

### Decision: Option A

Option A is cleaner - single API call, atomic behavior, no race conditions.

## API Changes

**Response change for `POST /api/v1/spec-tasks`:**

Current response: `SpecTask` object

New response: Same `SpecTask` object, but add a field:
```json
{
  "id": "spt_...",
  "name": "Fix the project startup",
  "found_existing": true,  // NEW: indicates this was an existing task, not newly created
  ...
}
```

**Alternative:** Return HTTP 200 with existing task vs 201 for new task. Frontend checks status code.

## UI Changes

1. **Success toast:** "Resuming existing task: {task.name}"
2. **Navigation:** Redirect to `/projects/{projectId}/tasks/{taskId}` 
3. **No form reset needed:** User is navigating away anyway

## Testing

1. Select "Continue existing" with branch that has active task → redirects to task
2. Select "Continue existing" with branch that has only completed tasks → creates new task
3. Select "Continue existing" with branch never used → creates new task
4. Select "Start fresh" → creates new branch and task (unchanged)

## Files to Modify

- `api/pkg/services/spec_driven_task_service.go` - Return existing task instead of error
- `api/pkg/types/simple_spec_task.go` - Add `FoundExisting` field to response (optional)
- `frontend/src/components/tasks/NewSpecTaskForm.tsx` - Handle redirect
- `frontend/src/pages/SpecTasksPage.tsx` - Handle redirect (same form exists here too)