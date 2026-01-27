# Design: Optimistic Concurrency Control for Git Push

## Architecture

The optimistic concurrency control (OCC) check is inserted into the existing `handleReceivePack` flow in `git_http_server.go`, between accepting the agent's push and calling `PushBranchToRemote`.

### Current Flow

```
Agent push → receive-pack → push to upstream → post-push hooks
```

### New Flow

```
Agent push → receive-pack → OCC check → push to upstream (if OK) → post-push hooks
                              ↓
                         (if diverged) → rollback branch → return error
```

## Key Design Decisions

### 1. Use `branchesBefore` as `old_ref`

The existing code already captures branch hashes before `git receive-pack` runs:

```go
branchesBefore := s.getBranchHashes(repoPath)
```

This map contains `old_ref` for each branch - what the middle repo had before the agent's push arrived.

### 2. Fetch Upstream HEAD Before Push

For each pushed branch, fetch the upstream branch to a temporary ref and compare:

```go
// Fetch upstream branch to refs/remotes/origin/<branch>
refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch)
Fetch(ctx, repoPath, FetchOptions{Remote: authURL, RefSpecs: []string{refSpec}})

// Get upstream commit
upstreamCommit := getRemoteTrackingCommit(ctx, repoPath, branch)

// Compare with old_ref
if oldRef != upstreamCommit {
    // Divergence detected!
}
```

### 3. Reuse Existing Helpers

From `gitea_git_helpers.go`:
- `GetBranchCommitID()` - get local branch commit
- `Fetch()` - fetch from remote with refspecs

From `git_repository_service_pull.go`:
- `getRemoteTrackingCommit()` - get commit from refs/remotes/origin/<branch>
- Pattern for building authenticated fetch URL via `buildAuthenticatedCloneURLForRepo()`

### 4. Rollback on Divergence

Use existing `rollbackBranchRefs()` to restore branch to `old_ref` state before returning error.

## Implementation Location

All changes are in `api/pkg/services/git_http_server.go`:

1. Add helper function `checkUpstreamDivergence()` that:
   - Takes repo path, auth URL, branch name, old_ref
   - Fetches upstream to temporary ref
   - Compares upstream HEAD to old_ref
   - Returns error if diverged

2. Modify `handleReceivePack()` to call this check before `PushBranchToRemote()`

## Error Handling

Error message format:
```
Push rejected: upstream branch changed externally. Pull latest changes and retry.
```

The rollback ensures the middle repo stays in sync with what the agent last knew, forcing the agent to pull and reconcile.

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Branch doesn't exist upstream | Allow push (new branch) |
| Branch doesn't exist locally (branchesBefore) | Allow push (new branch) |
| Upstream same as old_ref | Allow push |
| Upstream different from old_ref | Reject + rollback |
| Fetch fails (network error) | Log warning, allow push (fail-open) |

## Testing Strategy

1. Unit test `checkUpstreamDivergence()` with mocked git commands
2. Integration test in `git_http_server_test.go` with real git repos