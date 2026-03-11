# Design: Investigate 507K Zombie Git Processes

## Status: CLOSED — Root cause was external

The 507,582 zombie processes were caused by **a stray old Python kodit container**, not by Helix's git subprocess handling. Killing the container resolved the issue. No Helix code changes are needed.

## Code Audit (preserved for future reference)

The investigation produced a thorough audit of every `exec.Command("git", ...)` call site in the codebase. This is useful context if git subprocess handling ever needs attention.

### Where git subprocesses are spawned

| Location | Method | Context? | Timeout? | Hot path? |
|----------|--------|----------|----------|-----------|
| `api/pkg/desktop/diff.go` — `handleDiff()` | `exec.Command` | ❌ | ❌ | YES — polled every 3s per session |
| `api/pkg/desktop/diff.go` — `getWorkspaceInfo()` | `exec.Command` | ❌ | ❌ | YES — called per workspace per poll |
| `api/pkg/desktop/diff.go` — `generateHelixSpecsDiff()` | `exec.Command` | ❌ | ❌ | YES — called on helix-specs diffs |
| `api/pkg/desktop/diff.go` — `findHelixSpecsWorktree()` | `exec.Command` | ❌ | ❌ | YES — called per diff request |
| `api/pkg/desktop/diff.go` — `resolveBaseBranch()` | `exec.Command` | ❌ | ❌ | YES — loops over candidates |
| `api/pkg/server/spec_task_orchestrator_handlers.go` — `readDesignDocsFromGit()` | `exec.Command` | ❌ | ❌ | Moderate — per API request |
| `api/pkg/server/spec_task_design_review_handlers.go` — `backfillDesignReviewFromGit()` | `exec.Command` | ❌ | ❌ | Moderate — backfill path |
| `api/pkg/services/git_helpers.go` — various | `gitcmd.NewCommand` | ✅ | ✅ | Low — uses Gitea wrapper |
| `api/pkg/services/git_http_server.go` — upload/receive-pack | `gitcmd.NewCommand` | ✅ | ✅ | Low — managed by Gitea |
| `api/pkg/git/git_manager.go` — Diff | `exec.CommandContext` | ✅ | via ctx | Low |

### Existing patterns in the codebase

- **Pure-Go git reads**: `api/pkg/services/git_helpers.go` has a `GitRepo` wrapper using go-git with `ListFilesInBranch()`, `ReadFileFromBranch()`, `GetBranchCommitHash()`. These don't spawn subprocesses. The server-side `readDesignDocsFromGit()` and `backfillDesignReviewFromGit()` could be migrated to use this wrapper instead of `exec.Command`.
- **Context-aware git commands**: `api/pkg/git/git_manager.go` uses `exec.CommandContext(ctx, "git", ...)` — this is the correct pattern for subprocess calls that should respect request cancellation.
- **Gitea wrapper**: `gitcmd.NewCommand(...).RunStdString(ctx, &gitcmd.RunOpts{...})` handles timeouts and context automatically.

### Potential future improvements identified

1. **Add context to desktop diff.go**: All `exec.Command("git", ...)` calls in `diff.go` should use `exec.CommandContext` with the request context and a timeout. This is a straightforward mechanical refactor (~20 call sites).

2. **Migrate server-side reads to pure-Go GitRepo**: Replace `exec.Command("git", "ls-tree/show", ...)` in `readDesignDocsFromGit()` and `backfillDesignReviewFromGit()` with the existing `GitRepo` wrapper. Zero subprocess overhead.

3. **Event-driven diff instead of polling**: The frontend polls `/diff` every 3 seconds, spawning 10+ git subprocesses per poll. This could be replaced with `fsnotify` (already a dependency — used in `settings-sync-daemon`) watching the working tree + `.git/refs/`, pushing change notifications over a new `/ws/diff` WebSocket (the desktop server already has WebSocket infrastructure via `/ws/stream` and `/ws/input`). The frontend would fetch the full diff only when notified of actual changes.

## Conclusion

None of these code patterns caused the observed 507K zombie processes — that was a stray kodit container. The findings above are minor hygiene improvements that can be picked up opportunistically.