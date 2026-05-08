# Design

## Bug 1: Error message persists after PR merge

### Root Cause
When a task transitions from `pull_request` to `done` (merged), `task.Metadata["error"]` is never cleared. The error may have been set earlier by the 5-minute PR timeout check (`spec_task_orchestrator.go:697-706`). The frontend shows this error prominently in a red banner regardless of task phase (`TaskCard.tsx:1068-1086`), so it appears alongside the green "Task finished" success message.

### Fix
Keep the error in `task.Metadata["error"]` — it's useful for debugging. Instead, fix the frontend to not display the error banner when the task is in completed phase.

In `TaskCard.tsx` (line 1068), add a phase check so the error banner is suppressed when `task.phase === "completed"`. The error data remains in metadata for inspection via the API or database, but the user sees the clean "Task finished — Merged to default branch" message.

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
| `api/pkg/server/spec_task_workflow_handlers.go` | 509-590 | Add early return for already-tracked PRs |
| `frontend/src/components/tasks/TaskCard.tsx` | 1068-1086 | Skip error banner when phase is "completed" |
