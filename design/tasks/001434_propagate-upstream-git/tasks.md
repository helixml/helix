# Implementation Tasks

## Type Changes

- [ ] Add `LastPushError string` and `LastPushErrorAt *time.Time` fields to `GitRepository` struct in `api/pkg/types/git_repositories.go`
- [ ] Add `LastUpstreamPushError string` and `LastUpstreamPushErrorAt *time.Time` fields to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`

## Core Logic

- [ ] In `handleReceivePack()` (`api/pkg/services/git_http_server.go`), after `rollbackBranchRefs` in the `upstreamPushFailed` block (~line 620):
  - Update the `GitRepository` with error message, timestamp, and `Status = GitRepositoryStatusError`
  - Find SpecTask(s) by branch (reuse pattern from `handleFeatureBranchPush`)
  - Update each SpecTask with error message and timestamp

- [ ] In `handleReceivePack()`, after successful upstream push loop:
  - Clear `GitRepository.LastPushError` / `LastPushErrorAt`
  - Set `GitRepository.Status = GitRepositoryStatusActive`
  - Clear `SpecTask.LastUpstreamPushError` / `LastUpstreamPushErrorAt` for affected tasks

## Testing

- [ ] Manual test: Remove GitHub write access, push via agent, verify error appears in `GET /api/v1/git/repositories/{id}` response
- [ ] Manual test: Restore access, push again, verify error fields are cleared and status is `active`
- [ ] Manual test: Verify `GET /api/v1/spec-tasks/{id}` shows the error on the affected task
