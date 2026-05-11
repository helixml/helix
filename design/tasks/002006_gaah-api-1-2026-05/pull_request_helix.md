# fix(api): cache PR listings and back off on GitHub rate limits

## Summary

`api-1` was logging GitHub `403 API rate limit exceeded` continuously — `detectExternalPRActivity` runs every 30s, and for each of up to 10 eligible tasks it called `gitService.ListPullRequests` once per external repo, with no caching. With multiple task projects sharing the same upstream repos (`helixml/helix`, `helixml/helix-next`), the same `GET /repos/{owner}/{repo}/pulls` request fired ~20× per minute, blowing past GitHub's 5000 req/hr authenticated quota.

This adds a 60s in-process TTL cache on `GitRepositoryService.ListPullRequests` (shared across orchestrator, handlers, and the git HTTP server), explicit per-cycle dedup in the orchestrator, and rate-limit-aware backoff that caches the error until the upstream's reset window.

Also quiets the noisy "couldn't find remote ref" `Debug` line emitted by `recoverIncompletePushes` for branches that exist locally but not on the remote (deleted PR branches) — downgraded to `Trace` for that specific error class while keeping `Debug` for genuine remote/auth failures.

## Changes

- **New `prListCache`** (`git_repository_service_pr_list_cache.go`) — TTL-keyed `map[repoID] → {prs, err, expiresAt}`. Successful results cached for 60s; rate-limit errors cached until their reset time.
- **`ListPullRequests` wrapped** with cache lookup, miss-fetch, and cache-write. Detects `*github.RateLimitError` / `*github.AbuseRateLimitError` (via `errors.As`, so works through the wrapping layers) and logs once at WARN per backoff window with `repo_id` + `reset_in`.
- **Cache invalidation** added to `CreatePullRequest` and `UpdatePullRequest` so the next read sees fresh state.
- **Per-cycle dedup** in `detectExternalPRActivity` via a `map[repoID][]*types.PullRequest` memo — N tasks sharing M repos now hit the upstream M times per cycle, not N×M.
- **Quieter recovery log**: `isBranchAheadOfRemote` downgrades `couldn't find remote ref` to `Trace`.
- **Tests** for the cache (TTL hit/expire, error-until-expiresAt, invalidate) and `rateLimitBackoffUntil` (RateLimitError, wrapped via `%w`, AbuseRateLimitError, past-reset fallback, non-rate-limit pass-through).

## Why two layers (cache + per-cycle memo)?

The cache alone already satisfies "≤ 1 upstream call per repo per 60s". The orchestrator-level memo is defense-in-depth: it makes the per-cycle intent explicit at the call site, survives accidental cache-bypass refactors, and saves one map lookup per call.

## Test plan

- [x] `go build ./...` clean
- [x] `go test -run 'TestPRListCache|TestRateLimitBackoff' ./pkg/services/` — 8/8 passing
- [x] `go test -run 'TestSpecTaskOrchestratorTestSuite' ./pkg/services/` — no regression
- [x] `go test -run 'TestEnsurePullRequest' ./pkg/server/` — no regression
- [ ] Operator verification post-deploy: tail `api-1` logs across two poll cycles, confirm `ListPullRequests` calls drop to one per unique external repo per ~60s, and `couldn't find remote ref` lines are gone from default-level logs.
