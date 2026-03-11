# Implementation Tasks

## Status: CLOSED — No implementation needed

The 507,582 zombie processes were caused by a stray old Python kodit container, not by Helix's git subprocess handling. Killing the container resolved the issue.

## Future hygiene improvements (optional, not scheduled)

- [ ] Add `exec.CommandContext` with request context and timeout to all `exec.Command("git", ...)` calls in `api/pkg/desktop/diff.go` (~20 call sites)
- [ ] Migrate `readDesignDocsFromGit()` and `backfillDesignReviewFromGit()` from `exec.Command("git", ...)` to the existing pure-Go `GitRepo` wrapper in `services/git_helpers.go`
- [ ] Replace blind 3-second diff polling with event-driven `fsnotify` watcher + `/ws/diff` WebSocket push notifications