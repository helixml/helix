# Implementation Tasks

## Investigation & Preparation

- [ ] Add logging to capture exact error when agent push fails after upstream force-push
- [ ] Write failing test `TestAgentPushAfterUpstreamForcePush` that reproduces the bug

## Core Fix

- [ ] In `handleReceivePack`: capture middle repo branch commits BEFORE sync
- [ ] In `handleReceivePack`: after sync, detect if previous middle commit is now orphaned (force-push happened)
- [ ] Add helper `RebaseCommitsOnto(ctx, repoPath, commits []string, targetRef string) error` in `git_helpers.go`
- [ ] In `handleReceivePack`: when agent push results in orphaned commits, rebase them onto new upstream HEAD
- [ ] Update `PushBranchToRemote` to handle non-fast-forward by re-syncing and rebasing

## Error Handling

- [ ] Add clear error message when rebase fails due to conflicts
- [ ] Log warning when force-push divergence is detected
- [ ] Ensure orphaned commits are never silently dropped (log commit hashes at minimum)

## Testing

- [ ] Test: agent push succeeds after upstream force-push (no conflicts)
- [ ] Test: agent push fails gracefully when rebase has conflicts
- [ ] Test: concurrent agent pushes during force-push recovery
- [ ] Verify existing `TestForceSyncOverwritesDivergedLocal` still passes

## Cleanup

- [ ] Update any relevant comments in `git_http_server.go` about force-push handling
- [ ] Add design doc reference to code comments