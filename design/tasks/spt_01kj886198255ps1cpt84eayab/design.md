# Design: PR/MR Authored by Acting User

## Root Cause

`getGitHubClient(ctx, repo)` in `git_repository_service_pull_requests.go` resolves credentials using only `repo.OAuthConnectionID`, which is set once at repo-creation time and never updated. The acting user's identity is never passed into this function.

## Solution

In the HTTP handler, check whether the acting user already has an OAuth connection for the repo's provider. If they do, use their token for PR creation. If they do not, initiate the OAuth flow for them rather than silently using repo-level credentials.

### Handler Logic (user-initiated path)

```
createGitRepositoryPullRequest (handler)
  1. Look up acting user's OAuth connection for this provider
     → s.store.GetOAuthConnectionByUserAndProvider(ctx, user.ID, providerID)
  2a. Connection found → pass user.ID into CreatePullRequest (token used for PR authorship)
  2b. No connection   → call s.oauthManager.StartOAuthFlow(ctx, user.ID, providerID, redirectURL, ...)
                        → return OAuth authorization URL to client (HTTP 401 or dedicated response)
                        → client redirects user to GitHub/GitLab OAuth; after completion, user retries
```

For the service layer, thread `userID` down so `getGitHubClient` / `getGitLabClient` use the acting user's token (step 2a only reaches `CreatePullRequest` when the connection exists):

```
gitRepositoryService.CreatePullRequest(ctx, repoID, ..., userID)
  → createGitHubPullRequest(ctx, repo, ..., userID)
    → getGitHubClient(ctx, repo, userID)   ← look up user's connection, no fallback to repo creds
```

### Task workflow path (`ensurePullRequestForTask`)

Same check: if the acting user (the approver) has no OAuth connection, surface an error requiring them to connect their account before approval can proceed. Thread `userID` from `approveImplementation` through to `CreatePullRequest`.

## Key APIs

- **Check/fetch acting user's token:** `s.store.GetOAuthConnectionByUserAndProvider(ctx, userID, providerID)` (exists — used by `oauth/manager.go:284`)
- **Initiate OAuth flow:** `s.oauthManager.StartOAuthFlow(ctx, userID, providerID, redirectURL, metadata, scopes)` → returns authorization URL (`oauth/manager.go:363`)
- **No changes to how repos are stored.** `OAuthConnectionID` remains the repo-level default.

## Files to Change

| File | Change |
|------|--------|
| `api/pkg/services/git_repository_service_pull_requests.go` | Add `userID string` to `getGitHubClient`, `getGitLabClient`, `createGitHubPullRequest`, `createGitLabMergeRequest`, `CreatePullRequest`. Prepend user-OAuth lookup before existing fallback chain. |
| `api/pkg/server/git_repository_handlers.go` | Extract acting user (`getRequestUser(r)`) and pass `user.ID` to `CreatePullRequest`. |
| `api/pkg/server/spec_task_workflow_handlers.go` | Add `userID string` param to `ensurePullRequestForTask`; pass it through from `approveImplementation`. |

## Notes for Implementer

- `GetOAuthConnectionByUserAndProvider(ctx, userID, providerID string)` returns `(*types.OAuthConnection, error)`. Treat any error or empty `AccessToken` as "no connection — trigger OAuth flow".
- `StartOAuthFlow` returns an authorization URL string. The handler should return this to the client with a response shape the frontend already understands (check existing OAuth initiation endpoints for the pattern).
- Provider ID strings: confirm exact values from `oauth/manager.go` provider registration (likely `"github"` and `"gitlab"`).
- `userID` is always available in these handlers via `getRequestUser(r)`. There is no background-job path for user-initiated PR creation.
