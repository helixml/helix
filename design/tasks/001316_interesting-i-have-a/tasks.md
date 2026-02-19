# Implementation Tasks

- [x] Add `IsBranchMerged(ctx, repoID, branchName, targetBranch)` method to `git_repository_service.go` if not present
- [~] Add `detectExternalPRActivity()` function in `spec_task_orchestrator.go` that:
  - Lists tasks in `spec_review` or `implementation` status with non-empty `BranchName` but no `PullRequestID`
  - For each task, checks for open PRs on that branch
  - If open PR found: link it (`PullRequestID`, `PullRequestURL`) and transition to `pull_request` status
  - If no open PR: check if branch HEAD (or `LastPushCommitHash`) exists in main
  - If merged: transition to `done` status with `merged_to_main=true`, `merged_at` timestamp
- [ ] Call `detectExternalPRActivity()` from `prPollLoop()` after `pollPullRequests()`
- [ ] Add rate limiting: process max 10 tasks per poll cycle
- [ ] Add logging for PR detection and merged branch detection events
- [ ] Test with Azure DevOps: create task, open PR externally, verify task moves to `pull_request` column
- [ ] Test with Azure DevOps: merge PR externally, verify task moves to `done`
- [ ] Test with GitHub: same flows
- [ ] Test edge case: branch deleted after merge (should still detect via `LastPushCommitHash`)