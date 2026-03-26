# Design: Prevent Duplicate PR Creation

## Root Cause

The deduplication check in `ensurePullRequestForRepo` (file: `api/pkg/server/spec_task_workflow_handlers.go`, ~line 481) calls `ListPullRequests`, which fetches only **open** PRs from GitHub (`State: "open"` in `api/pkg/agent/skill/github/client.go:174`).

When a user closes a Helix-created PR:
1. The PR disappears from the `ListPullRequests` response.
2. The dedup loop finds no open PR matching the source branch → falls through to `CreatePullRequest`.
3. The task stays in `pull_request` status (the orchestrator logs "All PRs closed, task remains in pull_request status" at line 738–742 but takes no action to stop `ensurePRs`).
4. `handlePullRequest` in the orchestrator calls `ensurePRs` every 30 seconds → creates a new PR every cycle.

The rename scenario (close + reopen with new title) hits the same path: the close creates the duplicate before the reopen is detected.

## Key Files

| File | Role |
|------|------|
| `api/pkg/server/spec_task_workflow_handlers.go` | `ensurePullRequestForRepo` — dedup check and PR creation |
| `api/pkg/services/spec_task_orchestrator.go` | `handlePullRequest` — polling loop that calls `ensurePRs` |
| `api/pkg/agent/skill/github/client.go` | `ListPullRequests` — fetches only `State: "open"` PRs |
| `api/pkg/types/` | `SpecTask.RepoPullRequests` — tracks PR IDs and states |

## Solution

**Primary fix — check tracked PRs before creating:**

In `ensurePullRequestForRepo`, before calling `ListPullRequests`, check `task.RepoPullRequests` for an existing tracked PR for this repo. If one exists, skip creation regardless of its current state (open, closed, or merged). This makes the "has Helix already opened a PR for this repo?" check authoritative and avoids relying on a live GitHub API call whose results depend on PR state filtering.

```
if task already has a RepoPR entry for this repoID:
    fetch the current state of that PR (by ID)
    if closed → return it as-is, do NOT create a new PR
    if open → return it as-is (existing behavior, correct)
    if merged → return it as-is (task will transition to done)
```

**Why not fix the GitHub client to also fetch closed PRs?**

Fetching all PRs (open + closed) to find the right one by branch name is expensive and fragile for repos with many PRs. The `task.RepoPullRequests` list is already the authoritative record of which PRs Helix created — using it is cheaper and more correct.

**Secondary guard — stop ensurePRs when all tracked PRs are closed:**

In `handlePullRequest` (orchestrator), if all entries in `task.RepoPullRequests` are in `closed` state, skip the `o.ensurePRs(...)` call. This prevents the polling loop from attempting PR creation when the user has explicitly closed all PRs.

## Decision

Use both guards together. The primary fix (check `task.RepoPullRequests` first) is the correct invariant: Helix should never open a second PR for a repo that already has a tracked PR. The secondary guard is a belt-and-suspenders defence in the orchestrator polling path.

## Notes for Implementers

- `task.RepoPullRequests` is of type `[]types.RepoPR`. Each entry has `RepositoryID`, `PRID`, `PRState`.
- `PRState` is updated by `processExternalPullRequestStatus` (called from the same polling loop), so by the time `handlePullRequest` calls `ensurePRs`, `PRState` should already reflect the current closed state.
- The fix must preserve the case where `task.RepoPullRequests` is empty (no PR yet created) — in that case, proceed with normal dedup via `ListPullRequests` and creation.
- Azure DevOps and Bitbucket implementations of `ListPullRequests` should be checked for the same open-only filter issue, though the fix via `RepoPullRequests` is provider-agnostic.
