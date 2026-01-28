# Git System Integration Tests

**Date:** 2026-01-27
**Status:** Design
**Related:** 2026-01-27-git-repo-serialization.md

## Problem

The git system has grown complex with multiple interacting components:
- GitRepositoryService (sync, push, branch operations)
- GitHTTPServer (receive-pack, upload-pack, post-push hooks)
- External repo sync (WithExternalRepoRead/Write)
- Per-repo locking for serialization
- Rollback on push failure

Current tests are sparse (~900 lines) and don't cover:
- Concurrent operations / race conditions
- Full HTTP push flow end-to-end
- Lock serialization behavior
- Rollback scenarios
- Multiple agents pushing to same branch

## Test Strategy

Use **real git operations** with **local "upstream" repos** to simulate external repos without needing GitHub/ADO credentials. This lets us:
- Run tests in CI without external dependencies
- Control timing to trigger race conditions
- Verify actual git state after operations

### Test Architecture

```
┌─────────────┐     HTTP      ┌─────────────┐    git push    ┌─────────────┐
│  Test Code  │ ────────────> │ GitHTTPServer│ ────────────> │ Local Bare  │
│  (client)   │               │  (real)      │               │ "Upstream"  │
└─────────────┘               └─────────────┘               └─────────────┘
                                    │
                                    │ uses
                                    ▼
                              ┌─────────────┐
                              │ GitRepoSvc  │
                              │  (real)     │
                              └─────────────┘
```

### Local "Upstream" Pattern

Instead of using GitHub/ADO, create a local bare repo that acts as "upstream":

```go
func setupLocalUpstream(t *testing.T) (upstreamPath string, cleanup func()) {
    upstreamPath = t.TempDir()
    
    // Initialize bare repo as "upstream"
    err := giteagit.InitRepository(ctx, upstreamPath, true, "sha1")
    require.NoError(t, err)
    
    // Create initial commit via temp clone
    tempClone := t.TempDir()
    giteagit.Clone(ctx, upstreamPath, tempClone, giteagit.CloneRepoOptions{})
    // ... add file, commit, push back to upstream
    
    return upstreamPath, func() { os.RemoveAll(upstreamPath) }
}
```

Then configure the GitRepository to use `file://` URL:

```go
repo := &types.GitRepository{
    ID:          "test-repo",
    ExternalURL: "file://" + upstreamPath,
    IsExternal:  true,
    // ...
}
```

## Test Cases

### 1. Basic Operations (Sanity)

| Test | Description |
|------|-------------|
| `TestPushToExternalRepo` | Agent pushes, verify commit in upstream |
| `TestPullFromExternalRepo` | Upstream has commit, agent fetches it |
| `TestSyncFromUpstream` | SyncAllBranches fetches latest |
| `TestBranchCreation` | Create branch, push, verify in upstream |

### 2. Concurrent Operations (Race Conditions)

| Test | Description |
|------|-------------|
| `TestConcurrentPushes_SameRepo` | Two agents push to same repo simultaneously |
| `TestConcurrentPushes_SameBranch` | Two agents push to same branch (one should fail) |
| `TestPushWhileReading` | Agent pushes while UI reads - no data loss |
| `TestReadWhilePushing` | UI reads while agent pushes - consistent state |
| `TestConcurrentSyncs` | Multiple syncs don't corrupt state |

### 3. Lock Serialization

| Test | Description |
|------|-------------|
| `TestLockSerializesOperations` | Verify operations don't interleave |
| `TestLockPerRepo` | Different repos can operate concurrently |
| `TestLockReleasedOnError` | Lock released if operation fails |
| `TestLockReleasedOnPanic` | Lock released if operation panics |

### 4. Rollback Scenarios

| Test | Description |
|------|-------------|
| `TestRollbackOnUpstreamPushFailure` | Push to upstream fails, local rolled back |
| `TestRollbackPreservesOtherBranches` | Rollback only affects pushed branch |
| `TestNoRollbackOnSuccess` | Successful push persists |

### 5. Force Sync Behavior

| Test | Description |
|------|-------------|
| `TestForceSyncOverwritesLocal` | force=true overwrites diverged local |
| `TestForceSyncUnderLock` | Force sync blocked during push |
| `TestSyncAfterPush` | Sync after push shows new commit |

### 6. Edge Cases

| Test | Description |
|------|-------------|
| `TestPushToNonExistentBranch` | Creates branch on push |
| `TestPushEmptyCommit` | Handles no-op push gracefully |
| `TestSyncEmptyRepo` | Handles empty upstream |
| `TestConcurrentPushDifferentBranches` | Should both succeed |
| `TestUpstreamChangedBetweenSyncAndPush` | Detect and handle |

## Implementation Plan

### Phase 1: Test Infrastructure

1. Create `git_integration_test.go` with:
   - `setupLocalUpstream()` helper
   - `setupMiddleRepo()` helper  
   - `setupGitHTTPServer()` helper (real server on localhost)
   - `gitClone()` / `gitPush()` client helpers

2. Create `git_test_helpers_test.go` with:
   - `waitForCondition()` for async operations
   - `assertBranchCommit()` to verify state
   - `createTestCommit()` to add commits

### Phase 2: Basic Tests

Implement sanity tests first to validate test infrastructure works.

### Phase 3: Concurrency Tests

Use `sync.WaitGroup` and channels to orchestrate concurrent operations:

```go
func TestConcurrentPushes(t *testing.T) {
    upstream, cleanup := setupLocalUpstream(t)
    defer cleanup()
    
    middle := setupMiddleRepo(t, upstream)
    server := setupGitHTTPServer(t, middle)
    
    var wg sync.WaitGroup
    results := make(chan error, 2)
    
    // Agent 1
    wg.Add(1)
    go func() {
        defer wg.Done()
        clone1 := cloneRepo(t, server.URL)
        createCommit(t, clone1, "file1.txt", "content1")
        results <- gitPush(clone1)
    }()
    
    // Agent 2
    wg.Add(1)
    go func() {
        defer wg.Done()
        clone2 := cloneRepo(t, server.URL)
        createCommit(t, clone2, "file2.txt", "content2")
        results <- gitPush(clone2)
    }()
    
    wg.Wait()
    close(results)
    
    // Verify: both pushes should succeed (different files)
    for err := range results {
        require.NoError(t, err)
    }
    
    // Verify upstream has both commits
    assertCommitExists(t, upstream, "content1")
    assertCommitExists(t, upstream, "content2")
}
```

### Phase 4: Race Condition Tests

Use `-race` flag and stress testing:

```bash
go test -race -count=100 ./pkg/services/... -run TestConcurrent
```

## Files to Create

1. `api/pkg/services/git_integration_test.go` - Main integration tests
2. `api/pkg/services/git_test_helpers_test.go` - Test utilities
3. `api/pkg/services/git_concurrent_test.go` - Concurrency-specific tests

## Success Criteria

- All tests pass with `-race` flag
- Tests run in CI without external credentials
- Tests catch the race condition we just fixed (if we remove the locks)
- < 30 seconds total runtime for the suite
