# Implementation Tasks

- [x] Add `branchHasCommitsAhead` helper function in `spec_task_workflow_handlers.go` using existing `services.GetDivergence`
- [x] Add `ensurePullRequestForTask` method on `HelixAPIServer` that creates PR using `GitRepositoryService.CreatePullRequest`
- [x] In `approveImplementation`, after setting status to `pull_request`, check if branch has commits ahead and call `ensurePullRequestForTask` if so
- [ ] Re-fetch task after async PR creation to get `PullRequestID` and compute `PullRequestURL` for response
- [~] Update `agent_implementation_approved_push.tmpl` to remove empty commit instruction, just say "commit and push any remaining changes"
- [ ] Add logging for auto-PR creation path
- [ ] Test: approve task with commits already pushed → PR created immediately
- [ ] Test: approve task with no commits → agent pushes → PR created
- [ ] Test: approve task with commits pushed + uncommitted changes → PR created immediately, agent can push more