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

### US-3: Replace blind polling with event-driven diff updates
As a platform operator, I need the diff system to only do work when files actually change, instead of spawning 10+ git subprocesses every 3 seconds per session regardless of activity.

**Context:** The container already uses `fsnotify` (dependency exists — used in `settings-sync-daemon`) and already has a WebSocket infrastructure (`/ws/stream` for video). The desktop server in the container (`api/pkg/desktop/`) is the right place to watch for filesystem changes and push notifications.

**Approach:**
1. Add an `fsnotify` watcher in the desktop server that monitors the working tree and `.git/refs/` for changes (file writes, branch switches, commits)
2. Expose a new `/ws/diff` WebSocket endpoint on the desktop server that pushes a lightweight "files changed" notification when the watcher fires (debounced ~500ms)
3. The frontend subscribes to `/ws/diff` and only calls the existing `/diff` REST endpoint when it receives a change notification (or on initial connect)
4. Keep the existing `/diff` REST endpoint unchanged — it still does the full git diff computation, but now it's called on-demand instead of every 3 seconds

**Acceptance Criteria:**
- A new `fsnotify` watcher in the desktop server watches the git working tree and `.git/refs/` for changes
- A new `/ws/diff` WebSocket endpoint pushes change notifications to connected clients (debounced to avoid flooding during rapid edits)
- The frontend `useLiveFileDiff` hook subscribes to `/ws/diff` and fetches the full diff only on notification, replacing the 3-second `refetchInterval`
- Fallback: if the WebSocket disconnects, the frontend falls back to polling at a slower rate (e.g., 10s) until reconnected
- With an idle session (no file changes), zero git subprocesses are spawned for diff purposes
- Git subprocess volume drops from ~200/min/session (current: 10+ procs × 20 polls/min) to near-zero during idle periods

## Out of Scope

- Replacing all `gitcmd.NewCommand` usage (these already go through Gitea's git wrapper which has timeouts)
- Changing the git HTTP server's `upload-pack`/`receive-pack` handling (these are properly managed by `gitcmd.Run` with context)
- Adding process-level monitoring for git zombies (unnecessary if the root cause is fixed)
- Piggybacking diff notifications onto the existing `/ws/stream` video WebSocket (different lifecycle — video may be paused/disconnected while diff should keep working)