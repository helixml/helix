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

### Phase 1: Base Branch Sync on Task Start ✅ IMPLEMENTED

1. ✅ Added `SyncBaseBranch(ctx, repoID, branchName)` method:
   - Fetches only the specified branch to remote-tracking ref
   - Detects divergence using merge-base algorithm
   - Returns `BranchDivergenceError` if diverged

2. ✅ Called `SyncBaseBranch` at start of `StartSpecGeneration` and `StartJustDoItMode`:
   - Before creating feature branch
   - If diverged, sets task to error state with user-friendly message

3. ✅ Added divergence detection:
   - `countCommitsDiff()` counts commits ahead/behind
   - `FormatDivergenceErrorForUser()` creates clear error message

### Phase 2: Divergence Resolution Options (Future)

**Current state:** Divergence is detected and user gets error message, but no automated resolution.

**Future resolution options to implement:**

1. **Force Sync from Upstream** (Destructive)
   - API: `POST /api/v1/git-repositories/{id}/force-sync`
   - Uses `PullFromRemote(ctx, repoID, branchName, force=true)`
   - Overwrites local with upstream (loses local-only commits)
   - Requires confirmation dialog: "This will discard X local commits"
   - Use case: "I don't care about local changes, just give me upstream"

2. **Push Local to Upstream First** (Preservative)
   - API: `POST /api/v1/git-repositories/{id}/push-branch`
   - Push local changes to upstream before syncing
   - May fail if upstream has conflicting changes
   - Use case: "I have work in Helix that wasn't pushed yet"

3. **Show Commits and Let User Decide** (Informative)
   - List the specific commits that exist locally but not upstream
   - List the specific commits that exist upstream but not locally
   - User chooses: force sync, push first, or manual reconciliation
   - Best UX but most complex to implement

**Recommended implementation order:**
1. Force Sync (simplest, covers most recovery cases)
2. Show Commits (helps user understand the situation)
3. Push Local (less common need)

### Phase 3: UI Improvements (Future)

1. Show sync status on repository page
2. Add "Force Sync" button with confirmation
3. Show divergence warnings on task creation
4. Show commit diff when divergence detected

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
