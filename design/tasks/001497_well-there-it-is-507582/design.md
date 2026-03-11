# Design: Fix Zombie Git Process Accumulation

## Architecture Overview

The fix targets two layers: (1) add context/timeout to all raw `exec.Command("git", ...)` calls in the desktop diff hot path, and (2) migrate server-side reads to the existing pure-Go git library to eliminate subprocesses entirely.

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

### Decision 3: Replace blind polling with event-driven diff via fsnotify + WebSocket

**Problem**: Even with context cleanup, 3-second polling spawns ~200 git subprocesses/min/session during idle periods when nothing has changed. The fix should eliminate unnecessary work, not just make it safer.

**Approach**: Add an `fsnotify` watcher inside the desktop container server (`api/pkg/desktop/`) that watches the git working tree and `.git/refs/` for changes. When changes are detected (debounced ~500ms), push a lightweight notification over a new `/ws/diff` WebSocket endpoint. The frontend subscribes to this WebSocket and only calls the existing `/diff` REST endpoint when notified.

**Why this works well here**:
- `fsnotify` is already a dependency (used by `settings-sync-daemon` for watching Zed settings files)
- The desktop server already has WebSocket infrastructure (`/ws/stream` for video, `/ws/input` for input)
- The existing `/diff` REST endpoint stays unchanged — it just gets called on-demand instead of blind-polled
- During idle sessions, zero git subprocesses are spawned for diff purposes

**Container-side implementation** (`api/pkg/desktop/`):
1. New `diff_watcher.go`: `fsnotify.Watcher` on the working tree + `.git/refs/`. On any write/create/rename event, debounce 500ms then broadcast a `{"type":"changed"}` JSON message to all connected WebSocket clients.
2. New `/ws/diff` handler in `desktop.go`: upgrades to WebSocket, registers with the watcher's broadcast list, sends notifications until disconnect.

**Frontend implementation** (`frontend/src/hooks/useLiveFileDiff.ts`):
1. Open a WebSocket to `/api/v1/external-agents/{sessionId}/ws/diff` (proxied through the API the same way `/ws/stream` is)
2. On receiving a `changed` message, call the existing `/diff` REST endpoint (same `v1ExternalAgentsDiffDetail` call)
3. Also fetch on initial connect (to get current state)
4. Fallback: if WebSocket disconnects, fall back to polling at 10s until reconnected

**Rationale over alternatives**:
- Pure polling with longer interval: still wastes work during idle, and slower updates during active coding
- Piggybacking on `/ws/stream`: wrong lifecycle — video may be paused while user views diff panel
- Git hooks (post-commit, post-checkout): only catches git operations, misses uncommitted file edits which are the primary thing the diff viewer shows

## File Change Summary

| File | Change |
|------|--------|
| `api/pkg/desktop/diff.go` | Add `gitCommand()` helper. Refactor all `exec.Command("git", ...)` → `gitCommand(ctx, dir, ...)`. Thread `r.Context()` through `handleDiff`, `resolveBaseBranch`, `getWorkspaceInfo`, `generateHelixSpecsDiff`, `findHelixSpecsWorktree`. |
| `api/pkg/server/spec_task_orchestrator_handlers.go` | Replace `exec.Command("git", ...)` in `readDesignDocsFromGit()` with `services.OpenGitRepo()` + `ListFilesInBranch()` + `ReadFileFromBranch()`. |
| `api/pkg/server/spec_task_design_review_handlers.go` | Replace `exec.Command("git", ...)` in `backfillDesignReviewFromGit()` with `services.OpenGitRepo()` + pure-Go equivalents. |
| `api/pkg/desktop/diff_watcher.go` | **New file.** `fsnotify` watcher on working tree + `.git/refs/`. Debounced broadcast to WebSocket clients. |
| `api/pkg/desktop/desktop.go` | Register `/ws/diff` handler. |
| `api/pkg/server/external_agent_handlers.go` | Add proxy handler for `/ws/diff` WebSocket (same pattern as existing `/ws/stream` proxy). |
| `api/pkg/server/server.go` | Register the new `/ws/diff` route. |
| `frontend/src/hooks/useLiveFileDiff.ts` | Replace `refetchInterval: 3000` with WebSocket subscription to `/ws/diff`. Fetch on notification + initial connect. Fallback to 10s poll on disconnect. |

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| 10s timeout too short for large repos | Start with 10s, log timeouts at WARN level so we can tune. Most git read ops complete in <100ms. |
| Context cancellation leaves `index.lock` files | Read-only git operations (`rev-parse`, `status`, `diff`, `ls-tree`, `show`) don't create lock files. Only write operations do, and those aren't in the affected paths. |
| Pure-Go git library behaves differently than CLI | The `GitRepo` wrapper is already used for the same operations in other code paths (`FindTaskDirInBranch`, `ReadDesignDocs`). Battle-tested. |
| `fsnotify` floods during `git checkout` or large writes | 500ms debounce collapses rapid events into a single notification. The `/diff` endpoint is idempotent — extra calls are just redundant, not harmful. |
| `fsnotify` misses changes (e.g., NFS, bind mounts) | Fallback polling at 10s ensures the UI eventually catches up. This is strictly better than the current 3s blind polling. |
| WebSocket proxy adds complexity | Follow the exact pattern used by `/ws/stream` — the proxy code in `external_agent_handlers.go` already handles RevDial + WebSocket upgrade. |