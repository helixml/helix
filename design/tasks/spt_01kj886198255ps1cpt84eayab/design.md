# Design: PR/MR Authored by Acting User

## Root Cause

`getGitHubClient(ctx, repo)` in `git_repository_service_pull_requests.go` resolves credentials using only `repo.OAuthConnectionID`, which is set once at repo-creation time and never updated. The acting user's identity is never passed into this function.

## Solution

Thread an optional `userID string` parameter down the PR creation call chain. When a `userID` is provided, attempt to look up that user's GitHub/GitLab OAuth connection first; fall back to existing repo-level credential resolution if not found.

### Fallback Chain (new)

```
1. Acting user's OAuth token  (new — looked up by userID + provider)
2. Repo OAuth connection       (existing repo.OAuthConnectionID)
3. GitHub App                  (existing)
4. PAT                         (existing repo.GitHub.PersonalAccessToken or repo.Password)
```

### Call Chain Changes

```
createGitRepositoryPullRequest (handler)         ← pass user.ID
  → gitRepositoryService.CreatePullRequest(ctx, repoID, ..., userID)
    → createGitHubPullRequest(ctx, repo, ..., userID)
      → getGitHubClient(ctx, repo, userID)       ← try user's connection first
```

Same change applied to `createGitLabMergeRequest` / `getGitLabClient`.

For the task workflow path:
```
approveImplementation (handler)                  ← already has user
  → ensurePullRequestForTask(ctx, repo, task, userID)
    → gitRepositoryService.CreatePullRequest(ctx, ..., userID)
```

## Key APIs

- **Look up acting user's token:** `s.store.GetOAuthConnectionByUserAndProvider(ctx, userID, providerID)`
  - `providerID` is `"github"` or `"gitlab"` (match the string used in `oauth/manager.go`)
  - This method already exists on the store (used by `oauth/manager.go:284`)
- **No changes to how repos are stored.** `OAuthConnectionID` remains the repo-level default.

## Files to Change

| File | Change |
|------|--------|
| `api/pkg/services/git_repository_service_pull_requests.go` | Add `userID string` to `getGitHubClient`, `getGitLabClient`, `createGitHubPullRequest`, `createGitLabMergeRequest`, `CreatePullRequest`. Prepend user-OAuth lookup before existing fallback chain. |
| `api/pkg/server/git_repository_handlers.go` | Extract acting user (`getRequestUser(r)`) and pass `user.ID` to `CreatePullRequest`. |
| `api/pkg/server/spec_task_workflow_handlers.go` | Add `userID string` param to `ensurePullRequestForTask`; pass it through from `approveImplementation`. |

## Notes for Implementer

- The store method is `GetOAuthConnectionByUserAndProvider(ctx, userID, providerID string)` — returns `(*types.OAuthConnection, error)`. If `err != nil` or `conn.AccessToken == ""`, skip and continue to next fallback.
- `userID` may be empty (e.g., background jobs) — treat empty string as "no acting user, use repo credentials".
- No need to call `oauth/manager.go`'s `GetConnection` (which handles refresh); the store fetch is sufficient here since we only need a token at request time and existing background refresh keeps tokens current.
- Provider ID strings: check `oauth/manager.go` or provider registration to confirm the exact strings (likely `"github"` and `"gitlab"`).
