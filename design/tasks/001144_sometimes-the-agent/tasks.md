# Implementation Tasks

- [~] Add `branchHasCommitsAhead` helper function in `spec_task_workflow_handlers.go` using existing `services.GetDivergence`
- [ ] Add `ensurePullRequestForTask` method on `HelixAPIServer` that creates PR using `GitRepositoryService.CreatePullRequest`
- [ ] In `approveImplementation`, after setting status to `pull_request`, check if branch has commits ahead
- [ ] If commits exist, call `ensurePullRequestForTask` in goroutine to create PR immediately
- [ ] Re-fetch task after async PR creation to get `PullRequestID` and compute `PullRequestURL` for response
- [ ] Update `agent_implementation_approved_push.tmpl` to remove empty commit instruction, just say "commit and push any remaining changes"
- [ ] Add logging for auto-PR creation path
- [ ] Test: approve task with commits already pushed → PR created immediately
- [ ] Test: approve task with no commits → agent pushes → PR created
- [ ] Test: approve task with commits pushed + uncommitted changes → PR created immediately, agent can push more