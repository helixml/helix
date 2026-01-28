# Per-Repository Git Operation Serialization

**Date:** 2026-01-27
**Status:** Implementation
**Related:** 2026-01-27-helix-specs-sync-divergence.md

## Problem

Concurrent git operations on the same repository cause race conditions. Specifically:

1. Agent pushes commit A to helix-specs (receive-pack completes, local updated)
2. Before upstream push completes, a UI read triggers `SyncAllBranches(force=true)`
3. Force sync overwrites local ref back to old GitHub state
4. Upstream push sends the old commit (no-op)
5. Agent's commit A is silently lost

**Evidence from logs (2026-01-27):**
```
16:29:11-13Z - Multiple "Syncing from upstream before read" (force=true)
16:29:22Z   - Agent 001023 receive-pack completes (commit 1a8a3cdb8)
16:29:23Z   - Post-push hook shows wrong commit (362140a75)
```

## Solution

Serialize all git operations per-repository using a mutex map. Each compound operation (push, read, write) acquires a lock before starting and holds it until completion.

## Implementation

### 1. Add lock map to GitRepositoryService

```go
type GitRepositoryService struct {
    // ... existing fields
    repoLocks map[string]*sync.Mutex
    locksMu   sync.Mutex
}

func (s *GitRepositoryService) getRepoLock(repoID string) *sync.Mutex {
    s.locksMu.Lock()
    defer s.locksMu.Unlock()
    
    if s.repoLocks == nil {
        s.repoLocks = make(map[string]*sync.Mutex)
    }
    
    lock, ok := s.repoLocks[repoID]
    if !ok {
        lock = &sync.Mutex{}
        s.repoLocks[repoID] = lock
    }
    return lock
}
```

### 2. Wrap compound operations

**WithExternalRepoRead:**
```go
func (s *GitRepositoryService) WithExternalRepoRead(...) error {
    lock := s.getRepoLock(repo.ID)
    lock.Lock()
    defer lock.Unlock()
    // ... existing logic
}
```

**WithExternalRepoWrite:**
```go
func (s *GitRepositoryService) WithExternalRepoWrite(...) error {
    lock := s.getRepoLock(repo.ID)
    lock.Lock()
    defer lock.Unlock()
    // ... existing logic
}
```

### 3. Wrap git HTTP push flow

The push flow in `handleReceivePack` needs to be atomic:
- Pre-push sync
- receive-pack
- Post-push to upstream

```go
func (s *GitHTTPServer) handleReceivePack(...) {
    // Acquire lock before any git operations
    lock := s.gitRepoService.getRepoLock(repoID)
    lock.Lock()
    defer lock.Unlock()
    
    // ... existing pre-sync, receive-pack, post-push logic
}
```

## Operations Serialized

| Operation | Entry Point | Lock Scope |
|-----------|-------------|------------|
| Agent push | `handleReceivePack` | pre-sync + receive-pack + post-push |
| UI read | `WithExternalRepoRead` | sync + read |
| UI write | `WithExternalRepoWrite` | pre-sync + write + post-push |
| Strict read | `MustSyncBeforeRead` | sync + read |

## Why Always `force=true`

The Helix middle repo mirrors upstream (GitHub/ADO) state:
- Upstream is the source of truth
- The middle repo should match upstream exactly, except during active push operations
- Any divergence is temporary and protected by locks

With per-repo locks in place, the middle repo is always in one of two states:
1. **Idle**: Matches upstream exactly → `force=true` is correct
2. **Mid-push**: Holding lock, has unpushed commit → No sync can run (blocked by lock)

Therefore, `force=true` is always correct:
- If we're syncing, we want upstream's state
- If we have a local commit, the lock prevents any sync from running until it's pushed

Using `force=false` was confusing because it implied we might want to preserve local state that differs from upstream. But that state should never exist outside of a locked push operation.

## Trade-offs

**Pros:**
- Eliminates all race conditions between git operations
- Simple, easy to reason about
- No changes to git behavior, just ordering
- Consistent mental model: middle repo mirrors upstream

**Cons:**
- Serializes all operations per-repo (no concurrent reads)
- Could add latency if many operations queue up

**Mitigation:**
- For reads, could use RWMutex (multiple readers, single writer)
- But force sync during read IS a write, so Mutex is correct for now
- Can optimize later if needed

## Files Modified

1. `api/pkg/services/git_repository_service.go` - Added `repoLocks` map, `GetRepoLock()`, and `WithRepoLock()` helper
2. `api/pkg/services/git_external_sync.go` - Wrapped `WithExternalRepoRead`, `WithExternalRepoWrite`, `MustSyncBeforeRead` with locks
3. `api/pkg/services/git_http_server.go` - Wrapped `handleReceivePack` and `handleUploadPack` with locks
4. `api/pkg/server/git_repository_handlers.go` - Wrapped explicit `SyncAllBranches` API call with lock
5. `api/pkg/server/spec_task_workflow_handlers.go` - Wrapped merge flow (sync → merge → push) with lock

## Operations Now Serialized

| Operation | Entry Point | Lock Scope |
|-----------|-------------|------------|
| Agent push | `handleReceivePack` | pre-sync + receive-pack + post-push |
| Agent fetch | `handleUploadPack` | sync only (then releases for upload-pack) |
| UI read | `WithExternalRepoRead` | sync + read |
| UI write | `WithExternalRepoWrite` | pre-sync + write + post-push |
| Strict read | `MustSyncBeforeRead` | sync + read |
| Explicit sync API | `syncAllBranchesHandler` | sync |
| Server-side merge | `approveImplementationHandler` | sync + merge + push |
| Push to remote API | `pushToRemoteGitRepository` | push |
| Create PR API | `createPullRequest` | push + PR creation |
| Ensure PR (async) | `ensurePullRequest` | push |

## Audit Results (2026-01-27)

### Issues Found and Fixed

| Location | Issue | Fix |
|----------|-------|-----|
| `git_http_server.go:handleUploadPack` | `lock.Unlock()` without defer - not panic-safe | Wrapped in `func() { lock.Lock(); defer lock.Unlock(); ... }()` |
| `git_repository_handlers.go:pushToRemoteGitRepository` | `PushBranchToRemote` without lock | Wrapped in `WithRepoLock` |
| `git_repository_handlers.go:createPullRequest` | `PushBranchToRemote` + `CreatePullRequest` without lock | Wrapped both in `WithRepoLock` |
| `git_http_server.go:ensurePullRequest` | `PushBranchToRemote` in async goroutine without lock | Wrapped in `WithRepoLock` |

### No Reentrancy

Verified that inner functions do NOT acquire locks:
- `SyncAllBranches` - no lock calls
- `PushBranchToRemote` - no lock calls
- `CreatePullRequest` - no lock calls
- `CreateOrUpdateFileContents` - no lock calls

Lock acquisition only happens at entry points (HTTP handlers and helper wrappers).
Test `TestNoReentrancy_NestedLockWouldDeadlock` proves no deadlock.

### Panic Safety

All lock acquisitions use `defer lock.Unlock()` pattern.

### Data Race Fix

`GetRepository` was mutating fields on a shared `types.GitRepository` pointer from the store.
Fixed by making a defensive copy of the struct before mutating:
```go
storedRepo, err := s.store.GetGitRepository(ctx, repoID)
// Make defensive copy
gitRepo := *storedRepo
if storedRepo.Branches != nil {
    gitRepo.Branches = make([]string, len(storedRepo.Branches))
    copy(gitRepo.Branches, storedRepo.Branches)
}
```

## Tests

Integration tests in `api/pkg/services/git_integration_test.go`:

| Test | Purpose |
|------|---------|
| `TestPushToExternalRepo` | Basic push flow works |
| `TestSyncFromUpstream` | Sync pulls external changes |
| `TestForceSyncOverwritesDivergedLocal` | Force sync overwrites local divergence |
| `TestConcurrentPushes_DifferentFiles` | Concurrent pushes are serialized |
| `TestLockSerializesOperations` | Lock prevents concurrent locked operations |
| `TestLockPerRepo_DifferentReposConcurrent` | Different repos can operate concurrently |
| `TestPushWhileReading_NoDataLoss` | Push during read completes successfully |
| `TestNoReentrancy_NestedLockWouldDeadlock` | Proves no reentrancy deadlock |
| `TestHTTPServer_InfoRefs` | HTTP git protocol works |

All tests pass with `-race` flag.

## Force Push Protection for helix-specs

In addition to serialization, we protect the `helix-specs` branch from force pushes.
This is a forward-only branch for design documents, and force pushes could destroy
agent-generated content.

### Implementation

A pre-receive hook is installed in each bare repository that:
1. Reads ref updates from git on stdin (`<old> <new> <ref>` format)
2. Checks if `helix-specs` is being pushed
3. If old commit exists (not new branch) and is NOT an ancestor of new commit → reject

### Hook Installation

- **New repos**: Hook is installed at creation time (all code paths that create bare repos)
- **Existing repos**: Hook is installed/updated on API startup via `InstallPreReceiveHooksForAllRepos()`
- **Versioning**: Hook script includes version string; updates are applied if version differs

### Files Modified

- `api/pkg/services/gitea_git_helpers.go` - Added `InstallPreReceiveHook()` and `InstallPreReceiveHooksForAllRepos()`
- `api/pkg/services/git_repository_service.go` - Install hook after bare repo init, call bulk install on `Initialize()`
- `api/pkg/services/project_internal_repo_service.go` - Install hook after bare repo creation

### Error Message

When force push is rejected, the agent sees:
```
error: refusing to force-push to protected branch 'helix-specs'
hint: helix-specs is a forward-only branch to protect design documents.
hint: If you need to revert changes, create a new commit instead.
```

This message is intentionally similar to GitHub's protected branch error to train agents
to understand the constraint.

## Crash Recovery for External Repos

For external repos, there's a dangerous window between receive-pack completing (commit in
middle repo) and upstream push completing (commit in both places). If we crash in this
window, the commit could be lost on next sync.

### Solution

On startup, `recoverIncompletePushes()` checks all external repos for commits that exist
locally but haven't been pushed to upstream:

1. List all repos with `ExternalURL` set
2. For each repo, list all local branches
3. For each branch, check if local is ahead of `origin/<branch>`
4. If ahead, push to upstream

### Implementation

```go
// In GitRepositoryService.Initialize():
s.recoverIncompletePushes(ctx)

// Checks if local branch has commits not in origin:
// git rev-list --count origin/branch..branch
// If count > 0, push to upstream
```

### Files Modified

- `api/pkg/services/git_repository_service.go`:
  - Added `recoverIncompletePushes()` - scans repos on startup
  - Added `listLocalBranches()` - lists local branch names
  - Added `isBranchAheadOfRemote()` - checks if local is ahead of origin
  - Call `recoverIncompletePushes()` from `Initialize()`
