# Design: Prevent Duplicate PR on Rename

## Context

The close-PR scenario (closed PR → `ListPullRequests` returns open-only → dedup fails → new PR) is addressed in a separate PR. This document covers the **rename** scenario, which is in the same ballpark: both stem from `ensurePullRequestForRepo` relying solely on `ListPullRequests` rather than also checking the already-tracked `task.RepoPullRequests`.

## Key Files

| File | Role |
|------|------|
| `api/pkg/server/spec_task_workflow_handlers.go` | `ensurePullRequestForRepo` + `ensurePullRequestsForAllRepos` — orchestrator-triggered path |
| `api/pkg/services/git_http_server.go` | `ensurePullRequest` — push-triggered path |
| `api/pkg/services/spec_task_orchestrator.go` | `handlePullRequest` — polling loop that calls `ensurePRs` every 30 seconds |
| `api/pkg/agent/skill/github/client.go` | `ListPullRequests` — fetches open-state PRs only |

## Root Cause

There are two parallel PR-ensuring paths that do NOT share state correctly:

### Path A — Orchestrator polling (`ensurePullRequestsForAllRepos` → `ensurePullRequestForRepo`)

Called every 30 seconds from `handlePullRequest`. For each project repo:
1. Checks if local branch exists → if not, returns `nil, nil` (no error, no PR)
2. Pushes branch to remote
3. Calls `ListPullRequests` (open PRs only)
4. If found by branch name → returns the existing `RepoPR`
5. If **not found** OR if push/list step errors → returns `nil, error`

Back in `ensurePullRequestsForAllRepos` (line 590–601), errors cause `continue` (skip the repo). Then:
```go
task.RepoPullRequests = repoPRs   // DESTRUCTIVE REPLACEMENT
```

**If any repo's `ensurePullRequestForRepo` returns an error (e.g., push failed transiently, API hiccup during a rename, or 422 "already exists" on a race), that repo's PR is silently dropped from `repoPRs`. The replacement wipes it from `task.RepoPullRequests`.**

On the next poll, `task.RepoPullRequests` is empty for that repo → `ensurePullRequestForRepo` finds no tracked PR, calls `ListPullRequests` → if it now finds the renamed-but-open PR, it returns it correctly and no duplicate is created. But if it doesn't find it (race window, eventual consistency), it calls `CreatePullRequest` → **duplicate**.

### Path B — Push-triggered (`ensurePullRequest` in git_http_server.go)

Called when the agent pushes to the feature branch. This path:
1. Calls `ListPullRequests` (open PRs only)
2. If found by branch → updates the PR title back to Helix's version via `UpdatePullRequest` (unwanted side effect: overwrites user's rename)
3. Calls `updateRepoPullRequests` (append/update, not replace)
4. Has 422-error recovery: if `CreatePullRequest` returns "already exists", re-lists and finds it

This path is safer than Path A because it has 422 recovery and uses append rather than replace. But it also has the undesirable behaviour of silently renaming the PR back, conflicting with the user's explicit rename.

### Why Rename Specifically Triggers This

A rename doesn't change the PR's branch or state, so the dedup check should logically work. The "same ballpark" as the close scenario is that both expose the same structural weakness: **`ensurePullRequestsForAllRepos` does a destructive array replacement without handling transient errors**. A rename may correlate with a push from the agent (to update PR description file), which races with the orchestrator poll. If both calls to `ensurePullRequestForRepo` run at almost the same time, one can get a 422 "already exists" on push or a transient API error, causing `repoPRs` to be empty, wiping the tracked PR, and creating conditions for a duplicate on the next failure.

## Fix

### 1. Check `task.RepoPullRequests` first in `ensurePullRequestForRepo` (Path A)

Before calling `ListPullRequests` and before attempting to create a PR, check if a PR is already tracked for this repo:

```go
// In ensurePullRequestForRepo, after branch-exists check:
if existing := task.GetPRForRepo(repo.ID); existing != nil {
    // PR already tracked — fetch current state by ID
    pr, err := s.gitRepositoryService.GetPullRequest(ctx, repo.ID, existing.PRID)
    if err == nil {
        // Return the tracked PR regardless of state (open, closed, etc.)
        return &types.RepoPR{
            RepositoryID:   repo.ID,
            RepositoryName: repo.Name,
            PRID:           pr.ID,
            PRNumber:       pr.Number,
            PRURL:          pr.URL,
            PRState:        string(pr.State),
        }, nil
    }
    // If fetch fails (e.g., PR deleted), fall through to create
}
```

This makes `task.RepoPullRequests` the authoritative record. If Helix already opened a PR for this repo, it won't open another one regardless of what `ListPullRequests` returns.

### 2. Handle 422 "already exists" in `ensurePullRequestForRepo` (Path A)

The orchestrator path (unlike Path B) does not handle 422 errors — it just returns an error that causes the repo to be skipped. Add recovery:

```go
prID, err := s.gitRepositoryService.CreatePullRequest(...)
if err != nil {
    if strings.Contains(err.Error(), "already exists") {
        // Find the existing PR and track it
        freshPRs, _ := s.gitRepositoryService.ListPullRequests(ctx, repo.ID)
        for _, pr := range freshPRs {
            if pr.SourceBranch == sourceBranchRef || pr.SourceBranch == branch {
                return &types.RepoPR{...}, nil
            }
        }
    }
    return nil, fmt.Errorf("failed to create PR: %w", err)
}
```

### 3. Stop `ensurePullRequest` (Path B) from overwriting user-renamed PR titles

The push path calls `UpdatePullRequest` when it finds an existing open PR, restoring Helix's title over the user's rename. This is unwanted. The update should be conditional: only update if the PR description file has changed (e.g., compare a hash or only update if the file was pushed in this same push event).

Alternatively (simpler): skip `UpdatePullRequest` entirely in this path. The PR title only needs to be set at creation time.

## Notes for Implementers

- `task.GetPRForRepo(repoID)` is defined in `api/pkg/types/simple_spec_task.go:48`.
- Fix 1 is the primary guard and addresses both the rename and any similar "PR invisible" scenarios.
- Fix 2 is a robustness improvement that prevents the destructive replacement from wiping tracked PRs.
- Fix 3 is optional but improves UX by respecting user-chosen PR titles.
- The close scenario is handled separately — do not conflate fixes.
