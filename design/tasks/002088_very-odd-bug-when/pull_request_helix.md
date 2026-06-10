# fix(api): populate project context when worker starts first session

## Summary

When a Helix-OR worker is hired and its first session is launched, the
desktop container never finishes initializing — `HELIX_REPOSITORIES`
lands empty, `helix-workspace-setup.sh` aborts, the
`/home/retro/.helix-setup-failed` marker is written, and
`start-zed-helix.sh` loops on the setup-waiting check forever.

Root cause: `StartExternalAgentSession` (the worker-hire entry point)
built a `DesktopAgent` without `ProjectID`, `RepositoryIDs`, or
`PrimaryRepositoryID`. Two near-identical lookups already existed at
`startDevContainerForSession` and the exploratory-resume path, plus a
slight variant in the spec-task service — all of which worked correctly.
Only the worker-hire site had been missed.

Auto-wake retries didn't recover the stuck session because hydra
short-circuits with "Dev container already running, returning existing
session" before re-applying env vars. The fix has to be at the caller.

## Changes

- New pure method `(*types.DesktopAgent).SetRepoContext(repos, defaultRepoID)`
  in `api/pkg/types/types.go` — no I/O, callable from any package.
- New `(s *HelixAPIServer).attachProjectContext(ctx, agent, projectID)` in
  `api/pkg/server/session_handlers.go` — does the `ListGitRepositories` +
  `GetProject` lookups and delegates to `SetRepoContext`.
- `StartExternalAgentSession` now calls `attachProjectContext` after
  constructing the `DesktopAgent` (the bug fix).
- `startDevContainerForSession` (`spec_task_design_review_handlers.go:967`)
  and the exploratory-resume path (`session_handlers.go:1991`) both
  replaced with the helper. The exploratory-resume path keeps its
  separate `GetProject` 500-on-missing check as a precondition.
- `spec_driven_task_service.go` uses `SetRepoContext` directly (it already
  has `projectRepos` loaded for `SyncBaseBranchForTask`, so no extra DB
  call is needed).
- Surfaced `ListGitRepositories` errors loudly — the old inline code
  silently swallowed them and launched a broken container.

## Tests

- 5 pure-value tests for `SetRepoContext` (empty repos no-op, default
  wins, fallback to first repo, nil/empty-id filtering, all-filtered no-op).
- 5 `MockStore`-driven tests for `attachProjectContext` (happy path,
  empty `projectID` no-op, no repos leaves repo fields unset,
  `ListGitRepositories` error returned, `GetProject` error tolerated
  with first-repo fallback).
- Full `go build ./api/...` clean.

## Test plan

- [ ] CI green (Drone)
- [ ] Hire a Helix-OR worker against a project with attached repos, watch
      first session start, confirm `HELIX_REPOSITORIES` is populated in the
      container env and Zed launches without the
      `/home/retro/.helix-setup-failed` marker appearing.
