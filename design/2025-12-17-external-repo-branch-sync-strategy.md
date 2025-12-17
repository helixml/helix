# External Repository Branch Sync Strategy

**Date:** 2025-12-17
**Status:** Proposed
**Author:** Luke Marsden + Claude

## Problem Statement

When Helix imports an external repository (e.g., from Azure DevOps), we need a clear strategy for:
1. Which branches to sync on initial import
2. When and how to sync branches during task lifecycle
3. How to handle divergence/conflicts between Helix's local copy and upstream

## Current Architecture

Helix stores external repositories as **bare git repositories** in the filestore. This allows:
- Agents to push to the Helix repo via HTTP
- Helix to push changes to the external upstream when needed
- No working directory conflicts

```
External Repo (ADO/GitHub)     Helix Bare Repo        Agent Sandbox
        │                            │                      │
        │◄────── initial clone ──────┤                      │
        │                            │◄──── agent pushes ───┤
        │◄────── helix pushes ───────┤                      │
```

## Proposed Strategy

### 1. Initial Clone - Mirror ALL Branches

When importing an external repository:
- Use `Mirror: true` in go-git CloneOptions
- This clones ALL refs (branches, tags) as local refs
- Preserves upstream's default branch name (don't force rename master→main)

**Implementation:** Already done with `Mirror: true` option.

### 2. Feature Branch Push - Ownership Model

When an agent pushes to a feature branch from a SpecTask:
- Only push THAT specific branch to upstream
- Assume Helix has "ownership" of feature branches it creates
- Use branch naming convention: `feature/helix-<task-id>-<name>`

**Rationale:** Feature branches created by Helix tasks are owned by Helix. The agent may make multiple commits, and we push when ready. We don't need to sync other branches.

**Implementation:** Already working - `PushBranchToRemote` only pushes the specific branch.

### 3. Starting New SpecTask - Sync Base Branch Only

When starting a new SpecTask:
1. Identify the base branch (user-specified or repo's default branch)
2. Fetch ONLY that branch from upstream
3. Check for divergence:
   - **Fast-forward possible:** Local is behind upstream → auto-sync (fetch + fast-forward)
   - **Diverged:** Local has commits not in upstream → ERROR with clear message

**Divergence Detection:**

```
# Fast-forward (OK to sync):
upstream:  A ── B ── C ── D
local:     A ── B ── C

# Diverged (ERROR - manual reconciliation needed):
upstream:  A ── B ── C ── D
local:     A ── B ── E ── F
```

**Error Message for Divergence:**
```
Cannot sync base branch 'main': local and upstream have diverged.

Local branch has X commits not in upstream.
Upstream branch has Y commits not in local.

This can happen when:
- Someone pushed directly to the Helix copy of this branch
- The upstream branch was force-pushed
- A previous Helix task's changes were merged differently

To resolve:
1. Go to the external repository (Azure DevOps/GitHub)
2. Reconcile the branches manually
3. Force sync in Helix: [Force Sync Button]

Warning: Force sync will overwrite local changes with upstream.
```

### 4. When NOT to Sync

- **During task execution:** Don't sync while agent is working
- **After task completion:** Don't auto-sync (task might have pending PR)
- **On page refresh:** Don't auto-sync (just show current state)

### 5. Manual Sync Options

Provide UI options:
- **"Sync from Upstream"** button on repository page
  - Syncs all branches (SyncAllBranches with Prune)
  - Shows warning if any branches diverged
- **"Force Sync Branch"** option
  - Overwrites local with upstream (for recovery)
  - Requires confirmation

## Implementation Plan

### Phase 1: Base Branch Sync on Task Start (This PR)

1. Add `SyncBaseBranch(ctx, repoID, branchName)` method:
   - Fetches only the specified branch
   - Detects divergence
   - Returns error if diverged

2. Call `SyncBaseBranch` at start of `StartSpecGeneration`:
   - Before creating feature branch
   - If diverged, set task to error state with message

3. Add divergence detection helper:
   - Compare local HEAD vs remote HEAD
   - Count commits ahead/behind

### Phase 2: UI Improvements (Future)

1. Show sync status on repository page
2. Add manual sync buttons
3. Show divergence warnings

## Code Locations

- `api/pkg/services/git_repository_service_pull.go` - Pull/sync functions
- `api/pkg/services/spec_driven_task_service.go` - Task lifecycle
- `api/pkg/services/git_repository_service.go` - `GetExternalRepoStatus` (already has ahead/behind counting)

## Testing

1. Import ADO repo → verify all branches visible
2. Start task on main → verify base branch synced
3. Modify main in ADO → start new task → verify sync
4. Push to Helix main directly → start new task → verify divergence error
5. Force sync → verify recovery works

## Questions for Review

1. Should force sync require admin privileges?
2. Should we auto-sync on PR merge (when task completes)?
3. Should we notify user when upstream changes detected?
