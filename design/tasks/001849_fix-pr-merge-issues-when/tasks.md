# Implementation Tasks

## Bug 1: Hide error banner on completed tasks

- [x] In `TaskCard.tsx` (`frontend/src/components/tasks/TaskCard.tsx:1068`), add `task.phase !== "completed"` condition to the error banner so it is hidden when the task is merged. Keep the error in metadata for debugging.

## Bug 2: Prevent duplicate PR creation

- [~] In `ensurePullRequestForRepo()` (`api/pkg/server/spec_task_workflow_handlers.go`), add an early return before `ListPullRequests` when `task.RepoPullRequests` already contains an entry for the repo with a non-empty PRID

## Verification

- [ ] Build Go backend: `go build ./...` from `api/`
- [ ] Build frontend: `cd frontend && yarn build`
- [ ] Test scenario: create a spec task, let it create a PR, merge the PR, verify the task card shows "Task finished" without an error banner
- [ ] Test scenario: create a spec task, let it create a PR, close the PR on GitHub, verify no duplicate PR is created on the next poll cycle
