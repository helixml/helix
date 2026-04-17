# Design

## Bug 1: Error message persists after PR merge

### Root Cause
When a task transitions from `pull_request` to `done` (merged), `task.Metadata["error"]` is never cleared. The error may have been set earlier by the 5-minute PR timeout check (`spec_task_orchestrator.go:697-706`). The frontend shows this error prominently in a red banner regardless of task phase (`TaskCard.tsx:1068-1086`), so it appears alongside the green "Task finished" success message.

### Fix
Clear `task.Metadata["error"]` in all three code paths that transition a task to `done` status:

1. **`processExternalPullRequestStatus()`** (`spec_task_orchestrator.go:771-792`) — when all tracked PRs are merged
2. **`checkTaskForExternalPRActivity()`** (`spec_task_orchestrator.go:1056-1078`) — when an externally-merged PR is detected
3. **Branch-merge fallback** (`spec_task_orchestrator.go:841-849`) — when the branch is detected as merged to main without a tracked PR

In each path, before saving the task, add:
```go
if task.Metadata != nil {
    delete(task.Metadata, "error")
}
```

Additionally, the frontend should not display the error banner when the task is in completed phase, as a defensive measure against stale metadata.

## Bug 2: Duplicate PR creation when PR is closed/deleted

### Root Cause
The GitHub client's `ListPullRequests` (`api/pkg/agent/skill/github/client.go:175`) only fetches PRs with `State: "open"`. When `ensurePullRequestForRepo()` (`spec_task_workflow_handlers.go:557-590`) checks whether a PR already exists for the branch, it calls `ListPullRequests` — which doesn't return closed PRs. The closed-PR guard at line 577-589 never fires because the closed PR isn't in the result set. The function falls through and creates a new PR.

This happens via the orchestrator's `handlePullRequest()` → `ensurePRs()` call path, which runs every 30 seconds for tasks in `pull_request` status.

### Fix
Add an early return in `ensurePullRequestForRepo()` when the task already tracks a PR for this repository. Before calling `ListPullRequests`, check `task.RepoPullRequests` for an existing entry for the same repo. If found and the state is not empty (meaning we've seen this PR before), return the existing `RepoPR` without creating a new one.

This is safer than changing `ListPullRequests` to return all states (which would return potentially hundreds of old PRs for active repos).

```go
// In ensurePullRequestForRepo, before calling ListPullRequests:
for _, existing := range task.RepoPullRequests {
    if existing.RepositoryID == repo.ID && existing.PRID != "" {
        // Already tracked — don't recreate. Covers closed, merged, and open states.
        return &existing, nil
    }
}
```

This check ensures that once a PR is tracked for a repo, we never create a duplicate. The existing `processExternalPullRequestStatus` polling will continue to monitor the PR's actual state via `GetPullRequest` (which fetches by ID, not by listing).

## Key Files

| File | Lines | What to change |
|------|-------|---------------|
| `api/pkg/services/spec_task_orchestrator.go` | 771-792, 841-849, 1056-1078 | Clear metadata error on all done transitions |
| `api/pkg/server/spec_task_workflow_handlers.go` | 509-590 | Add early return for already-tracked PRs |
| `frontend/src/components/tasks/TaskCard.tsx` | 1068-1086 | Skip error banner when phase is "completed" |
