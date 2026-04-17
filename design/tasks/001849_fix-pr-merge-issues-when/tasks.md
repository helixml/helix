# Implementation Tasks

## Bug 1: Hide error banner on completed tasks

- [x] In `TaskCard.tsx` (`frontend/src/components/tasks/TaskCard.tsx:1068`), add `task.phase !== "completed"` condition to the error banner so it is hidden when the task is merged. Keep the error in metadata for debugging.

## Bug 2: Prevent duplicate PR creation

- [x] In `ensurePullRequestForRepo()` (`api/pkg/server/spec_task_workflow_handlers.go`), add an early return before `ListPullRequests` when `task.RepoPullRequests` already contains an entry for the repo with a non-empty PRID

## Verification

- [x] Build Go backend: `go build ./pkg/server/ ./pkg/types/ ./pkg/services/` (full `go build ./...` has pre-existing ollama dependency issue)
- [x] Build frontend: TypeScript type-check passes (full `yarn build` has pre-existing dist/ permission issue)
- [ ] Test scenario: create a spec task, let it create a PR, merge the PR, verify the task card shows "Task finished" without an error banner
- [ ] Test scenario: create a spec task, let it create a PR, close the PR on GitHub, verify no duplicate PR is created on the next poll cycle
