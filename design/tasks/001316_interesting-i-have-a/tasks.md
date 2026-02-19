# Implementation Tasks

- [ ] Add `IsCommitInBranch(ctx, repoID, commitSHA, branchName)` method to `git_repository_service.go` if not already present
- [ ] Add `detectMergedBranches()` function in `spec_task_orchestrator.go` that:
  - Lists tasks in `spec_review` or `implementation` status with non-empty `BranchName`
  - Checks if each task's branch HEAD (or `LastPushCommitHash`) exists in main
  - Transitions merged tasks to `done` status with `merged_to_main=true`, `merged_at` timestamp
- [ ] Call `detectMergedBranches()` from `prPollLoop()` after `pollPullRequests()`
- [ ] Add rate limiting: process max 10 tasks per poll cycle
- [ ] Add logging for merged branch detection events
- [ ] Test with Azure DevOps: create task, merge PR externally, verify auto-completion
- [ ] Test with GitHub: same flow
- [ ] Test edge case: branch deleted after merge (should still detect via `LastPushCommitHash`)