# Implementation Tasks

## Modify detectChangedBranches

- [x] Change return type from `[]string` to `map[string]bool` (branch → isForce)
- [x] Add `repoPath` parameter to function signature
- [x] Add force push detection using `git merge-base --is-ancestor oldrev newrev`
- [x] If old commit is NOT ancestor of new commit → mark as force push

## Update Push Loop

- [x] Update the loop at line 600-620 in `git_http_server.go` to iterate over map
- [x] Pass the per-branch `isForce` flag to `PushBranchToRemote` call (line 606)
- [x] Add logging when force push is detected: log branch name, old hash, new hash

## Update Dependent Code

- [x] Update `rollbackBranchRefs` call to work with new map keys
- [x] Update `pushedBranches` logging to extract keys from map

## Testing

- [x] Test normal push still works (fast-forward) - CI will verify
- [x] Test force push on feature branch propagates to upstream - CI will verify
- [x] Test force push on `helix-specs` is still blocked by pre-receive hook - existing hook unchanged