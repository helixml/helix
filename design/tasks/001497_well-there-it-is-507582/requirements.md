# Requirements: Investigate 507K Zombie Git Processes

## Status: CLOSED — Root cause was external

## Actual Root Cause

The 507,582 zombie processes were caused by **a stray old Python kodit container**, not by Helix's git subprocess handling. Killing the container resolved the issue. No Helix code changes are needed.

## Investigation Findings (preserved for future reference)

The investigation did surface real (but non-critical) code hygiene issues worth noting for future work:

### Desktop `/diff` endpoint — no context propagation
- The frontend `useLiveFileDiff` hook polls every **3 seconds** per open session
- Each `/diff` request spawns **10+ git subprocesses**: `git rev-parse`, `git status`, `git merge-base`, `git diff --numstat`, `git diff --name-status`, `git ls-files`, plus per-file `git diff` when `includeContent=true`
- All use raw `exec.Command("git", ...)` with **no context and no timeout**
- Located in `api/pkg/desktop/diff.go`: `handleDiff()`, `getWorkspaceInfo()`, `generateHelixSpecsDiff()`, `findHelixSpecsWorktree()`, `resolveBaseBranch()`
- These processes do complete quickly in practice, so they aren't accumulating to dangerous levels — but they lack defense-in-depth

### Server-side spec task handlers — `exec.Command` without context
- `readDesignDocsFromGit()` in `spec_task_orchestrator_handlers.go` uses raw `exec.Command` (no context)
- `backfillDesignReviewFromGit()` in `spec_task_design_review_handlers.go` uses raw `exec.Command` (no context)
- A pure-Go `GitRepo` wrapper already exists in `services/git_helpers.go` with `ListFilesInBranch()`, `ReadFileFromBranch()`, `GetBranchCommitHash()` — these could replace the subprocess calls

### Potential future improvement: event-driven diff
- Blind 3-second polling could be replaced with `fsnotify` watching + WebSocket push notifications
- `fsnotify` is already a dependency (used in `settings-sync-daemon`)
- The desktop server already has WebSocket infrastructure (`/ws/stream`, `/ws/input`)
- This would reduce git subprocess volume from ~200/min/session to near-zero during idle periods

## Conclusion

None of these issues caused the observed 507K zombie processes. They are minor hygiene improvements that can be picked up opportunistically if the diff polling ever becomes a real bottleneck.