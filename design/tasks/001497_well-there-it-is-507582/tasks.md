# Implementation Tasks

## Phase 1: Fix the primary leak — desktop diff.go (highest impact)

- [ ] Create a `gitCommand(ctx context.Context, dir string, args ...string) ([]byte, error)` helper in `api/pkg/desktop/diff.go` that wraps `exec.CommandContext` with a 10-second timeout derived from the passed context
- [ ] Refactor `handleDiff()` to accept and propagate `r.Context()` to all git subprocess calls, replacing every `exec.Command("git", ...)` with the new `gitCommand()` helper
- [ ] Refactor `resolveBaseBranch()` to accept a `context.Context` parameter and use `gitCommand()` for its `git rev-parse --verify` loop
- [ ] Refactor `getWorkspaceInfo()` to accept a `context.Context` parameter and use `gitCommand()` for `git rev-parse`, `git show-ref` calls
- [ ] Refactor `generateHelixSpecsDiff()` to accept a `context.Context` parameter and use `gitCommand()` for all git subprocess calls (status, diff, ls-files, etc.)
- [ ] Refactor `findHelixSpecsWorktree()` to accept a `context.Context` parameter and use `gitCommand()` for `git worktree list` and `git rev-parse` calls
- [ ] Update all callers of the refactored functions to pass the appropriate context
- [ ] Verify existing tests in `api/pkg/desktop/diff_test.go` still pass
- [ ] Add a test that cancels the context mid-operation and verifies no git processes are leaked

## Phase 2: Migrate server-side reads to pure-Go GitRepo (eliminate subprocesses)

- [ ] Refactor `readDesignDocsFromGit()` in `api/pkg/server/spec_task_orchestrator_handlers.go` to use `services.OpenGitRepo()` + `GitRepo.ListFilesInBranch()` + `GitRepo.ReadFileFromBranch()` instead of `exec.Command("git", "ls-tree", ...)` and `exec.Command("git", "show", ...)`
- [ ] Refactor `backfillDesignReviewFromGit()` in `api/pkg/server/spec_task_design_review_handlers.go` to use `services.OpenGitRepo()` + `GitRepo.GetBranchCommitHash()` + `GitRepo.ListFilesInBranch()` + `GitRepo.ReadFileFromBranch()` instead of raw `exec.Command` calls
- [ ] Ensure `GitRepo.Close()` is called via `defer` in both refactored functions
- [ ] Verify the orchestrator and design review handler tests still pass

## Phase 3: Replace blind polling with event-driven diff (fsnotify + WebSocket)

### Container-side (desktop server)
- [ ] Create `api/pkg/desktop/diff_watcher.go` with an `fsnotify.Watcher` that monitors the git working tree and `.git/refs/` for write/create/rename events
- [ ] Implement 500ms debounce: on any filesystem event, reset a timer; when timer fires, broadcast a `{"type":"changed"}` JSON message to all registered WebSocket clients
- [ ] Add a `/ws/diff` WebSocket handler in `api/pkg/desktop/desktop.go` that upgrades the connection, registers it with the watcher's broadcast list, and sends notifications until disconnect
- [ ] Start the watcher when the desktop server starts; stop it on shutdown

### API proxy
- [ ] Add a WebSocket proxy handler for `/ws/diff` in `api/pkg/server/external_agent_handlers.go` following the same RevDial + WebSocket upgrade pattern used by the existing `/ws/stream` proxy
- [ ] Register the new route in `api/pkg/server/server.go` alongside the existing external-agent WebSocket routes

### Frontend
- [ ] Update `frontend/src/hooks/useLiveFileDiff.ts`: open a WebSocket to `/api/v1/external-agents/{sessionId}/ws/diff`, fetch the full diff on initial connect and on each `changed` notification
- [ ] Remove the `refetchInterval: 3000` polling — replace with on-demand fetches triggered by WebSocket messages
- [ ] Add fallback: if WebSocket disconnects, fall back to polling at 10s until reconnected
- [ ] Verify `DiffViewer` component works with the updated hook (no changes expected — it consumes the same `useLiveFileDiff` return shape)

## Phase 4: Verify and test

- [ ] Build the API: `cd api && go build ./pkg/server/ ./pkg/desktop/ ./pkg/services/`
- [ ] Run desktop diff tests: `cd api && go test -v ./pkg/desktop/ -count=1`
- [ ] Run frontend build: `cd frontend && yarn build`
- [ ] Deploy to dev environment and monitor `ps aux | grep git | wc -l` over 10 minutes with 3+ active sessions — count should stay under 50
- [ ] Verify that with an idle session (no file changes), zero git subprocesses are spawned for diff purposes
- [ ] Verify that editing a file in the container triggers a diff update in the frontend within ~1 second