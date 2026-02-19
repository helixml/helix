# Design: Auto-Detect External PRs and Merged Branches for Spec Tasks

## Overview

Extend the spec task orchestrator to detect when:
1. A PR has been opened externally for a task's branch → move to `pull_request` status
2. A task's branch has been merged to main → move to `done` status

This handles cases where PRs are created and/or merged outside the normal Helix workflow.

## Current Architecture

The system already has:
1. `detectAndLinkExistingPR()` - Only runs at task creation with `BranchModeExisting`, looks for open PRs
2. `pollPullRequests()` - Only polls tasks already in `pull_request` status with `PullRequestID` set
3. `handleMainBranchPush()` - Detects merges on internal repos via git hooks, but doesn't help for external repos

**Gaps**:
- Tasks in `spec_review` or `implementation` status are never checked for externally-opened PRs
- Tasks without a `PullRequestID` are never checked for merge status

## Solution

### Approach: Extend the PR Polling Loop

Add a new function `detectExternalPRActivity()` called alongside `pollPullRequests()` that:

1. Lists tasks in `spec_review` or `implementation` status that have a `BranchName` but no `PullRequestID`
2. For each task, checks for open PRs on that branch
3. If open PR found → link it and transition to `pull_request` status
4. If no open PR, check if the branch has been merged to main
5. If merged → transition to `done` status

### Detection Logic

```go
// Pseudocode
for _, task := range tasksWithBranchButNoPR {
    // First: check for open PRs
    prs := gitService.ListPullRequests(ctx, repoID)
    for _, pr := range prs {
        if pr.SourceBranch == task.BranchName && pr.State == "open" {
            task.PullRequestID = pr.ID
            task.Status = TaskStatusPullRequest
            return // done with this task
        }
    }
    
    // Second: check if branch is merged
    isMerged := gitService.IsBranchMerged(ctx, repoID, task.BranchName, defaultBranch)
    if isMerged {
        task.Status = TaskStatusDone
        task.MergedToMain = true
    }
}
```

### Branch Merge Detection Method

Use git history check:
- Check if the branch's HEAD commit exists in main's history
- This works even if the PR was squash-merged or the branch was deleted

**Fallback**: If branch no longer exists, check if `task.LastPushCommitHash` is in main.

## Components Modified

| File | Change |
|------|--------|
| `api/pkg/services/spec_task_orchestrator.go` | Add `detectExternalPRActivity()`, call from `prPollLoop()` |
| `api/pkg/services/git_repository_service.go` | Add `IsBranchMerged()` method if not present |

## Error Handling

- If branch doesn't exist and no `LastPushCommitHash`: log warning, skip (don't break polling)
- If git API call fails: log error, continue to next task
- Rate limit: Process max 10 tasks per poll cycle to avoid API exhaustion

## Testing

1. **Open PR detection**: Create task, open PR externally via ADO/GitHub, verify task moves to `pull_request` column
2. **Merged branch detection**: Create task, merge PR externally, verify task moves to `done`
3. **Edge case**: Branch deleted after merge (should still detect via `LastPushCommitHash`)