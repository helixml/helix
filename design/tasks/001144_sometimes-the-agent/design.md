# Design: Auto-Open PR on Approval When Commits Exist

## Overview

When a user approves an implementation for an external repository, the backend should:
1. Immediately open a PR if the feature branch already has commits pushed
2. Tell the agent to commit and push any remaining uncommitted work (no empty commit needed)

## Current Behavior

In `spec_task_workflow_handlers.go`:
1. User calls `approveImplementation`
2. If `shouldOpenPullRequest(repo)` returns true:
   - Status → `pull_request`
   - Send WebSocket message to agent asking it to push (including empty commit if needed)
3. Agent receives message, pushes commit
4. `handleFeatureBranchPush` detects push in `pull_request` status
5. Calls `ensurePullRequest` which creates the PR

**Problem**: 
- If agent forgets to push, PR never gets created
- Empty commit is unnecessary when commits already exist

## Proposed Change

In `approveImplementation`, after setting status to `pull_request`:

```go
// Check if branch already has commits - if so, create PR immediately
hasCommits, err := s.branchHasCommitsAhead(ctx, repo.LocalPath, specTask.BranchName, repo.DefaultBranch)
if err == nil && hasCommits {
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        if err := s.ensurePullRequestForTask(ctx, repo, specTask); err != nil {
            log.Error().Err(err).Str("task_id", specTask.ID).Msg("Failed to create PR on approval")
        }
    }()
}

// Always tell agent to commit and push any uncommitted changes (no empty commit mention)
message, err := prompts.ImplementationApprovedPushInstruction(specTask.BranchName)
```

Also update the prompt template to remove the empty commit instruction.

## Implementation Details

### New Helper Function

Add in `spec_task_workflow_handlers.go`:

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

The `ensurePullRequest` function is currently on `GitHTTPServer`. Options:
1. Add a wrapper method on `HelixAPIServer` that delegates to GitRepositoryService
2. Move the PR creation logic to GitRepositoryService (cleaner)

**Decision**: Add `ensurePullRequestForTask` method on `HelixAPIServer` that reuses the existing PR creation logic from `GitRepositoryService.CreatePullRequest`.

### Update Prompt Template

Update `agent_implementation_approved_push.tmpl`:

**Before:**
```
If you have uncommitted changes, commit them first. If all changes are already committed, you can push an empty commit:
git commit --allow-empty -m "chore: open pull request for review"
git push origin {{ .BranchName }}
```

**After:**
```
Please commit and push any remaining uncommitted changes:
git add -A
git commit -m "Complete implementation"
git push origin {{ .BranchName }}

If all your changes are already committed and pushed, no action is needed.
```

## Key Files

- `helix/api/pkg/server/spec_task_workflow_handlers.go` - Add commit check and auto-PR call
- `helix/api/pkg/services/gitea_git_helpers.go` - Existing `GetDivergence` function
- `helix/api/pkg/services/git_repository_service.go` - Existing `CreatePullRequest` method
- `helix/api/pkg/prompts/templates/agent_implementation_approved_push.tmpl` - Update prompt

## Flow Summary

| Scenario | PR Creation | Agent Message |
|----------|-------------|---------------|
| Commits already pushed | Backend creates immediately | "Commit and push any remaining changes" |
| No commits yet | Created when agent pushes | "Commit and push any remaining changes" |

## Testing

1. Create task, push commits to feature branch, approve → PR created immediately
2. Create task, no commits, approve → agent pushes → PR created
3. Create task, push some commits, have uncommitted changes, approve → PR created immediately, agent pushes more commits

## Implementation Notes

### Files Modified

1. `api/pkg/server/spec_task_workflow_handlers.go`:
   - Added `branchHasCommitsAhead` helper using `services.GetDivergence`
   - Added `ensurePullRequestForTask` method that pushes branch, checks for existing PR, creates if needed
   - Modified `approveImplementation` to check for commits and create PR immediately if found

2. `api/pkg/prompts/templates/agent_implementation_approved_push.tmpl`:
   - Removed empty commit instruction entirely
   - Now just instructs agent to commit and push any uncommitted changes
   - Added note that if everything is already pushed, no action needed

3. `api/pkg/prompts/helix_code_prompts_test.go`:
   - Updated assertion to match new prompt wording

### Key Decisions

- PR creation runs in goroutine (async) to not block the API response
- Agent message is still always sent - it handles the case where agent has uncommitted work
- The `ensurePullRequestForTask` method on `HelixAPIServer` mirrors the logic from `GitHTTPServer.ensurePullRequest` but with proper error handling for the task update
- Used existing `services.GetDivergence` from `gitea_git_helpers.go` rather than creating new git logic