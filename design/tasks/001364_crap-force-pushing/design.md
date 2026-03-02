# Design: Force Push Handling in Git Integration

## Architecture Overview

Helix uses a "middle repo" pattern for git integration:

```
External Upstream (GitHub/ADO)
        ↕ (fetch/push)
Middle Repo (Helix bare repo at /filestore/git-repositories/)
        ↕ (clone/push via git HTTP)
Agent Working Copy (inside sandbox container)
```

The problem: when upstream is force-pushed, the middle repo diverges and subsequent operations fail.

## Current Behavior

1. `SyncAllBranches(force=true)` uses refspec `+refs/heads/*:refs/heads/*` which SHOULD force-update
2. `handleReceivePack` syncs from upstream BEFORE accepting agent push
3. `PushBranchToRemote` pushes middle repo → upstream (fails on non-fast-forward)

**The bug**: When upstream was force-pushed, the middle repo is "ahead" (has commits upstream doesn't). The current force-sync overwrites middle repo refs, but if an agent has unpushed commits in their working copy, those commits become orphaned when they try to push.

## Solution Design

### Change 1: Detect Force-Push in `handleReceivePack`

In `git_http_server.go`, after sync but before receive-pack:

```go
// Detect if upstream was force-pushed (middle was ahead before sync)
if wasMiddleAhead {
    log.Warn().Str("repo_id", repoID).Msg("Detected upstream force-push - middle repo was reset")
    // Agent's push will be non-fast-forward against new upstream
    // Need to inform agent to rebase
}
```

### Change 2: Rebase Agent Commits on Divergence

When agent push fails with non-fast-forward error in `handleReceivePack`:

1. Capture the pushed commits (already have `branchesAfter` hashes)
2. Reset middle repo branch to upstream HEAD
3. Cherry-pick/rebase agent commits onto new HEAD
4. Push to upstream

This happens server-side, transparent to the agent.

### Change 3: Track Middle Repo State

Add tracking to detect divergence before sync:

```go
func (s *GitHTTPServer) handleReceivePack(...) {
    // Before sync, check if we have commits upstream doesn't
    middleCommit := getBranchHash(repoPath, branch)
    
    // Sync from upstream
    s.gitRepoService.SyncAllBranches(...)
    
    // After sync, check if middle commit is now orphaned
    upstreamCommit := getBranchHash(repoPath, branch)
    if middleCommit != upstreamCommit {
        // Force-push detected, middleCommit is now orphaned
    }
}
```

## Key Files to Modify

| File | Change |
|------|--------|
| `api/pkg/services/git_http_server.go` | Add rebase-on-divergence logic in `handleReceivePack` |
| `api/pkg/services/git_helpers.go` | Add `RebaseCommitsOnto(ctx, repoPath, commits, targetRef)` helper |
| `api/pkg/services/git_repository_service_pull.go` | Return divergence info from `SyncAllBranches` |

## Error Handling

- If rebase fails (conflict), reject the push with clear error
- Log all force-push detection events
- Never silently drop commits

## Testing

Add test in `git_integration_test.go`:

```go
func (s *GitIntegrationSuite) TestAgentPushAfterUpstreamForcePush() {
    // 1. Agent clones, makes commit A
    // 2. External user force-pushes upstream (different history)
    // 3. Agent pushes commit A
    // Expected: A is rebased onto new upstream and pushed
}
```

## Constraints

- No UI changes required
- Must work with existing agent workflow (agent doesn't know about force-push)
- Pre-receive hook already protects `helix-specs` branch from force-push