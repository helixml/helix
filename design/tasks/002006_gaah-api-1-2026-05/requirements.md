# Requirements: Stop Hammering GitHub API in External PR Detection

## Problem

`api-1` is logging GitHub 403 rate limit errors continuously:

```
spec_task_orchestrator.go:1031 > Failed to list PRs, skipping repo
  error="...403 API rate limit exceeded for user ID 264658..."
  repo_id=code-helix-1773055528
```

The `detectExternalPRActivity` poll loop calls `gitService.ListPullRequests(ctx, repo.ID)` for **every external repo** of **every eligible task**, **every 30 seconds**. Each call hits the GitHub REST API with no caching, no conditional requests, and no shared result across tasks. With many tasks pointing at the same handful of repos (`helixml/helix`, `helixml/helix-next`), the same `GET /repos/{owner}/{repo}/pulls` request is repeated 10× per cycle, blowing through the authenticated 5000 req/hr quota.

A secondary symptom appears in the same log burst:

```
git_repository_service.go:294 > Failed to fetch remote before ahead check, skipping branch
  error="...couldn't find remote ref fix/e2e-stale-phase-completion..."
  branch=fix/e2e-stale-phase-completion
```

`recoverIncompletePushes` runs `git fetch origin <branch>` for every local branch on every external repo at startup. Branches that exist locally but not on the remote (deleted PRs, renamed branches) all log a noisy DEBUG line. This is benign but adds to the noise the user is reacting to.

## User Stories

- As an operator running Helix, I want the API to stop tripping the GitHub rate limit so that PR detection, CI status, and other GitHub-backed features keep working without 60-minute outages.
- As a developer reading `api-1` logs, I want PR-detection failures and "branch not on remote" cases to log at a level that matches their severity, so the log isn't a wall of noise.

## Acceptance Criteria

- [ ] `detectExternalPRActivity` calls `ListPullRequests` **at most once per repo per poll cycle**, regardless of how many tasks reference that repo. (Today: N tasks × M repos. Target: M repos.)
- [ ] Repeated `ListPullRequests` calls within a short window (default 60s) for the same repo return a cached result rather than re-hitting the upstream provider. Cache TTL is configurable.
- [ ] When the GitHub client returns a 403 rate-limit error, the orchestrator backs off: subsequent calls for that repo are short-circuited until the rate-limit reset window passes (parsed from `X-RateLimit-Reset` if available, otherwise a 5-minute fallback).
- [ ] Rate-limit errors log **once per backoff window** at WARN level (not once per task at DEBUG), with `repo_id`, `reset_in`, and the request URL. Routine "no PRs found" cases stay quiet.
- [ ] `recoverIncompletePushes` no longer fetches remote refs for branches that don't exist on the remote. Either: (a) list remote branches once via `git ls-remote --heads origin` and intersect with local branches before fetching, or (b) silently swallow the "couldn't find remote ref" error class without the per-branch DEBUG log.
- [ ] No behavior regression: tasks with externally-opened PRs still get detected and moved to `pull_request` status; tasks whose branch was merged still get moved to `done`. Add or extend a unit test in `spec_task_orchestrator_test.go` proving the per-repo dedup.
- [ ] No new env vars required for default behavior. Optional: `HELIX_GITHUB_PR_LIST_CACHE_TTL` and `HELIX_GITHUB_RATE_LIMIT_BACKOFF` for operators who want to tune.

## Out of Scope

- Switching to GitHub webhooks for PR events (much bigger change; tracked separately).
- Conditional `If-None-Match` / ETag handling on the GitHub client (nice-to-have follow-up).
- Rate-limit handling for Azure DevOps / Bitbucket / GitLab (same caching applies, but those providers haven't been observed tripping limits).
