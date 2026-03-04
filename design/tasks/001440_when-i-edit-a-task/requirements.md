# Requirements: Fix Task Description Not Being Used When Launching

## Problem Statement

When a user edits a task description before launching it, the task gets launched with the old description instead of the edited one.

## Root Cause Analysis

Two separate bugs were identified:

### Bug 1: BacklogTableView Uses Wrong Field

In `BacklogTableView.tsx`, when editing a task's prompt:
- **Load**: `setEditingPrompt(task.original_prompt || "")` - reads from `original_prompt`
- **Display**: `{task.original_prompt || "(No prompt)"}` - displays `original_prompt`
- **Save**: `updates: { description: editingPrompt }` - saves to `description`

The fields `original_prompt` and `description` are different:
- `original_prompt` - Immutable field set at task creation
- `description` - User-editable field that should be used for display and editing

This causes:
1. User edits and saves → `description` updated to "New text"
2. User clicks edit again → loads `original_prompt` ("Old text")
3. User sees old text, may inadvertently save old text back

### Bug 2: Display Shows Wrong Field

The display also shows `original_prompt` instead of `description`, confusing users about what text will be used.

## User Stories

### US-1: Edit Task Description in Backlog Table
**As a** user editing a task in the backlog table view  
**I want** to see and edit the current description  
**So that** my changes persist correctly

**Acceptance Criteria:**
- [ ] Clicking to edit loads `description` (falling back to `original_prompt` if empty)
- [ ] The table displays `description` (falling back to `original_prompt` if empty)
- [ ] Saved changes persist when re-opening the edit

### US-2: Consistent Description Display
**As a** user viewing tasks  
**I want** to see the same description everywhere  
**So that** I know what text will be used when the task launches

**Acceptance Criteria:**
- [ ] All UI locations show `description` with fallback to `original_prompt`
- [ ] Editing in any location shows the same text

## Affected Files

- `helix/frontend/src/components/tasks/BacklogTableView.tsx`

## Out of Scope

- Changing how `original_prompt` vs `description` work on the backend (this is correct)
- Adding edit functionality to other views (already working in SpecTaskDetailContent)