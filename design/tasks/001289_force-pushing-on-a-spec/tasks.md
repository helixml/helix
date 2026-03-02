# Implementation Tasks

## Part 1: Force Push Detection and Propagation

### Modify detectChangedBranches
- [ ] Change return type from `[]string` to `map[string]bool` (branch → isForce)
- [ ] Add `repoPath` parameter to function signature
- [ ] Add force push detection using `git merge-base --is-ancestor oldrev newrev`
- [ ] If old commit is NOT ancestor of new commit → mark as force push

### Update Push Loop
- [ ] Update the loop at line 600-620 in `git_http_server.go` to iterate over map
- [ ] Pass the per-branch `isForce` flag to `PushBranchToRemote` call (line 606)
- [ ] Add logging when force push is detected: log branch name, old hash, new hash

### Update Dependent Code
- [ ] Update `rollbackBranchRefs` call to work with new map keys
- [ ] Update `pushedBranches` logging to extract keys from map

## Part 2: Startup Recovery for Diverged Branches

### Add Divergence Detection
- [ ] Add `isBranchDivergedFromRemote(ctx, repoPath, branch)` function in `git_repository_service.go`
- [ ] Use `git rev-list --left-right --count origin/branch...branch` to get ahead/behind counts
- [ ] Return true if both ahead AND behind counts are > 0

### Extend recoverIncompletePushes
- [ ] After `isBranchAheadOfRemote` check, also call `isBranchDivergedFromRemote`
- [ ] Determine if branch is protected: `branch == "helix-specs" || branch == repo.DefaultBranch`
- [ ] For diverged protected branches: log warning and skip
- [ ] For diverged feature branches: call `PushBranchToRemote(ctx, repo.ID, branch, true)`
- [ ] Add logging for recovery actions (recovered vs skipped)

## Testing

### Unit Tests
- [ ] Test `detectChangedBranches` correctly identifies force push (non-ancestor case)
- [ ] Test `detectChangedBranches` correctly identifies normal push (ancestor case)
- [ ] Test `isBranchDivergedFromRemote` returns true when diverged
- [ ] Test `isBranchDivergedFromRemote` returns false when just ahead

### Integration Tests
- [ ] Test normal push still works (fast-forward)
- [ ] Test force push on feature branch propagates to upstream
- [ ] Test force push on `helix-specs` is still blocked by pre-receive hook
- [ ] Test startup recovery force-pushes diverged feature branch
- [ ] Test startup recovery skips diverged `helix-specs` branch