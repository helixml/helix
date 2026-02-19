# Design: Auto-Detect Merged PRs for Spec Tasks

## Overview

Extend the spec task orchestrator to detect when a task's branch has been merged to main, even if no PR was tracked by Helix. This handles cases where PRs are created and merged outside the normal Helix workflow.

## Current Architecture

The system already has:
1. `detectAndLinkExistingPR()` - Only looks for **open** PRs (`pr.State == "active"`)
2. `pollPullRequests()` - Only polls tasks in `pull_request` status with `PullRequestID` set
3. `handleMainBranchPush()` - Detects merges on internal repos via git hooks, but doesn't help for external repos

**Gap**: Tasks in `spec_review` or `implementation` status with no `PullRequestID` are never checked for merge status.

## Solution

### Approach: Extend the PR Polling Loop

Add a new function `detectMergedBranches()` called alongside `pollPullRequests()` that:
1. Lists tasks in `spec_review` or `implementation` status that have a `BranchName`
2. For each task, checks if the branch has been merged to main
3. If merged, transitions task to `done` status

### Key Decision: Branch Merge Detection Method

Use git history check rather than listing PRs:
- Check if the branch's HEAD commit exists in main's history
- This works even if the PR was squash-merged or the branch was deleted

```go
// Pseudocode
commits, _ := gitService.GetBranchCommits(ctx, repoID, task.BranchName, 1)
if len(commits) > 0 {
    isMerged := gitService.IsCommitInBranch(ctx, repoID, commits[0].SHA, repo.DefaultBranch)
    if isMerged {
        transitionToDone(task)
    }
}
```

**Fallback**: If branch no longer exists, check if `task.LastPushCommitHash` is in main.

## Components Modified

| File | Change |
|------|--------|
| `api/pkg/services/spec_task_orchestrator.go` | Add `detectMergedBranches()`, call from `prPollLoop()` |
| `api/pkg/services/git_repository_service.go` | Add `IsCommitInBranch()` method if not present |

## Error Handling

- If branch doesn't exist and no `LastPushCommitHash`: log warning, skip (don't break polling)
- If git API call fails: log error, continue to next task
- Rate limit: Process max 10 tasks per poll cycle to avoid API exhaustion

## Testing

1. Create task with branch, merge branch externally via ADO/GitHub
2. Wait for poll cycle (30 seconds)
3. Verify task transitions to `done` with `merged_to_main=true`
