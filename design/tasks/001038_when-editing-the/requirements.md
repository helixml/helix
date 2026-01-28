# Requirements: Spec Task Description Edit Bug

## Problem Statement

When a user edits the description of a spec task and then starts the task, the edited description is not sent to the agent. The original description is used instead.

## User Story

As a user editing a spec task before starting it, I want my edited description to be used when the agent starts working, so that the agent receives my corrected/updated instructions.

## Root Cause Analysis

**Discovered in codebase:**
- Tasks have two fields: `Description` (user-editable) and `OriginalPrompt` (immutable original)
- When a task is created in `CreateTaskFromPrompt()`, both fields are set to the same prompt value
- When user edits via frontend, `handleSaveEdit()` calls `updateSpecTask.mutateAsync()` which updates `Description`
- When starting planning, `StartSpecGeneration()` builds the prompt using `task.OriginalPrompt`, ignoring `Description`
- Same issue in `StartJustDoItMode()` - uses `OriginalPrompt` for the agent prompt

**Files involved:**
- `api/pkg/services/spec_driven_task_service.go` - Lines 400-401 use `task.OriginalPrompt`
- `api/pkg/services/spec_driven_task_service.go` - Line 637 uses `task.OriginalPrompt` in Just Do It mode
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` - Lines 543-548 correctly update `Description`

## Acceptance Criteria

1. When a user edits a spec task description and starts planning, the edited description is sent to the agent
2. When a user edits a spec task description and starts in "Just Do It" mode, the edited description is sent to the agent
3. The `OriginalPrompt` field remains immutable for audit/history purposes
4. Tasks created without edits continue to work (Description == OriginalPrompt)