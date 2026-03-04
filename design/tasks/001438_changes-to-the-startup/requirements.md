# Requirements: Startup Script Not Persisted on Desktop Restart

## Problem Statement

When a user edits the startup script in Project Settings and restarts the human desktop (exploratory session), the updated startup script is not picked up. The changes are saved to the `helix-specs` branch in git, but the desktop container's worktree doesn't pull the latest changes on restart.

## User Stories

### US1: Startup script changes apply on restart
As a developer, when I edit the startup script in Project Settings and click "Test Startup Script" (which restarts the desktop), I expect my updated script to run.

**Acceptance Criteria:**
- Startup script changes saved via UI are committed and pushed to `helix-specs` branch
- When desktop restarts, the `helix-specs` worktree pulls latest changes from remote
- The updated `.helix/startup.sh` is executed during workspace setup

## Root Cause Analysis

1. **Save flow works correctly**: `updateProject` handler saves startup script via `SaveStartupScriptToHelixSpecs()` → commits to bare repo → pushes to upstream
2. **Bug location**: `helix-workspace-setup.sh` lines 460-470 - when worktree already exists, it only logs "Design docs worktree already exists" but **never runs `git pull`**
3. **Result**: Desktop container has stale version of `helix-specs` branch, runs old startup script

## Technical Requirements

### TR1: Pull helix-specs on existing worktree
When `helix-workspace-setup.sh` detects an existing helix-specs worktree:
- Fetch from origin
- Pull/merge latest changes from `origin/helix-specs`
- Log success/failure
- Continue even if pull fails (worktree may have local uncommitted changes)

### TR2: Handle pull conflicts gracefully
If there are local uncommitted changes in helix-specs worktree:
- Stash changes before pull
- Pull from remote
- Apply stash (best effort)
- Log warning if stash apply fails

## Out of Scope
- Auto-saving startup script on blur (already works)
- Startup script version history (already works)
- Golden build mode (has separate logic that works correctly)