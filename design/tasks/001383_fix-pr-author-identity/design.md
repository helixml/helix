# Design: Fix PR Author Identity

## Architecture

### Current Call Chain

```
createGitRepositoryPullRequest (git_repository_handlers.go:1531)
  user := getRequestUser(r)          ← user is available here
  gitRepositoryService.CreatePullRequest(ctx, repoID, ...)   ← user.ID NOT passed
    → createGitHubPullRequest
      → getGitHubClient(ctx, repo)   ← uses repo.OAuthConnectionID (initializer)

approveImplementation (spec_task_workflow_handlers.go:25)
  user := getRequestUser(r)          ← user is available here
  ensurePullRequestForTask(ctx, repo, task)   ← user NOT passed
    → gitRepositoryService.CreatePullRequest(ctx, repo.ID, ...)   ← user.ID NOT passed
```

### Fix: Thread `userID` Through the Call Chain

**Step 1 — Service layer** (`git_repository_service_pull_requests.go`):

Add `userID string` parameter to `CreatePullRequest`, `getGitHubClient`, `createGitHubPullRequest`, `getGitLabClient`, `createGitLabMergeRequest`.

New resolution order in `getGitHubClient` and `getGitLabClient`:
1. Acting user's OAuth connection (look up by `userID` + provider type via `store.GetOAuthConnectionByUserAndProvider`)
2. GitHub App credentials on repo struct (unchanged — service-to-service, no user identity)
3. `repo.OAuthConnectionID` — repo-level default
4. PAT (`repo.GitHub.PersonalAccessToken` / `repo.GitLab.PersonalAccessToken`)
5. `repo.Password`

For GitHub App (step 2), keep it **before** user OAuth. GitHub Apps are a privileged service identity; using the app is correct for automated actions even if the user has a connection. Wait — per the task spec the preferred fallback chain is: acting user OAuth → repo OAuth → GitHub App → PAT → username/password. The user's personal token should come first to get correct attribution. **Apply the chain as specified in the task.**

Corrected order for `getGitHubClient(ctx, repo, userID)`:
1. Acting user's GitHub OAuth connection (if `userID != ""`)
2. GitHub App (repo struct)
3. `repo.OAuthConnectionID`
4. PAT
5. `repo.Password`

For `getGitLabClient(ctx, repo, userID)`:
1. Acting user's GitLab OAuth connection (if `userID != ""`)
2. `repo.OAuthConnectionID`
3. PAT
4. `repo.Password`

**Finding the user's OAuth connection:** Use `store.GetOAuthConnectionByUserAndProvider(ctx, userID, providerID)`. The `providerID` must be resolved from the provider type (GitHub or GitLab). Use `store.ListOAuthConnections(ctx, &ListOAuthConnectionsQuery{UserID: userID})` and filter by provider type, OR look up the provider by type via `oauth.Manager.GetProviderByName` then call `GetOAuthConnectionByUserAndProvider`. The simpler option: call `store.ListOAuthConnections` with just `UserID` and find the first connection whose provider type matches — consistent with how `sample_project_access_handlers.go` does it.

**Step 2 — Handler** (`git_repository_handlers.go:1587`):

```go
prID, prErr = s.gitRepositoryService.CreatePullRequest(
    r.Context(), repoID, request.Title, request.Description,
    request.SourceBranch, request.TargetBranch, user.ID)  // add user.ID
```

**Step 3 — Spec task workflow** (`spec_task_workflow_handlers.go`):

`ensurePullRequestForTask` gets a `userID string` parameter. `approveImplementation` passes `user.ID` when calling `ensurePullRequestForTask`.

**Step 4 — `git_http_server.go`** (line 1068):

This path is triggered by the agent (not a direct HTTP request). Pass `""` for `userID` so it falls back to repo-level credentials — correct behaviour for automated agent-triggered PRs.

### Key Files

| File | Change |
|------|--------|
| `api/pkg/services/git_repository_service_pull_requests.go` | Add `userID` to `CreatePullRequest`, `getGitHubClient`, `getGitLabClient` and their callees; implement user-first resolution |
| `api/pkg/server/git_repository_handlers.go` | Pass `user.ID` to `CreatePullRequest` |
| `api/pkg/server/spec_task_workflow_handlers.go` | Thread `userID` through `approveImplementation` → `ensurePullRequestForTask` → `CreatePullRequest` |
| `api/pkg/services/git_http_server.go` | Pass `""` as `userID` (agent path, no acting user) |

### Existing OAuth Lookup Infrastructure

- `store.GetOAuthConnectionByUserAndProvider(ctx, userID, providerID)` — direct lookup (needs providerID UUID)
- `store.ListOAuthConnections(ctx, &ListOAuthConnectionsQuery{UserID: userID})` — list all user connections, filter by type
- `types.OAuthProvider.Type` — `OAuthProviderTypeGitHub = "github"`, `OAuthProviderTypeGitLab = "gitlab"`
- `types.OAuthConnection.ProviderType` — set on the connection struct; use this to filter without needing a second DB call

### Codebase Patterns Found

- Provider type constants are in `api/pkg/types/oauth.go` (`OAuthProviderTypeGitHub`, `OAuthProviderTypeGitLab`)
- `OAuthConnection` has `ProviderType` field — check this when filtering by provider without an extra join
- `sample_project_access_handlers.go` (lines 82-86) uses `ListOAuthConnections{UserID: user.ID}` then filters manually — same pattern to use here
- `store.MockStore` has generated mocks for `GetOAuthConnectionByUserAndProvider` and `ListOAuthConnections` — tests can use these directly

### No Schema Changes

`repo.OAuthConnectionID` stays as-is. Only runtime resolution logic changes.
