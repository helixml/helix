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

### Task workflow path (spec task approval)

`approveImplementation` checks the user's OAuth connection upfront (returns 400 if missing). It then passes `user.ID` as `approverUserID` to `ensurePullRequestsForAllRepos`, which threads it through `ensurePullRequestForRepo` → `CreatePullRequest`.

```
approveImplementation
  → ensurePullRequestsForAllRepos(ctx, task, primaryRepoID, approverUserID)
    → ensurePullRequestForRepo(ctx, repo, task, primaryRepoPath, userID)
      → gitRepositoryService.CreatePullRequest(ctx, repo.ID, ..., userID)
        → getGitHubClient(ctx, repo, userID)   ← user's token used
```

Background polling (orchestrator's `handlePullRequest`) passes `""` for `userID`, so it falls back to repo-level credentials.

### `EnsurePRsFunc` type update

The `EnsurePRsFunc` callback type in `spec_task_orchestrator.go` also gets the `userID` parameter. The orchestrator's own call passes `""` (no user context in background polling).

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/services/git_repository_service_pull_requests.go` | Add `userID string` to `getGitHubClient`, `getGitLabClient`, `createGitHubPullRequest`, `createGitLabMergeRequest`, `CreatePullRequest`. User-OAuth lookup added as step 2 in resolution chain. |
| `api/pkg/server/git_repository_handlers.go` | Pass `user.ID` to `CreatePullRequest`; enforce OAuth via `ensureUserOAuthConnection`. |
| `api/pkg/server/spec_task_workflow_handlers.go` | Add `userID string` to `ensurePullRequestForRepo` and `ensurePullRequestsForAllRepos`; thread `user.ID` from `approveImplementation`. OAuth check added upfront in `approveImplementation`. |
| `api/pkg/services/git_http_server.go` | Pass `""` to `CreatePullRequest` (background path, use repo-level creds). |
| `api/pkg/services/spec_task_orchestrator.go` | Update `EnsurePRsFunc` type to include `userID string`; pass `""` from background polling. |

## Notes for Implementer

- Use `s.store.ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{UserID: userID})` to find connections; filter by `conn.Provider.Type == types.OAuthProviderTypeGitHub` (or `TypeGitLab`).
- **GitHub App takes priority over user OAuth** — intentional; service-level auth for automated systems.
- The handler's `ensureUserOAuthConnection` helper handles the 401+auth_url response pattern.
- `OAuthConnectionID` on the repo struct is unchanged. It remains the fallback when `userID` is empty.
- `ensurePullRequestForTask` (deprecated wrapper) passes `""` for userID — existing callers unaffected.
