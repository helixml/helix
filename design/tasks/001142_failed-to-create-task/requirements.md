# Requirements: Continue Existing Branch Should Resume Active Task

## Problem Statement

When a user selects "Continue existing" / "Resume work on a branch" in the new task form, the system incorrectly tries to create a **new** task and then fails because the branch already has an active task.

**Current error:**
```
failed to create task: branch 'feature/001124-fix-the-project-startup' already has an active task: 
Fix the project startup script at /home/retro/work/helix-... (spt_01kg43ybqf95mmfd65df9b8fvv). 
Complete or archive that task first, or create a new branch
```

**Expected behavior:** The system should detect the existing active task and navigate/redirect the user to it, rather than attempting to create a new task.

## User Stories

### US1: Resume Work on Existing Task
**As a** developer  
**I want to** select a branch with an existing task and continue working on it  
**So that** I can pick up where I left off without remembering the task ID

### US2: Clear Feedback When Resuming
**As a** developer  
**I want to** see a message that I'm being redirected to the existing task  
**So that** I understand what's happening

## Acceptance Criteria

### AC1: Detect and Redirect to Existing Task
- [ ] When user selects "Continue existing" mode with a branch that has an active task
- [ ] System should find the existing active task instead of failing
- [ ] User should be redirected to that task's detail page
- [ ] A toast/notification should inform the user: "Resuming existing task: {task name}"

### AC2: Only Create New Task When Branch Has No Active Task
- [ ] If branch exists but has no active task (all tasks completed/archived), create new task
- [ ] If branch has never been used by any task, create new task
- [ ] New task creation should work as before in these cases

### AC3: Handle Edge Case - Multiple Inactive Tasks
- [ ] If branch has multiple completed/archived tasks, creating a new task should still work
- [ ] The validation should only block when there's an **active** task

## Out of Scope

- Changing how "Start fresh" (new branch) mode works
- Adding ability to work on same branch with multiple active tasks
- Modifying branch selection UI