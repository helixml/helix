# Requirements: Allow Multiple Tasks on Same Branch

## Problem Statement

When a user selects "Continue existing" / "Resume work on a branch", the system blocks task creation if there's already an active task on that branch. This is overly restrictive - there are legitimate reasons to create multiple tasks on the same branch (e.g., different aspects of work, changed requirements, starting fresh with a new agent conversation).

**Current error:**
```
failed to create task: branch 'feature/001124-fix-the-project-startup' already has an active task: 
Fix the project startup script at /home/retro/work/helix-... (spt_01kg43ybqf95mmfd65df9b8fvv). 
Complete or archive that task first, or create a new branch
```

**Expected behavior:** Allow the user to create a new task on the branch. Multiple tasks can coexist on the same branch.

## User Stories

### US1: Create New Task on Branch With Existing Task
**As a** developer  
**I want to** create a new task on a branch that already has an active task  
**So that** I can start fresh or work on a different aspect without archiving my previous work

### US2: Resume Work With New Context
**As a** developer  
**I want to** continue on an existing branch with a new prompt/task  
**So that** I can provide updated requirements or start a new conversation with the agent

## Acceptance Criteria

### AC1: Remove Branch-Task Uniqueness Validation
- [ ] When user selects "Continue existing" mode with a branch that has an active task
- [ ] System should create the new task without error
- [ ] The new task should be associated with the selected branch
- [ ] Existing tasks on that branch remain unchanged

### AC2: Multiple Active Tasks Per Branch Allowed
- [ ] A branch can have multiple active tasks simultaneously
- [ ] Each task maintains its own state (status, specs, session, etc.)
- [ ] Tasks are independent - completing/archiving one doesn't affect others

## Out of Scope

- Merging or linking related tasks on the same branch
- Warning users about existing tasks (they chose "Continue existing" deliberately)
- Changing task list filtering or display