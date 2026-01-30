# Design: Auto-Open PR on Approval When Commits Exist

## Overview

When a user approves an implementation for an external repository, the backend should immediately open a PR if the feature branch already has commits ahead of the default branch, rather than waiting for the agent to push.

## Current Behavior

In `spec_task_workflow_handlers.go`:
1. User calls `approveImplementation`
2. If `shouldOpenPullRequest(repo)` returns true:
   - Status → `pull_request`
   - Send WebSocket message to agent asking it to push (even empty commit)
3. Agent receives message, pushes commit
4. `handleFeatureBranchPush` detects push in `pull_request` status
5. Calls `ensurePullRequest` which creates the PR

**Problem**: If agent forgets to push, PR never gets created.

## Proposed Change

Add a check in `approveImplementation` after setting status to `pull_request`:

```go
// After: specTask.Status = types.TaskStatusPullRequest

// Check if branch already has commits - if so, create PR immediately
hasCommits, err := s.branchHasCommitsAhead(ctx, repo.LocalPath, specTask.BranchName, repo.DefaultBranch)
if err == nil && hasCommits {
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        if err := s.gitHTTPServer.EnsurePullRequest(ctx, repo, specTask, specTask.BranchName); err != nil {
            log.Error().Err(err).Str("task_id", specTask.ID).Msg("Failed to auto-create PR on approval")
        }
    }()
}

// Still send message to agent (fallback for edge cases)
```

## Implementation Details

### New Helper Function

Add `branchHasCommitsAhead` in `spec_task_workflow_handlers.go` or use existing `GetDivergence` from `gitea_git_helpers.go`:

```go
func (s *HelixAPIServer) branchHasCommitsAhead(ctx context.Context, repoPath, featureBranch, defaultBranch string) (bool, error) {
    ahead, _, err := services.GetDivergence(ctx, repoPath, featureBranch, defaultBranch)
    if err != nil {
        return false, err
    }
    return ahead > 0, nil
}
```

### Exposing ensurePullRequest

The `ensurePullRequest` function is on `GitHTTPServer`. Either:
1. Make it a method on `HelixAPIServer` (preferred - cleaner)
2. Export it from `GitHTTPServer` and call via server reference

**Decision**: Move PR creation logic to a shared service or duplicate the minimal logic needed in the workflow handler.

## Key Files

- `helix/api/pkg/server/spec_task_workflow_handlers.go` - Add check and auto-PR call
- `helix/api/pkg/services/gitea_git_helpers.go` - Existing `GetDivergence` function
- `helix/api/pkg/services/git_http_server.go` - Existing `ensurePullRequest` function

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Race condition with agent push | `ensurePullRequest` already checks for existing PRs |
| Branch doesn't exist in external repo | Push branch before creating PR (already done in `ensurePullRequest`) |

## Testing

1. Create task, make commits on feature branch, don't push
2. Approve implementation
3. Verify PR is created immediately without agent action