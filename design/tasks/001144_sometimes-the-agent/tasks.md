# Implementation Tasks

- [ ] Add `branchHasCommitsAhead` helper function in `spec_task_workflow_handlers.go` using existing `services.GetDivergence`
- [ ] Expose `ensurePullRequest` logic - either extract to GitRepositoryService or add method to HelixAPIServer
- [ ] In `approveImplementation`, after setting status to `pull_request`, check if branch has commits ahead
- [ ] If commits exist, call `ensurePullRequest` in goroutine to create PR immediately
- [ ] Re-fetch task after async PR creation to get `PullRequestID` and compute `PullRequestURL` for response
- [ ] Add logging for auto-PR creation path
- [ ] Test: approve task with existing commits → PR created immediately
- [ ] Test: approve task with no commits → waits for agent push (existing behavior)