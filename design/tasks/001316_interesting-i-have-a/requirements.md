# Requirements: Auto-Detect External PRs and Merged Branches for Spec Tasks

## Problem Statement

When a spec task has status `spec_review` (or `implementation`), the UI shows an "Open PR" button. However, if a PR was already opened or merged externally (outside of the normal Helix workflow), the task remains stuck showing "Open PR" instead of being automatically moved to the right state.

## User Stories

### US1: Open PR Detection
As a user with a task in `spec_review` or `implementation` status, when a PR is opened externally for the task's branch (via ADO/GitHub/GitLab UI), I want the system to detect this within 1-2 minutes and move the task to `pull_request` status.

### US2: Merged Branch Detection
As a user with a task in `spec_review` or `implementation` status, when the task's branch gets merged to main externally, I want the system to detect this within 1-2 minutes and automatically complete the task.

## Acceptance Criteria

1. **AC1**: The system periodically checks tasks in `spec_review` or `implementation` status that have a `BranchName` but no `PullRequestID`
2. **AC2**: If an open PR is found for the branch, the task is transitioned to `pull_request` status with `PullRequestID` and `PullRequestURL` set
3. **AC3**: If the branch is merged (no open PR), the task is transitioned to `done` status with `merged_to_main=true` and `merged_at` set
4. **AC4**: Works for all supported git providers: GitHub, GitLab, Azure DevOps
5. **AC5**: No UI changes required - the existing buttons will update based on new status

## Out of Scope

- Detecting if branch was deleted without merge (leave task in current state)
- Retroactively fixing historical tasks (only applies to active tasks going forward)