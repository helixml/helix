# Implementation Tasks

## Bug 1: Clear error metadata on merge transitions

- [ ] In `processExternalPullRequestStatus()` (`api/pkg/services/spec_task_orchestrator.go:771`), clear `task.Metadata["error"]` before saving when all PRs are merged
- [ ] In `checkTaskForExternalPRActivity()` (`api/pkg/services/spec_task_orchestrator.go:1064`), clear `task.Metadata["error"]` before saving when a merged PR is detected
- [ ] In the branch-merge fallback (`api/pkg/services/spec_task_orchestrator.go:841`), clear `task.Metadata["error"]` before saving when branch is detected as merged
- [ ] In `TaskCard.tsx` (`frontend/src/components/tasks/TaskCard.tsx:1068`), skip the error banner when `task.phase === "completed"` as a defensive frontend guard

## Bug 2: Prevent duplicate PR creation

- [ ] In `ensurePullRequestForRepo()` (`api/pkg/server/spec_task_workflow_handlers.go`), add an early return before `ListPullRequests` when `task.RepoPullRequests` already contains an entry for the repo with a non-empty PRID

## Verification

- [ ] Build Go backend: `go build ./...` from `api/`
- [ ] Build frontend: `cd frontend && yarn build`
- [ ] Test scenario: create a spec task, let it create a PR, merge the PR, verify the task card shows "Task finished" without an error banner
- [ ] Test scenario: create a spec task, let it create a PR, close the PR on GitHub, verify no duplicate PR is created on the next poll cycle
