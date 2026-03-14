# Design: Multi-Repo Pull Request Tracking

## Current State

`SpecTask` has a single `PullRequestID string` and a computed `PullRequestURL string`. The `ensurePullRequest` function in `git_http_server.go` only persists `PullRequestID` for the project's **default repo** (by explicit design comment) to avoid false merge detection on secondary repos. The orchestrator's merge detection (`processExternalPullRequestStatus`, `handleMainBranchPush`) always queries `project.DefaultRepoID`.

Key files:
- `api/pkg/types/simple_spec_task.go` — SpecTask struct
- `api/pkg/services/git_http_server.go` — `ensurePullRequest`, `handleFeatureBranchPush`
- `api/pkg/services/spec_task_orchestrator.go` — `processExternalPullRequestStatus`, `handlePullRequest`, `pollPullRequests`, `detectExternalPRActivity`
- `api/pkg/services/git_repository_service.go` — `GetPullRequestURL`
- `frontend/src/components/tasks/SpecTaskActionButtons.tsx` — PR button display
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — task detail panel

## Data Model Changes

Add a new struct and a JSONB column on `SpecTask`:

```go
// types/simple_spec_task.go

type RepoPREntry struct {
    RepoID    string     `json:"repo_id"`
    RepoName  string     `json:"repo_name"`   // display name, denormalized
    PRID      string     `json:"pr_id"`
    PRURL     string     `json:"pr_url"`
    State     string     `json:"state"`       // "open" | "merged" | "closed"
    MergedAt  *time.Time `json:"merged_at,omitempty"`
}

// On SpecTask:
RepoPRs []RepoPREntry `json:"repo_prs,omitempty" gorm:"type:jsonb;serializer:json"`
```

The existing `PullRequestID` / `PullRequestURL` fields are kept unchanged for backward compatibility. New code writes to `RepoPRs`; old single-repo fields remain as the fallback.

No database migration is needed beyond adding the nullable JSONB column.

## Backend Changes

### 1. Track repo on feature branch push (`handleFeatureBranchPush`)

When an agent pushes to a feature branch on **any** repo (not just the default), upsert a `RepoPREntry` for that repo into `task.RepoPRs` with `PRID = ""` (placeholder until PR is created). This records that the repo is "in scope" for this task.

### 2. Create/update PRs for all tracked repos (`ensurePullRequest`)

Change the function signature/call sites so it is invoked once per tracked repo (not just the default repo). For each repo in `task.RepoPRs`:
- Push branch to that repo's remote
- Check for an existing open PR; if found, update the entry
- If not found, create a PR and update the entry with the new PR id/url/state

Remove the guard that skips storing `PullRequestID` for non-default repos (that guard only existed because there was no per-repo storage).

### 3. Merge detection checks all repos (`processExternalPullRequestStatus`, `handleMainBranchPush`)

Replace the current single-repo check with a loop over `task.RepoPRs`:
- For each entry with a `PRID`, fetch the PR state from the external repo API
- Update `entry.State` and `entry.MergedAt` in the slice
- Only set `task.Status = done` when **all** entries are either `merged` or (no PR, branch merged via git)

`handleMainBranchPush` (internal repos): same logic — mark that repo's entry as `merged`, then check all entries before transitioning to `done`.

`detectExternalPRActivity`: extend to iterate all repos in `RepoPRs`, not just the default repo.

### 4. API response

Expose `RepoPRs []RepoPREntry` in the existing task JSON response. The computed `PullRequestURL` field can remain for old clients; new clients use `RepoPRs`.

## Frontend Changes

### SpecTaskActionButtons / SpecTaskDetailContent

When `task.repo_prs` is non-empty, render a list instead of a single button:

```
GitHub - helix/helix         [View PR #42]  ✓ Merged
GitHub - helix/zed           [View PR #17]  ⏳ Open
GitHub - helix/qwen-code     [View PR #5]   ⏳ Open
```

Each row: repo name, PR link (opens in new tab), status chip (Merged / Open / Closed).

Fall back to the existing single-button behaviour when `repo_prs` is empty (backward compat).

## Backward Compatibility

- Old tasks: `RepoPRs` is null/empty → existing code paths unchanged, `PullRequestID` still drives merge detection
- New tasks created after deploy: `RepoPRs` is populated; old fields still set for the default repo for safety
- No data migration required; the JSONB column defaults to null

## Risks / Constraints

- Rate limiting on external PR API calls increases with more repos per task; the existing 10-tasks-per-poll cap may need to account for tasks with many repos
- `ensurePullRequest` is currently called synchronously during implementation approval; calling it for N repos adds latency — consider making it async or parallelising the N remote pushes/PR creations
