# Requirements: Auto-Open PR on Approval When Commits Exist

## Problem Statement

When a user approves an implementation, the system sends a message to the agent asking it to push a commit (even an empty one) to trigger PR creation. Sometimes the agent forgets to do this, leaving the PR unopened.

## User Stories

### US-1: Automatic PR creation when commits exist
As a project owner, when I approve an implementation that already has commits on the feature branch, I want the PR to be opened immediately without waiting for the agent to push.

## Acceptance Criteria

### AC-1: Auto-PR on approval
- [ ] When `approveImplementation` is called and the task status moves to `pull_request`
- [ ] Check if the feature branch already has commits ahead of the default branch
- [ ] If commits exist, immediately call `ensurePullRequest` to create the PR
- [ ] Still send the agent message (as fallback for edge cases)

### AC-2: No change when no commits
- [ ] If the feature branch has no commits ahead of default, behave as before (wait for agent push)

### AC-3: PR URL returned immediately
- [ ] When PR is auto-created, return the `PullRequestURL` in the API response