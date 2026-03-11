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

## Phase 3: Add git process monitoring as safety net

- [ ] Add a `countGitProcesses(tree *ProcessTree) int` method to `ProcessTracker` in `api/pkg/runner/process_monitor.go` that counts nodes whose command contains `git ` (with trailing space to avoid false positives like `digital`)
- [ ] Call `countGitProcesses()` from `scanForOrphansInternal()` after building the process tree
- [ ] Log at WARN level when git process count exceeds 100, including the count and a sample of process commands
- [ ] Optionally: identify and SIGTERM git processes older than 5 minutes that are not descendants of tracked processes (reuse existing `isProcessOrphaned` / `isProcessTooYoung` logic)

## Phase 4: Verify and test

- [ ] Build the API: `cd api && go build ./pkg/server/ ./pkg/desktop/ ./pkg/runner/ ./pkg/services/`
- [ ] Run desktop diff tests: `cd api && go test -v ./pkg/desktop/ -count=1`
- [ ] Run process monitor tests (if any): `cd api && go test -v ./pkg/runner/ -count=1`
- [ ] Deploy to dev environment and monitor `ps aux | grep git | wc -l` over 10 minutes with 3+ active sessions — count should stay under 50
- [ ] Check API logs for any new WARN-level git process count messages
- [ ] Run frontend `yarn build` to confirm no frontend changes needed