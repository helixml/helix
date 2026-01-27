# Implementation Tasks

## Core Implementation

- [ ] Add `checkUpstreamDivergence()` helper function to `git_http_server.go`
  - Takes: `ctx`, `repoPath`, `authURL`, `branchName`, `oldRef` (from branchesBefore)
  - Fetches upstream branch to `refs/remotes/origin/<branch>` using existing `Fetch()` helper
  - Gets upstream commit using `getRemoteTrackingCommit()` pattern from `git_repository_service_pull.go`
  - Returns `nil` if oldRef matches upstream (or branch is new)
  - Returns error with message "Push rejected: upstream branch changed externally. Pull latest changes and retry." if diverged

- [ ] Modify `handleReceivePack()` to call OCC check before `PushBranchToRemote()`
  - After branch restriction check, before the upstream push loop
  - For each pushed branch on external repos, call `checkUpstreamDivergence()`
  - If divergence detected: call `rollbackBranchRefs()` and return early
  - Build authenticated URL using existing `buildAuthenticatedCloneURLForRepo()` via gitRepoService

## Logging

- [ ] Add Debug log when fetching upstream for OCC check (branch, repo_id)
- [ ] Add Debug log on successful OCC check (branch, old_ref, upstream_ref)
- [ ] Add Warn log when divergence detected (branch, old_ref, upstream_ref, repo_id)
- [ ] Add Warn log if fetch fails (allow push but log the error)

## Error Handling

- [ ] Handle case where branch doesn't exist upstream (allow push - it's a new branch)
- [ ] Handle fetch failures gracefully (log warning, allow push to proceed)
- [ ] Ensure rollback happens before returning error to agent

## Testing

- [ ] Add unit test for `checkUpstreamDivergence()` - upstream matches old_ref
- [ ] Add unit test for `checkUpstreamDivergence()` - upstream diverged
- [ ] Add unit test for `checkUpstreamDivergence()` - new branch (no upstream)
- [ ] Add integration test in `git_http_server_test.go` simulating external push