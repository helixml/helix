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

## Divergence Scenarios and Agent Reconciliation

### When Force Push is Appropriate

Force push is safe when the agent is updating **its own work** - e.g., after rebasing onto the latest main branch. The agent knows it's replacing its previous commits with amended versions.

### When Force Push Could Cause Data Loss

If someone else pushed to the same feature branch (directly to GitHub, bypassing Helix), a force push would overwrite their changes. This is rare for agent feature branches but possible.

### How Agents Can Reconcile

The agent can already reconcile because:

1. **Pre-push sync**: Before accepting any push, Helix calls `SyncAllBranches` which fetches upstream changes into the local bare repo
2. **Agent can fetch**: The agent can `git fetch origin` to see upstream commits
3. **Agent can merge/rebase**: The agent can incorporate upstream changes before pushing

**Reconciliation workflow for agents:**
```bash
# Agent detects push failed (non-fast-forward)
git fetch origin
git log --oneline origin/feature/my-branch..HEAD  # See local commits
git log --oneline HEAD..origin/feature/my-branch  # See upstream commits

# Option A: Rebase (if upstream has new commits to incorporate)
git rebase origin/feature/my-branch
git push  # Now fast-forward, no force needed

# Option B: Force push (if agent intentionally replacing its own work)
git push --force
```

### Future Enhancement: Divergence Detection and Flagging

Out of scope for this task, but a future enhancement could:

1. **Detect divergence on push failure**: When upstream push fails with non-fast-forward, check if it's:
   - Agent's own commits being replaced (safe to force push)
   - Unknown upstream commits that would be lost (needs reconciliation)

2. **Flag on SpecTask**: Add a status flag like `needs_reconciliation` with details:
   - "Branch diverged: local has 3 commits, upstream has 2 unknown commits"
   - "Upstream commits by: user@example.com (not this agent)"

3. **Agent guidance**: Provide clear instructions in error message for reconciliation

This is deferred because:
- It's complex to determine "ownership" of commits
- The simple case (agent rebasing its own work) should just work with force push
- True divergence (someone else pushed) is rare for agent branches

## Security Considerations

- Force push is only propagated, not enabled - if the local receive-pack rejected it, we never get here
- Pre-receive hook still enforces `helix-specs` protection
- Agent branch restrictions (`HELIX_ALLOWED_BRANCHES`) prevent agents from touching default branch
- Logging captures all force push events for audit trail

## Alternatives Considered

1. **Always force push**: Risky - could overwrite legitimate upstream changes
2. **Parse receive-pack output**: Complex and fragile - protocol is binary
3. **Compare ancestry (chosen)**: Simple, reliable, matches what git does internally