# Requirements: Allow Move to Backlog from Pull Request and Done States

## Context

Currently, tasks in `pull_request` or `done` status cannot be moved back to backlog. The "Move to Backlog" button is hidden for these states. Users need this when:

- A PR is open but the approach is wrong and needs rethinking
- A task was marked done/merged but later found to not actually work
- A merged task needs follow-up work (may result in another PR from the same branch)

## User Stories

1. **As a project manager**, I want to move a task from `pull_request` back to backlog so I can rethink the approach or pause work on it.

2. **As a project manager**, I want to move a `done`/merged task back to backlog when I discover it didn't actually work or needs more work, so the task re-enters the workflow.

## Acceptance Criteria

1. **Move to Backlog from Pull Request**
   - The "Move to Backlog" button/menu item appears for tasks in `pull_request` status
   - Clicking it sets status to `backlog` (same as existing move-to-backlog behavior)
   - Agent is stopped if running (existing `useMoveToBacklog` already handles this)

2. **Move to Backlog from Done/Merged**
   - The "Move to Backlog" button/menu item appears for tasks in `done` status
   - Clicking it sets status to `backlog`
   - The task reappears in the Backlog column on the Kanban board

3. **No Backend Changes Required**
   - The backend `updateSpecTask` handler already accepts any status value without transition validation
   - Only frontend guard logic needs updating

4. **Existing Behavior Preserved**
   - Archived tasks still cannot be moved to backlog
   - Queued tasks (`queued_implementation`, `queued_spec_generation`, `spec_approved`) still cannot be moved to backlog (these are mid-transition)
   - Tasks already in `backlog` don't show the button