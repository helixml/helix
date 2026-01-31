# SyncBaseBranch Missing Locking Fix

**Date:** 2026-01-28
**Status:** Fixed
**Related:** 2026-01-27-git-repo-serialization.md

## Incident

When starting a SpecTask with an external repository that had upstream changes, the task failed with:

```
failed to sync base branch 'main' for repository 'zed-4': failed to fetch from remote: fetch failed: exit status 1 -
From https://github.com/helixml/zed
4869a760c0..06002c07a5 main -> origin/main
error: fetching ref refs/remotes/origin/main failed: incorrect old value provided -
From https://github.com/helixml/zed
4869a760c0..06002c07a5 main -> origin/main
error: fetching ref refs/remotes/origin/main failed: incorrect old value provided -
From https://github.com/helixml/zed
4869a760c0..06002c07a5 main -> origin/main
error: fetching ref refs/remotes/origin/main failed: incorrect old value provided
```

## Root Cause Analysis

### The Error

The git error "incorrect old value provided" occurs during atomic ref updates when:
1. Git reads the current ref value (e.g., from packed-refs or loose ref)
2. Git tries to update the ref using compare-and-swap (CAS)
3. Between steps 1 and 2, another process modified the ref
4. The CAS fails because the "old" value doesn't match

The error was repeated 3 times because git internally retries ref updates.

### Missing Locking

The "complete locking audit" on 2026-01-27 (commit 068236b15) was **incomplete**. It missed 4 functions that had existed for over a month:

| Function | Added | Missed in Audit |
|----------|-------|-----------------|
| `GetExternalRepoStatus` | Dec 10, 2025 | Yes |
| `PullFromRemote` | Dec 10, 2025 | Yes |
| `SyncBaseBranch` | Dec 17, 2025 | Yes |
| `PushPullRequest` | ~Dec 2025 | Yes |

The Jan 27 audit only covered HTTP handler entry points (`handleUploadPack`, `pushToRemoteGitRepository`, etc.) but missed direct service-layer entry points.

**Call paths without locking:**
1. `SpecDrivenTaskService.startTaskExecution` → `SyncBaseBranchForTask` → `SyncBaseBranch`
2. `SpecDrivenTaskService.runTaskFromPending` → `SyncBaseBranchForTask` → `SyncBaseBranch`
3. `SpecDrivenTaskService.ApproveSpecs` → `SyncBaseBranch`

None of these entry points acquired the repo lock before calling `SyncBaseBranch`.

### Why It Happened Now

The error occurred when:
1. The upstream repo (helixml/zed) had new commits pushed
2. Multiple UI/API operations triggered concurrent git operations on the same local repo
3. One operation (e.g., a git read via GitHTTPServer) updated `refs/remotes/origin/main`
4. The `SyncBaseBranch` call then tried to update the same ref concurrently
5. The atomic ref update failed due to the race

## The Fix

Added repo locking to `SyncBaseBranch` in `git_repository_service_pull.go`:

```go
func (s *GitRepositoryService) SyncBaseBranch(ctx context.Context, repoID, branchName string) error {
    // ... validation ...

    // Acquire repo lock to serialize git operations on this repository.
    // This prevents race conditions where concurrent syncs could cause
    // "incorrect old value provided" errors during ref updates.
    lock := s.GetRepoLock(repoID)
    lock.Lock()
    defer lock.Unlock()

    // ... fetch and update logic ...
}
```

### Why Lock Inside SyncBaseBranch

Unlike `SyncAllBranches` which is called from locked contexts (callers hold lock), `SyncBaseBranch` is an **entry point** that is called without locks:

| Function | Called From | Caller Holds Lock? |
|----------|-------------|-------------------|
| `SyncAllBranches` | `WithExternalRepoRead`, `handleReceivePack`, etc. | **Yes** |
| `SyncBaseBranch` | `ApproveSpecs`, `SyncBaseBranchForTask` | **No** |

Therefore, `SyncBaseBranch` must acquire the lock internally.

### No Reentrancy Risk

Verified that `SyncBaseBranch` is never called from a locked context:
- `SyncBaseBranchForTask` iterates over repos and calls `SyncBaseBranch` for each
- Each call uses a different repoID, so acquires a different lock
- No caller holds the lock before calling `SyncBaseBranch`

## Repo State After Incident

Despite the error, the repository was eventually updated correctly:
- `refs/heads/main` = `06002c07a5db74ca26d2201f46d20eb7ca44f20d` (new commit)
- `refs/remotes/origin/main` = `06002c07a5db74ca26d2201f46d20eb7ca44f20d` (new commit)
- `packed-refs` still had old value for main, but loose refs take precedence

## Files Modified

- `api/pkg/services/git_repository_service_pull.go` - Added locking to `SyncBaseBranch`, `PullFromRemote`
- `api/pkg/services/git_repository_service.go` - Added locking to `PushPullRequest`, `GetExternalRepoStatus`
- `api/pkg/services/git_integration_test.go` - Added `TestSyncBaseBranch_AcquiresLock`, `TestSyncBaseBranch_NoDeadlockFromEntryPoint`

## Testing

All git integration tests pass with `-race` flag:
- `TestLockSerializesOperations` - confirms serialization
- `TestNoReentrancy_NestedLockWouldDeadlock` - confirms no deadlock
- `TestConcurrentPushes_DifferentFiles` - confirms concurrent safety

## Future Prevention

When adding new git sync/fetch functions:
1. Check if the function is an **entry point** (called without lock) or **inner function** (called with lock)
2. Entry points must acquire the lock internally
3. Inner functions must NOT acquire the lock (to avoid deadlock)
4. Add to the locking audit table in design docs
5. Add integration test if appropriate

## Complete Locking Audit (2026-01-28)

### Entry Point Functions (acquire lock internally)

| Function | File | Lock Added | Notes |
|----------|------|------------|-------|
| `SyncBaseBranch` | git_repository_service_pull.go | **Yes** | Called from ApproveSpecs, SyncBaseBranchForTask |
| `PullFromRemote` | git_repository_service_pull.go | **Yes** | Called from pullFromRemoteGitRepository handler |
| `PushPullRequest` | git_repository_service.go | **Yes** | Called from pushPullRequest handler |
| `GetExternalRepoStatus` | git_repository_service.go | **Yes** | Called from getRepositoryStatus handler, does fetch |
| `WithExternalRepoRead` | git_external_sync.go | Yes (existing) | Wrapper for read operations |
| `WithExternalRepoWrite` | git_external_sync.go | Yes (existing) | Wrapper for write operations |
| `MustSyncBeforeRead` | git_external_sync.go | Yes (existing) | Wrapper for strict reads |

### Inner Functions (callers hold lock - do NOT add lock)

| Function | File | Notes |
|----------|------|-------|
| `SyncAllBranches` | git_repository_service_pull.go | Called from locked contexts |
| `PushBranchToRemote` | git_repository_service_push.go | Called from locked contexts |
| `CreatePullRequest` | git_repository_service_pull_requests.go | Called from locked contexts |
| `CreateOrUpdateFileContents` | git_repository_service_contents.go | Called from WithExternalRepoWrite |
| `CreateBranch` | git_repository_service.go | Called from WithExternalRepoWrite |

### Read-Only Functions (no locking needed)

| Function | File | Notes |
|----------|------|-------|
| `BrowseTree` | git_repository_service_contents.go | Read only, no sync |
| `GetFileContents` | git_repository_service_contents.go | Read only, no sync |
| `ListCommits` | git_repository_commits.go | Read only |
| `ListBranches` | git_repository_service.go | Read only |
| `GetRepository` | git_repository_service.go | Read only (DB + local git) |
