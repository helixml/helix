# Implementation Tasks

- [~] Add a `prListCache` struct (TTL-keyed `map[repoID] → {prs, err, fetchedAt}`) on `GitRepositoryService`; initialize in the service constructor / `Init()` with a 60s default TTL.
- [ ] Wrap `GitRepositoryService.ListPullRequests` in `git_repository_service_pull_requests.go:215` to read/write the cache. Cache successful results for the full TTL; cache GitHub `*RateLimitError` / `*AbuseRateLimitError` until the rate-limit reset time.
- [ ] In `listGitHubPullRequests` (`git_repository_service_pull_requests.go:604`), detect `*github.RateLimitError` from `go-github` and return a typed error the cache layer can recognize. Log the rate-limit hit at WARN once per cache-insert with `repo_id` + `reset_in`.
- [ ] Invalidate the cache entry for a repo whenever a PR-mutating operation runs against it (`createGitHubPullRequest`, ADO/Bitbucket equivalents).
- [ ] Refactor `detectExternalPRActivity` (`spec_task_orchestrator.go:946`): build the unique set of external repos across the eligible task batch, call `ListPullRequests` once per unique repo, pass the resulting `map[repoID][]*types.PullRequest` into `checkTaskForExternalPRActivity`.
- [ ] Update `checkTaskForExternalPRActivity` signature to accept the prefetched PR map and skip the inner `ListPullRequests` call. Keep the fallback `IsBranchMerged` check unchanged.
- [ ] In `git_repository_service.go:282 isBranchAheadOfRemote`, recognize the `couldn't find remote ref` error class and skip without logging at DEBUG (downgrade to `Trace` or omit the log).
- [ ] Extend `spec_task_orchestrator_test.go` with a test: 5 tasks across 2 unique repos → assert mock `ListPullRequests` called exactly 2 times in one cycle.
- [ ] Add a unit test for the cache: two back-to-back calls within TTL → one upstream call; call after TTL expiry → two upstream calls; rate-limit error → cached until reset.
- [ ] (Optional) Wire `HELIX_GITHUB_PR_LIST_CACHE_TTL` and `HELIX_GITHUB_RATE_LIMIT_BACKOFF` env vars through `config/server.go` if defaults need to be tunable.
- [ ] Manual verification in dev: tail `api-1` logs across two poll cycles, confirm the per-repo PR list call count matches the unique repo count, and that the "couldn't find remote ref" lines are gone.
- [ ] Commit and push as a single PR following conventional commits (`fix(api): cache PR listings and back off on GitHub rate limits`). Run `cd api && go build ./...` before pushing.
