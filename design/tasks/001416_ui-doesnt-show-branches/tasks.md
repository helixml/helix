# Implementation Tasks

## Backend: Data Model

- [x] Add `RepoPR` struct to `api/pkg/types/simple_spec_task.go`
- [x] Add `RepoPullRequests` field to `SpecTask` struct (JSON column)
- [x] Update `SpecTask` JSON serialization to include new field

## Backend: PR Creation Logic

- [x] Refactor `ensurePullRequestForTask()` in `spec_task_workflow_handlers.go` to accept repo parameter
- [x] Create `ensurePullRequestsForAllRepos()` that iterates project repos
- [x] Update PR creation to store result in `RepoPullRequests` array
- [x] Backfill deprecated `PullRequestID`/`PullRequestURL` from primary repo for compat

## Backend: Push Detection

- [x] Update `handleFeatureBranchPush()` in `git_http_server.go` to trigger PR creation for pushed repo
- [x] Ensure non-primary repo pushes also trigger PR workflow

## API Updates

- [x] Ensure `GetSpecTask` returns `repo_pull_requests` in response
- [x] Update OpenAPI spec with new field
- [x] Run `./stack update_openapi` to regenerate client

## Frontend: UI Updates

- [x] Update `SpecTaskForActions` interface in `SpecTaskActionButtons.tsx` with `repo_pull_requests`
- [x] Modify "View Pull Request" button to show dropdown when multiple PRs exist
- [x] Display repo name + PR number for each entry
- [x] Handle single-PR case (no dropdown, same as current behavior)

## Testing

- [x] Verify Go code compiles: `go build ./pkg/server/ ./pkg/services/ ./pkg/types/`
- [x] Verify frontend builds: `cd frontend && yarn build`
- [ ] Manual test: create multi-repo project and verify PRs appear in UI