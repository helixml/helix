# Design: Fix Zombie Git Process Accumulation

## Architecture Overview

The fix targets three layers: (1) add context/timeout to all raw `exec.Command("git", ...)` calls, (2) reduce subprocess volume in the hottest path, and (3) add a safety-net monitor for git processes.

## Key Findings from Code Investigation

### Where git subprocesses are spawned

| Location | Method | Context? | Timeout? | Hot path? |
|----------|--------|----------|----------|-----------|
| `api/pkg/desktop/diff.go` — `handleDiff()` | `exec.Command` | ❌ | ❌ | **YES — polled every 3s per session** |
| `api/pkg/desktop/diff.go` — `getWorkspaceInfo()` | `exec.Command` | ❌ | ❌ | YES — called per workspace per poll |
| `api/pkg/desktop/diff.go` — `generateHelixSpecsDiff()` | `exec.Command` | ❌ | ❌ | YES — called on helix-specs diffs |
| `api/pkg/desktop/diff.go` — `findHelixSpecsWorktree()` | `exec.Command` | ❌ | ❌ | YES — called per diff request |
| `api/pkg/desktop/diff.go` — `resolveBaseBranch()` | `exec.Command` | ❌ | ❌ | YES — loops over candidates |
| `api/pkg/server/spec_task_orchestrator_handlers.go` — `readDesignDocsFromGit()` | `exec.Command` | ❌ | ❌ | Moderate — per API request |
| `api/pkg/server/spec_task_design_review_handlers.go` — `backfillDesignReviewFromGit()` | `exec.Command` | ❌ | ❌ | Moderate — backfill path |
| `api/pkg/services/git_helpers.go` — various | `gitcmd.NewCommand` | ✅ | ✅ | Low — uses Gitea wrapper |
| `api/pkg/services/git_http_server.go` — upload/receive-pack | `gitcmd.NewCommand` | ✅ | ✅ | Low — managed by Gitea |
| `api/pkg/git/git_manager.go` — Diff | `exec.CommandContext` | ✅ | via ctx | Low |

### Why zombies accumulate

1. **No context propagation**: `exec.Command("git", ...)` ignores the HTTP request context. When a client disconnects or a new poll fires, the old git processes keep running.
2. **No timeout**: If a git operation blocks on a lock (`index.lock`), it hangs forever.
3. **High frequency**: The frontend polls `/diff` every 3 seconds. Each poll spawns 10+ git subprocesses. With 5 sessions open in the UI, that's ~150 git process spawns per second.
4. **No reaping**: The existing process monitor (`process_monitor.go`) only watches for VLLM/Ollama patterns. Git zombies are invisible.

### Existing patterns to follow

- `api/pkg/services/git_helpers.go` has a `GitRepo` wrapper using go-git (pure Go, no subprocesses) with methods like `ListFilesInBranch()`, `ReadFileFromBranch()`, `GetBranchCommitHash()`. These are the right tools for the server-side read operations.
- `api/pkg/git/git_manager.go` correctly uses `exec.CommandContext(ctx, "git", ...)` — this is the pattern to follow for desktop-side operations where pure Go isn't available.
- `gitcmd.NewCommand(...).RunStdString(ctx, &gitcmd.RunOpts{...})` is the Gitea wrapper that already handles timeouts.

## Design Decisions

### Decision 1: Add `exec.CommandContext` with timeout to all desktop/diff.go calls

**Approach**: Thread the `http.Request` context through all git helper functions in `diff.go`. Create a helper that wraps `exec.CommandContext` with a 10-second timeout derived from the request context.

```go
func gitCommand(ctx context.Context, dir string, args ...string) ([]byte, error) {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    cmd := exec.CommandContext(ctx, "git", args...)
    cmd.Dir = dir
    return cmd.Output()
}
```

**Rationale**: This is the minimal, surgical fix. Every function in `diff.go` that currently calls `exec.Command` gets refactored to use this helper. When the HTTP request is cancelled (client disconnects, new poll supersedes), the context cancellation kills the git process.

**Scope of change**: ~20 call sites in `diff.go`, all following the same mechanical transformation.

### Decision 2: Migrate server-side reads to pure-Go GitRepo

**Approach**: Replace `exec.Command("git", "ls-tree", ...)` and `exec.Command("git", "show", ...)` in `readDesignDocsFromGit()` and `backfillDesignReviewFromGit()` with the existing `GitRepo.ListFilesInBranch()` and `GitRepo.ReadFileFromBranch()` methods from `services/git_helpers.go`.

**Rationale**: These are bare-repo read operations that the pure-Go library handles perfectly. Zero subprocess overhead. The `GitRepo` wrapper already exists and is tested.

**Scope of change**: 2 functions in 2 files. The `GitRepo` wrapper is already imported elsewhere in the services package.

### Decision 3: Add git process counting to the orphan monitor

**Approach**: Add a lightweight git-process counter to the existing `orphanMonitorLoop()` in `process_monitor.go`. Every 30 seconds (existing interval), count processes matching `git` in the process tree. Log a warning if count exceeds a threshold.

**Rationale**: Defense in depth. Even after fixing the known leak sources, we want early warning if new ones appear. Reusing the existing monitor avoids adding new goroutines or timers.

**Implementation**: Add a `countGitProcesses()` method that scans `tree.Nodes` for command strings containing `git ` (note the space, to avoid matching e.g. `digital`). Log at WARN level if count > 100. Optionally SIGTERM git processes older than 5 minutes that aren't children of tracked processes.

### Decision 4: Do NOT change the frontend poll interval

**Rationale**: 3-second polling is a product requirement for "live" diff updates. The fix should be on the backend — making each poll cheap (via context cleanup and potentially pure-Go reads), not by degrading the UX.

## File Change Summary

| File | Change |
|------|--------|
| `api/pkg/desktop/diff.go` | Add `gitCommand()` helper. Refactor all `exec.Command("git", ...)` → `gitCommand(ctx, dir, ...)`. Thread `r.Context()` through `handleDiff`, `resolveBaseBranch`, `getWorkspaceInfo`, `generateHelixSpecsDiff`, `findHelixSpecsWorktree`. |
| `api/pkg/server/spec_task_orchestrator_handlers.go` | Replace `exec.Command("git", ...)` in `readDesignDocsFromGit()` with `services.OpenGitRepo()` + `ListFilesInBranch()` + `ReadFileFromBranch()`. |
| `api/pkg/server/spec_task_design_review_handlers.go` | Replace `exec.Command("git", ...)` in `backfillDesignReviewFromGit()` with `services.OpenGitRepo()` + pure-Go equivalents. |
| `api/pkg/runner/process_monitor.go` | Add `countGitProcesses()` method. Call it from `scanForOrphansInternal()`. Log warning if threshold exceeded. Optionally reap stale git processes. |

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| 10s timeout too short for large repos | Start with 10s, log timeouts at WARN level so we can tune. Most git read ops complete in <100ms. |
| Context cancellation leaves `index.lock` files | Read-only git operations (`rev-parse`, `status`, `diff`, `ls-tree`, `show`) don't create lock files. Only write operations do, and those aren't in the affected paths. |
| Pure-Go git library behaves differently than CLI | The `GitRepo` wrapper is already used for the same operations in other code paths (`FindTaskDirInBranch`, `ReadDesignDocs`). Battle-tested. |
| Reaping git processes could kill legitimate long-running operations | Only reap processes older than 5 minutes. No legitimate git read operation takes that long. Push/fetch operations go through `gitcmd` which has its own timeout management. |