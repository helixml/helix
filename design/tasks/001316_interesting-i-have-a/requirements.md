# Requirements: Auto-Detect Merged PRs for Spec Tasks

## Problem Statement

When a spec task has status `spec_review` (or `implementation`), the UI shows an "Open PR" button. However, if a PR was already opened AND merged externally (outside of the normal Helix workflow), the task remains stuck showing "Open PR" instead of being automatically moved to "done" status.

## User Stories

### US1: Merged PR Detection on Task Load
As a user viewing a spec task, when the task's branch has already been merged to main, I want the system to automatically detect this and move the task to "done" status, so I don't see misleading action buttons.

### US2: Periodic Merged Branch Detection
As a user with tasks in `spec_review` or `implementation` status, when a PR gets merged externally (via ADO/GitHub/GitLab UI), I want the system to detect this within 1-2 minutes and automatically complete the task.

## Acceptance Criteria

1. **AC1**: When loading a task in `spec_review` or `implementation` status, the system checks if the task's branch has been merged to main
2. **AC2**: If the branch is merged, the task is automatically transitioned to `done` status with `merged_to_main=true` and `merged_at` set
3. **AC3**: The existing PR polling loop (`pollPullRequests`) is extended to also check for merged branches on tasks without a `PullRequestID`
4. **AC4**: Works for all supported git providers: GitHub, GitLab, Azure DevOps
5. **AC5**: No UI changes required - the existing buttons will disappear once status is "done"

## Out of Scope

- Detecting if branch was deleted without merge (leave task in current state)
- Retroactively fixing historical tasks (only applies to active tasks going forward)