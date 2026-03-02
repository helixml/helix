# Design: Force Push Support for Agent Feature Branches

## Overview

Enable agents to force-push on their feature branches by detecting force pushes in the git HTTP server and propagating the force flag to upstream pushes. Also add startup recovery for branches that have diverged from upstream.

## Current Architecture

```
Agent → git push → Helix Git HTTP Server → receive-pack (local bare repo) → PushBranchToRemote(force=false) → GitHub
```

**Problem**: `PushBranchToRemote` is always called with `force: false` on line 606 of `git_http_server.go`, regardless of whether the incoming push was a force push.

## Solution

### Part 1: Detect and Propagate Force Pushes

Detect force pushes by comparing commit ancestry and propagate the force flag per-branch.

#### Key Files
- `api/pkg/services/git_http_server.go` - Handles incoming pushes, calls `PushBranchToRemote`

#### Changes Required

1. **Modify `detectChangedBranches`** to also detect if each change was a force push:
   - Compare old commit hash with new commit hash
   - Use `git merge-base --is-ancestor oldrev newrev` to check if fast-forward
   - If old is NOT ancestor of new → force push detected

2. **Track force push flag per branch** instead of just branch names:
   - Change from `[]string` (branch names) to `map[string]bool` (branch → isForce)

3. **Pass force flag to `PushBranchToRemote`**:
   - Change line 606: `PushBranchToRemote(branchCtx, repoID, branch, isForce)`

#### Force Push Detection Logic

```go
// In git_http_server.go
func (s *GitHTTPServer) detectChangedBranches(repoPath string, before, after map[string]string) map[string]bool {
    result := make(map[string]bool) // branch -> isForce
    for branch, newHash := range after {
        oldHash, existed := before[branch]
        if !existed || oldHash != newHash {
            isForce := false
            if existed && oldHash != "" {
                // Check if old is ancestor of new (fast-forward)
                _, _, err := gitcmd.NewCommand("merge-base", "--is-ancestor").
                    AddDynamicArguments(oldHash, newHash).
                    RunStdString(context.Background(), &gitcmd.RunOpts{Dir: repoPath})
                if err != nil {
                    // Not ancestor = force push
                    isForce = true
                }
            }
            result[branch] = isForce
        }
    }
    return result
}
```

### Part 2: Startup Recovery for Diverged Branches

Extend the existing `recoverIncompletePushes` function to also handle diverged branches.

#### Current Behavior
- `recoverIncompletePushes` finds branches where local is ahead of remote
- Calls `PushBranchToRemote(ctx, repo.ID, branch, false)` - normal push only

#### New Behavior
- Also detect branches where local and remote have **diverged** (not fast-forward)
- For diverged feature branches, use force push to recover
- Skip protected branches (`helix-specs`, default branch)

#### Detection Logic

```go
// isBranchDivergedFromRemote checks if local branch has diverged from remote
// (local has commits not in remote AND remote has commits not in local)
func (s *GitRepositoryService) isBranchDivergedFromRemote(ctx context.Context, repoPath, branch string) (bool, error) {
    // Get ahead/behind counts
    // git rev-list --left-right --count origin/branch...branch
    stdout, _, err := gitcmd.NewCommand("rev-list").
        AddArguments("--left-right", "--count").
        AddDynamicArguments("origin/"+branch+"..."+branch).
        RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
    // Parse "behind\tahead" - diverged if both > 0
}
```

#### Recovery Flow

```go
// In recoverIncompletePushes, after checking isBranchAheadOfRemote:
if ahead {
    // Check if it's diverged (needs force) or just ahead (normal push)
    diverged, _ := s.isBranchDivergedFromRemote(ctx, repo.LocalPath, branch)
    
    // Only force push on feature branches, never on protected branches
    isProtected := branch == "helix-specs" || branch == repo.DefaultBranch
    useForce := diverged && !isProtected
    
    if diverged && isProtected {
        log.Warn().Str("branch", branch).Msg("Protected branch diverged - manual intervention required")
        continue
    }
    
    s.PushBranchToRemote(ctx, repo.ID, branch, useForce)
}
```

## Protection Scope

**Why we don't need extra protection checks in Part 1:**

Agents can only push to branches listed in `HELIX_ALLOWED_BRANCHES` (enforced by pre-receive hook). This list only includes:
- `helix-specs` (for design docs - but force push blocked by pre-receive hook)
- Their assigned feature branch (e.g., `feature/001036-my-task`)

Agents are **never** allowed to push to the default branch (main/master). So when we propagate the force flag, it will only ever apply to feature branches.

**Protected branches summary:**
| Branch | Who can push | Force push allowed |
|--------|--------------|-------------------|
| `helix-specs` | Agents (design docs only) | No (pre-receive hook blocks) |
| Default branch (main) | Users only | No (agents can't push at all) |
| Feature branches | Assigned agent | Yes (this change enables it) |

## Security Considerations

- Force push is only propagated, not enabled - if the local receive-pack rejected it, we never get here
- Pre-receive hook still enforces `helix-specs` protection
- Agent branch restrictions (`HELIX_ALLOWED_BRANCHES`) prevent agents from touching default branch
- Logging captures all force push events for audit trail
- Startup recovery explicitly skips protected branches

## Alternatives Considered

1. **Always force push**: Risky - could overwrite legitimate upstream changes
2. **Parse receive-pack output**: Complex and fragile - protocol is binary
3. **Compare ancestry (chosen)**: Simple, reliable, matches what git does internally