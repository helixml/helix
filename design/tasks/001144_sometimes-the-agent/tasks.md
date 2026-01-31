# Implementation Tasks

- [x] Add `branchHasCommitsAhead` helper function in `spec_task_workflow_handlers.go` using existing `services.GetDivergence`
- [x] Add `ensurePullRequestForTask` method on `HelixAPIServer` that creates PR using `GitRepositoryService.CreatePullRequest`
- [x] In `approveImplementation`, after setting status to `pull_request`, check if branch has commits ahead and call `ensurePullRequestForTask` if so
- [x] Update `agent_implementation_approved_push.tmpl` to remove empty commit instruction, just say "commit and push any remaining changes"
- [x] Update test for new prompt content
- [x] Add logging for auto-PR creation path (already done in implementation)
- [ ] Build and verify no compile errors
- [ ] Manual test: approve task with commits already pushed → PR created immediately