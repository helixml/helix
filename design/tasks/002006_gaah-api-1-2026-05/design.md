# Design: Stop Hammering GitHub API in External PR Detection

## Where the calls come from

Two related call sites in `api/pkg/services/`:

1. `spec_task_orchestrator.go:946 detectExternalPRActivity` — runs every 30s (see `prPollLoop`, line 886). For each of up to 10 eligible tasks, calls `checkTaskForExternalPRActivity` (line 1006), which calls `ListPullRequests` once per external repo on the project (line 1029). With multiple projects sharing the same upstream repo (`helixml/helix`, `helixml/helix-next`), this is the dominant source of GitHub traffic.
2. `git_repository_service.go:282 isBranchAheadOfRemote` — called from `recoverIncompletePushes` once per local branch on each external repo at startup. This doesn't hit the GitHub REST API (it's `git fetch`), so it doesn't contribute to the 403, but it's the second log line the user is reacting to.

The `ListPullRequests` chain reaches GitHub here:
`spec_task_orchestrator.go:1029` → `git_repository_service_pull_requests.go:215 ListPullRequests` → `:604 listGitHubPullRequests` → `agent/skill/github/client.go:322 Client.ListPullRequests` → `github.com/google/go-github` `PullRequests.List`.

There is **no caching** anywhere in that chain. Every call is a fresh HTTP round-trip.

## Approach

### Layer 1 — repo-scoped per-cycle dedup (cheapest, biggest win)

In `detectExternalPRActivity`, build the unique set of `(repo.ID → []*types.PullRequest)` *once* before the per-task loop. Pass the prefetched map into `checkTaskForExternalPRActivity` instead of calling `ListPullRequests` inside it.

This alone collapses N×M calls per cycle into M calls per cycle. With 10 tasks pointing at 2 repos, that's 20 → 2 calls per 30s — a 10× reduction.

### Layer 2 — short-TTL cache on `GitRepositoryService.ListPullRequests`

Wrap the public `ListPullRequests(ctx, repoID)` with a small in-memory TTL cache keyed by `repoID`. Default TTL: 60s. This protects callers *outside* the orchestrator (handlers in `git_repository_handlers.go:1508`, `spec_task_workflow_handlers.go:603`, `git_http_server.go:1349`) from repeating the same call back-to-back during user activity.

Implementation: a struct field on `GitRepositoryService`:

```go
type prListCacheEntry struct {
    prs       []*types.PullRequest
    fetchedAt time.Time
    err       error // only cached for rate-limit errors, see Layer 3
}

type prListCache struct {
    mu      sync.Mutex
    entries map[string]prListCacheEntry
    ttl     time.Duration
}
```

Cache is per-process (no Redis). Invalidation is purely time-based — we accept up to TTL of staleness. Mutating endpoints (CreatePullRequest etc.) explicitly delete the entry for that repo.

### Layer 3 — rate-limit backoff

The `go-github` library returns `*github.RateLimitError` (or `*github.AbuseRateLimitError`) on 403 with rate-limit headers. Detect these in `listGitHubPullRequests` (the lowest layer that knows it's GitHub) and:

- Parse the `Reset` time off the error (or fall back to `now + 5m`).
- Cache an error sentinel in the same `prListCache` with `fetchedAt = reset`. Subsequent calls within the window return the cached error immediately without hitting GitHub.
- Log **once per cache-insert** at WARN with `reset_in` and `repo_id`.

This means after the first 403, all 10 task-checks in this cycle (and the next ~10 cycles) skip the API call entirely.

Important: the cached error is **not** propagated to *callers* as a real failure — the orchestrator already does `continue` on error, and that's the right behavior. We just want to stop the 9 redundant API calls behind it.

### Layer 4 — quiet `recoverIncompletePushes` for missing remote branches

In `git_repository_service.go:282 isBranchAheadOfRemote`, before calling `git fetch`, do a single `git ls-remote --heads origin` per repo (cached for the duration of the recovery pass) and skip branches that aren't in the result set. This eliminates the per-branch fetch attempt and the corresponding DEBUG line.

Alternative (simpler): keep the fetch attempt, but inspect the error string for `couldn't find remote ref` and downgrade to `Trace` (not logged at default levels).

Recommendation: **alternative**. `recoverIncompletePushes` runs once at startup, so the cost saved by `ls-remote` is small, and the simpler change is less risky.

## Key decisions

- **Cache lives on `GitRepositoryService`, not on the GitHub client.** That way the cache is shared across all callers (handlers, orchestrator, http server) and works the same for ADO/Bitbucket/GitLab. The GitHub-specific rate-limit detection still happens in `listGitHubPullRequests` but writes into the same shared cache.
- **TTL = 60s by default.** Long enough to absorb a 30s poll cycle plus a coincident user-driven request. Short enough that PR state stays fresh-feeling in the UI.
- **No conditional requests / ETag** in this PR. `go-github` supports it, but it adds complexity (storing per-call ETags, handling 304 responses). The TTL cache + rate-limit backoff is enough to drop us well under quota; ETags are a follow-up.
- **No webhooks.** Out of scope. Polling is fine if it's not pathological.
- **Error sentinel uses the same cache.** Avoids a separate "error backoff map" that has to stay in sync.

## Files to touch

- `api/pkg/services/spec_task_orchestrator.go` — refactor `detectExternalPRActivity` to prefetch `ListPullRequests` once per unique repo, pass into `checkTaskForExternalPRActivity` (rename helper / add new signature taking a `map[string][]*types.PullRequest`).
- `api/pkg/services/git_repository_service_pull_requests.go` — add `prListCache` struct, wrap `ListPullRequests`, add invalidation hooks in `CreatePullRequest` paths.
- `api/pkg/services/git_repository_service.go` — initialize the cache in `Init()` or constructor; downgrade or skip the "couldn't find remote ref" log in `isBranchAheadOfRemote`.
- `api/pkg/services/spec_task_orchestrator_test.go` — add a test that asserts `ListPullRequests` is called exactly once per unique repo when multiple tasks share repos.
- (Optional) `api/pkg/config/server.go` — add `GithubPRListCacheTTL` and `GithubRateLimitBackoff` if we expose env vars. Skip if defaults are good enough.

## Implementation Notes

- **`SpecTaskOrchestrator.gitService` is a concrete `*GitRepositoryService`, not an interface.** I considered introducing an interface to enable mocking the orchestrator-level dedup, but it cascades through `NewSpecTaskOrchestrator`'s callers and is broader than this fix. Instead, the cache layer carries the proof-of-fix in unit tests; the orchestrator-level memo is a one-line `prsByRepo[repo.ID]` lookup verifiable by code review.
- **GitHub `RateLimitError` propagates through `%w` wrapping correctly.** `Client.ListPullRequests` (skill) and `listGitHubPullRequests` (services) both already wrap with `fmt.Errorf("...: %w", err)`, so `errors.As` in `rateLimitBackoffUntil` unwraps to find the typed error from `go-github`. No change needed at the GitHub-client layer.
- **Test constructors don't initialize the cache.** `git_http_server_test.go` and `git_repository_service_pull_requests_test.go` build `&GitRepositoryService{...}` literals without `prListCache`. The wrapper code in `ListPullRequests`, `CreatePullRequest`, and `UpdatePullRequest` is nil-safe (`if s.prListCache != nil`) so existing tests keep working without modification.
- **Cache invalidation is at the dispatch level.** Putting it in `CreatePullRequest` / `UpdatePullRequest` (before the provider switch) covers all four providers (GitHub, ADO, GitLab, Bitbucket) with one line each, instead of repeating it in every provider-specific helper.
- **Skipped env-var plumbing.** Defaults are 60s cache / 5m fallback backoff. No operator has hit a case requiring different values; adding `HELIX_GITHUB_PR_LIST_CACHE_TTL` etc. would be premature config bloat. Trivial to add later.
- **Memoize failures too.** In the orchestrator's `prsByRepo`, even a failing `ListPullRequests` writes `nil` so the next task with the same repo skips the call rather than re-fetching. This matters when the cache layer returns the cached rate-limit error: that's *fast*, but writing nil into the map means the orchestrator doesn't even spend the lookup.

## Risk / gotchas

- **Test fixtures using `MockGitRepositoryService`** mock the public interface, so adding a cache inside the concrete impl is invisible to existing tests. Good.
- **Cache must be safe under concurrent poll-loop access.** Use `sync.Mutex` (single map, not high-volume). RWMutex isn't worth the complexity here.
- **Don't cache empty lists differently from non-empty lists.** A repo with zero open PRs is a valid result; cache it normally.
- The orchestrator's loop also takes a per-task lock via `o.store.UpdateSpecTask`. The prefetch happens *before* per-task work, so it doesn't extend any lock-held window.
