# Design: Force Push Support for Agent Feature Branches

## Overview

Enable agents to force-push on their feature branches by detecting force pushes in the git HTTP server and propagating the force flag to upstream pushes.

## Current Architecture

```
Agent → git push → Helix Git HTTP Server → receive-pack (local bare repo) → PushBranchToRemote(force=false) → GitHub
```

**Problem**: `PushBranchToRemote` is always called with `force: false` on line 606 of `git_http_server.go`, regardless of whether the incoming push was a force push.

## Solution

Detect force pushes by comparing commit ancestry and propagate the force flag per-branch.

### Key Files
- `api/pkg/services/git_http_server.go` - Handles incoming pushes, calls `PushBranchToRemote`

### Changes Required

1. **Modify `detectChangedBranches`** to also detect if each change was a force push:
   - Compare old commit hash with new commit hash
   - Use `git merge-base --is-ancestor oldrev newrev` to check if fast-forward
   - If old is NOT ancestor of new → force push detected

2. **Track force push flag per branch** instead of just branch names:
   - Change from `[]string` (branch names) to `map[string]bool` (branch → isForce)

3. **Pass force flag to `PushBranchToRemote`**:
   - Change line 606: `PushBranchToRemote(branchCtx, repoID, branch, isForce)`

### Force Push Detection Logic

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

## Protection Scope

**Why we don't need extra protection checks:**

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

## Alternatives Considered

1. **Always force push**: Risky - could overwrite legitimate upstream changes
2. **Parse receive-pack output**: Complex and fragile - protocol is binary
3. **Compare ancestry (chosen)**: Simple, reliable, matches what git does internally