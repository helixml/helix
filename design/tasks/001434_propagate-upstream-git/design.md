# Design: Propagate Upstream Git Push Errors

## Context

In `api/pkg/services/git_http_server.go`, the upstream push to GitHub happens **after** the HTTP 200 response has already been sent to the git client. The code correctly rolls back the local ref on failure, but the error is only logged—it's never persisted or surfaced to users.

## Existing Patterns

The codebase already has a similar pattern for clone failures:
- `GitRepository.CloneError` and `GitRepository.Status = error` are set when async clone fails
- The error is cleared on success

We'll follow the same pattern for push failures.

## Data Model Changes

### GitRepository (types/git_repositories.go)

Add two fields:
```go
LastPushError   string     `json:"last_push_error,omitempty"`
LastPushErrorAt *time.Time `json:"last_push_error_at,omitempty"`
```

The existing `Status` field already supports `GitRepositoryStatusError`.

### SpecTask (types/simple_spec_task.go)

Add two fields:
```go
LastUpstreamPushError   string     `json:"last_upstream_push_error,omitempty"`
LastUpstreamPushErrorAt *time.Time `json:"last_upstream_push_error_at,omitempty"`
```

## Implementation Location

The fix is localized to `handleReceivePack()` in `git_http_server.go`, specifically the `if upstreamPushFailed` block (~line 624).

### On Failure

After `rollbackBranchRefs()`:
1. Update `GitRepository` with error info and status
2. Find `SpecTask(s)` by branch name (reuse existing task-lookup pattern from `handleFeatureBranchPush`)
3. Update each matching task with the error

### On Success

After the successful push loop:
1. Clear `LastPushError`/`LastPushErrorAt` on `GitRepository`
2. Reset `Status` to `active` (if currently `error`)
3. Find and clear errors on associated `SpecTask(s)`

## Task Lookup Strategy

Reuse the existing pattern from `handleFeatureBranchPush`:
```go
projectIDs, err := s.store.GetProjectsForRepository(ctx, repo.ID)
for _, projectID := range projectIDs {
    tasks, _ := s.store.ListSpecTasks(ctx, &types.SpecTaskFilters{ProjectID: projectID})
    // filter by task.BranchName matching pushed branch
}
```

## Context Handling

Since HTTP headers are already sent, we use `context.Background()` with a timeout—consistent with the existing `branchCtx` pattern used for the push itself.

## API Exposure

No new endpoints needed. The error fields will automatically appear in:
- `GET /api/v1/git/repositories/{id}` — via `GitRepository` struct
- `GET /api/v1/spec-tasks/{id}` — via `SpecTask` struct

## Database Migration

GORM AutoMigrate handles the new columns automatically—no manual migration needed.

## Key Decision: Error Message Format

Store the raw error from `PushBranchToRemote()`. It already contains human-readable context like:
```
failed to push to external repository: remote: Permission to org/repo.git denied to app/helix-control-plane
```
