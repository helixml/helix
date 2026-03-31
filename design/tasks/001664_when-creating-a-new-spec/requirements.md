# Requirements: Immediate Cache Invalidation on Spec Task Creation

## User Story

As a user creating a new spec task, I want the task to appear in the list immediately after submission, so that I know my input was saved and haven't lost my work.

## Problem

After submitting a new spec task, the task does not appear in the Kanban/list view for ~5 seconds. During this gap, it looks like the submission failed or the task vanished — eroding confidence right after the user invested effort in writing the description.

**Root cause:** Cache invalidation in `NewSpecTaskForm.tsx` (line 348) is called after all label mutations complete. If label mutations are slow or the invalidation doesn't fire (e.g., component unmounts before reaching that line), the task list only refreshes on the next 10-second polling cycle — averaging ~5 seconds of invisible new task.

## Acceptance Criteria

- [ ] The new task appears in the task list within ~1 second of the API confirming creation
- [ ] Cache is invalidated immediately after `v1SpecTasksFromPromptCreate` succeeds, not after label mutations
- [ ] The behavior is consistent whether or not labels are selected
- [ ] Existing functionality (labels being added, form closing) is not broken
