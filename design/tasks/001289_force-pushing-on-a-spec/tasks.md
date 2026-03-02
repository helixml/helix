# Implementation Tasks

## Force Push Detection

- [ ] Modify `detectChangedBranches` in `api/pkg/services/git_http_server.go` to return `map[string]bool` (branch → isForce) instead of `[]string`
- [ ] Add force push detection using `git merge-base --is-ancestor oldrev newrev`
- [ ] Update function signature to take `repoPath` parameter for running git commands

## Propagate Force Flag

- [ ] Update the loop at line 600-620 in `git_http_server.go` to use the new map structure
- [ ] Pass the per-branch `isForce` flag to `PushBranchToRemote` call (line 606)
- [ ] Add logging when force push is detected and propagated

## Protected Branch Safety

- [ ] Add check to skip force push for default branch (main/master) even if detected
- [ ] Verify `helix-specs` protection still works (pre-receive hook handles this)

## Update Dependent Code

- [ ] Update `rollbackBranchRefs` call to work with new data structure
- [ ] Update any logging that references `pushedBranches`

## Testing

- [ ] Test normal push still works (fast-forward)
- [ ] Test force push on feature branch propagates to upstream
- [ ] Test force push on `helix-specs` is still blocked
- [ ] Test force push on default branch is blocked