# Design: PR/MR Authored by Acting User

## Root Cause

`getGitHubClient(ctx, repo)` in `git_repository_service_pull_requests.go` resolves credentials using only `repo.OAuthConnectionID`, which is set once at repo-creation time and never updated. The acting user's identity is never passed into this function.

## Solution

Thread `userID` (the acting user) from the HTTP handler all the way into `getGitHubClient` / `getGitLabClient`. When `userID` is provided, look up that user's OAuth connections first; if none found, return an error (the handler surfaces this as an OAuth prompt). If `userID` is empty (background/automated calls), fall through to repo-level credentials unchanged.

### Credential resolution chain in `getGitHubClient`

```
1. GitHub App (service-to-service, highest priority — unchanged)
2. Acting user's OAuth connection  ← NEW
   → s.store.ListOAuthConnections(ctx, &ListOAuthConnectionsQuery{UserID: userID})
   → find conn where conn.Provider.Type == OAuthProviderTypeGitHub && conn.AccessToken != ""
   → if userID non-empty and no connection found: return error (handler prompts OAuth)
3. Repo-level OAuth connection (repo.OAuthConnectionID)  ← fallback when userID empty
4. Repo-level PAT
5. Username/password
```

Same chain applies to `getGitLabClient`.

### Handler logic (user-initiated path)

`createGitRepositoryPullRequest` calls the existing `ensureUserOAuthConnection` helper, which:
1. Lists the user's OAuth connections
2. If none match the provider type, starts an OAuth flow and returns a 401 with `auth_url`
3. If found, calls `s.gitRepositoryService.CreatePullRequest(..., user.ID)` so the service uses their token

### Task workflow path (`ensurePullRequestForTask`)

`approveImplementation` checks the user's OAuth connection upfront; returns 400 if missing. It then passes `user.ID` as `approverUserID` to `ensurePullRequestForTask`, which passes it to `CreatePullRequest`.

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

- Use `s.store.ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{UserID: userID})` to find connections; filter by `conn.Provider.Type == types.OAuthProviderTypeGitHub` (or `TypeGitLab`).
- `userID` is always available in these handlers via `getRequestUser(r)`. There is no background-job path for user-initiated PR creation.
- **GitHub App takes priority over user OAuth** — this is intentional. GitHub App is service-level auth for automated systems and is never overridden by individual user tokens.
- The handler's `ensureUserOAuthConnection` helper already handles the 401+auth_url response pattern — reuse it rather than reimplementing.
- `OAuthConnectionID` on the repo struct is unchanged. It remains the fallback when `userID` is empty (background calls, older code paths).
