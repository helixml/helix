# Implementation Tasks

## Modify detectChangedBranches

- [~] Change return type from `[]string` to `map[string]bool` (branch → isForce)
- [ ] Add `repoPath` parameter to function signature
- [ ] Add force push detection using `git merge-base --is-ancestor oldrev newrev`
- [ ] If old commit is NOT ancestor of new commit → mark as force push

## Update Push Loop

- [ ] Update the loop at line 600-620 in `git_http_server.go` to iterate over map
- [ ] Pass the per-branch `isForce` flag to `PushBranchToRemote` call (line 606)
- [ ] Add logging when force push is detected: log branch name, old hash, new hash

## Update Dependent Code

- [ ] Update `rollbackBranchRefs` call to work with new map keys
- [ ] Update `pushedBranches` logging to extract keys from map

## Testing

- [ ] Test normal push still works (fast-forward)
- [ ] Test force push on feature branch propagates to upstream
- [ ] Test force push on `helix-specs` is still blocked by pre-receive hook