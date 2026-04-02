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

Resolution has two distinct modes depending on whether an interactive user is present:

**User-initiated flow** (`userID != ""`):
1. Look up acting user's OAuth connection by `userID` + provider type.
2. If found, use it. Done.
3. If NOT found, return a structured `OAuthRequiredError` (HTTP 422, error code `oauth_required`, includes `provider_type`). Do NOT fall back to repo-level credentials -- the PR would be mis-attributed.

**Agent/automated flow** (`userID == ""`):
1. GitHub App credentials (GitHub only)
2. `repo.OAuthConnectionID` -- repo-level default
3. PAT (`repo.GitHub.PersonalAccessToken` / `repo.GitLab.PersonalAccessToken`)
4. `repo.Password`

This means `getGitHubClient(ctx, repo, userID)` and `getGitLabClient(ctx, repo, userID)` branch on whether `userID` is empty. The agent path retains the full fallback chain for backwards compatibility. The user path either resolves to the user's own token or fails with `oauth_required`.

**Finding the user's OAuth connection:** Use `store.GetOAuthConnectionByUserAndProvider(ctx, userID, providerID)`. The `providerID` must be resolved from the provider type (GitHub or GitLab). Use `store.ListOAuthConnections(ctx, &ListOAuthConnectionsQuery{UserID: userID})` and filter by provider type, OR look up the provider by type via `oauth.Manager.GetProviderByName` then call `GetOAuthConnectionByUserAndProvider`. The simpler option: call `store.ListOAuthConnections` with just `UserID` and find the first connection whose provider type matches — consistent with how `sample_project_access_handlers.go` does it.

**Step 2 — Error type** (`types/` or inline in service):

Define an `OAuthRequiredError` that carries the provider type (e.g. `"github"`, `"gitlab"`). The handler layer checks for this error type with `errors.As` and returns a JSON response:

```json
{
  "error": "oauth_required",
  "message": "GitHub OAuth connection required to open a PR under your account",
  "provider_type": "github"
}
```

HTTP status: 422 Unprocessable Entity.

**Step 3 — Handler** (`git_repository_handlers.go:1587`):

```go
prID, prErr = s.gitRepositoryService.CreatePullRequest(
    r.Context(), repoID, request.Title, request.Description,
    request.SourceBranch, request.TargetBranch, user.ID)  // add user.ID
```

If `prErr` is an `OAuthRequiredError`, return 422 with the structured JSON above instead of a generic 500.

**Step 4 — Spec task workflow** (`spec_task_workflow_handlers.go`):

`ensurePullRequestForTask` gets a `userID string` parameter. `approveImplementation` passes `user.ID` when calling `ensurePullRequestForTask`. If the PR creation returns `OAuthRequiredError`, propagate it back to the HTTP handler so the frontend receives the structured 422 response. Do NOT move the task to `pull_request` status if OAuth is missing -- the task stays in `implementation` so the user can retry after connecting.

**Step 5 — `git_http_server.go`** (line 1068):

This path is triggered by the agent (not a direct HTTP request). Pass `""` for `userID` so it uses the agent/automated flow with full repo-level credential fallback.

**Step 6 — Frontend** (`specTaskWorkflowService.ts` + `SpecTaskActionButtons.tsx`):

In the mutation's `onError` handler, check for the `oauth_required` error code in the API response. When detected:
1. Show a message: "Connect your GitHub account to open PRs under your name."
2. Redirect the user to the OAuth connection flow (e.g. `/account/connections` or the provider-specific OAuth initiation URL).
3. After OAuth setup completes, the user can retry "Open PR" -- no page reload needed if using React Query invalidation.

### Key Files

| File | Change |
|------|--------|
| `api/pkg/services/git_repository_service_pull_requests.go` | Add `userID` to `CreatePullRequest`, `getGitHubClient`, `getGitLabClient` and their callees; user-initiated flow returns `OAuthRequiredError` if no connection, agent flow retains full fallback chain |
| `api/pkg/types/` (or inline) | Define `OAuthRequiredError` struct with `ProviderType` field |
| `api/pkg/server/git_repository_handlers.go` | Pass `user.ID` to `CreatePullRequest`; handle `OAuthRequiredError` -> 422 JSON response |
| `api/pkg/server/spec_task_workflow_handlers.go` | Thread `userID` through `approveImplementation` -> `ensurePullRequestForTask` -> `CreatePullRequest`; propagate `OAuthRequiredError` to handler; do NOT advance task status on OAuth failure |
| `api/pkg/services/git_http_server.go` | Pass `""` as `userID` (agent path, no acting user) |
| `frontend/src/services/specTaskWorkflowService.ts` | Handle `oauth_required` error code in `onError`; redirect to OAuth connection flow |
| `frontend/src/components/tasks/SpecTaskActionButtons.tsx` | Show OAuth connection prompt when mutation fails with `oauth_required` |

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
