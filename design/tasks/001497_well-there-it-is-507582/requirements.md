# Requirements: Fix Zombie Git Process Accumulation

## Problem Statement

507,582 zombie git processes accumulate over time, representing 99.8% of all processes. The root cause is unmanaged `exec.Command("git", ...)` subprocess spawning across multiple hot paths without timeouts, context cancellation, or process reaping.

## Root Cause Analysis

Three distinct sources of zombie git processes have been identified:

### 1. Desktop `/diff` endpoint — polling hot path (PRIMARY CAUSE)
- The frontend `useLiveFileDiff` hook polls every **3 seconds** per open session
- Each `/diff` request spawns **10+ git subprocesses**: `git rev-parse`, `git status`, `git merge-base`, `git diff --numstat`, `git diff --name-status`, `git ls-files`, plus per-file `git diff` when `includeContent=true`
- All use raw `exec.Command("git", ...)` with **no context, no timeout, no cleanup**
- Located in `api/pkg/desktop/diff.go`: `handleDiff()`, `getWorkspaceInfo()`, `generateHelixSpecsDiff()`, `findHelixSpecsWorktree()`, `resolveBaseBranch()`
- With N browser tabs open, this generates ~3.3 × N × 10 = **33N git processes per second**

### 2. Server-side spec task handlers — `exec.Command` without context
- `readDesignDocsFromGit()` in `spec_task_orchestrator_handlers.go` spawns `git ls-tree` + N × `git show` per design doc, using raw `exec.Command` (no context)
- `backfillDesignReviewFromGit()` in `spec_task_design_review_handlers.go` spawns `git rev-parse` + `git ls-tree` + 3 × `git show`, also raw `exec.Command`
- These run on API request paths and can hang indefinitely on lock contention

## User Stories

### US-1: Eliminate zombie git processes from diff polling
As a platform operator, I need the `/diff` endpoint to not leak git processes so that the system remains stable under normal usage.

**Acceptance Criteria:**
- All `exec.Command("git", ...)` calls in `desktop/diff.go` use `exec.CommandContext` with a bounded timeout (e.g., 10s)
- No git process outlives its parent HTTP request
- Running `ps aux | grep git | wc -l` stays under 50 during normal operation with 5+ active sessions

### US-2: Eliminate zombie git processes from server-side git reads
As a platform operator, I need server-side git operations to not leak processes when the repo is locked or slow.

**Acceptance Criteria:**
- `readDesignDocsFromGit()` and `backfillDesignReviewFromGit()` use `exec.CommandContext` with the request context (or a derived timeout context)
- Alternatively, migrate these to the existing pure-Go `GitRepo` wrapper in `services/git_helpers.go` which doesn't spawn subprocesses

### US-3: Reduce git subprocess volume from diff polling
As a platform operator, I need the diff endpoint to spawn fewer processes per request so that even with many sessions, the system isn't overwhelmed.

**Acceptance Criteria:**
- Consider using the pure-Go `GitRepo` (go-git/gitea) wrapper for read-only operations like `ls-tree` and `show` instead of spawning subprocesses
- OR batch multiple git queries into fewer subprocess invocations
- OR increase the frontend poll interval for diffs (currently 3s is aggressive)

## Out of Scope

- Replacing all `gitcmd.NewCommand` usage (these already go through Gitea's git wrapper which has timeouts)
- Changing the git HTTP server's `upload-pack`/`receive-pack` handling (these are properly managed by `gitcmd.Run` with context)
- Adding process-level monitoring for git zombies (unnecessary if the root cause is fixed)