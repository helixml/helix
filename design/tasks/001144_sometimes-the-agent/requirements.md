# Requirements: Auto-Open PR on Approval When Commits Exist

## Problem Statement

When a user approves an implementation, the system sends a message to the agent asking it to push a commit (even an empty one) to trigger PR creation. This has two problems:
1. Sometimes the agent forgets to push, leaving the PR unopened
2. The empty commit is unnecessary if commits already exist on the branch

## User Stories

### US-1: Automatic PR creation when commits already pushed
As a project owner, when I approve an implementation that already has commits pushed to the feature branch, I want the PR to be opened immediately without requiring an empty commit from the agent.

### US-2: Agent prompted to push uncommitted work
As a project owner, when I approve an implementation, I want the agent to be reminded to commit and push any remaining uncommitted changes (but not told to make an empty commit).

## Acceptance Criteria

### AC-1: Auto-PR on approval when commits exist
- [ ] When `approveImplementation` is called and the feature branch has commits ahead of default
- [ ] Immediately create the PR (don't wait for agent)
- [ ] Return `PullRequestURL` in the API response

### AC-2: Agent message updated
- [ ] Always send a message to the agent to commit and push any uncommitted changes
- [ ] Remove the instruction about empty commits - it's no longer needed
- [ ] Message should say "commit and push any remaining changes" not "push empty commit to trigger PR"

### AC-3: No commits scenario
- [ ] If feature branch has no commits ahead, still send the agent message
- [ ] PR will be created when agent pushes (existing behavior via `handleFeatureBranchPush`)