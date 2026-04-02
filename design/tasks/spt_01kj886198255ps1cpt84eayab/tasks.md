# Implementation Tasks

- [ ] In `git_repository_handlers.go`: in `createGitRepositoryPullRequest`, extract the acting user via `getRequestUser(r)` and check for their OAuth connection via `s.store.GetOAuthConnectionByUserAndProvider(ctx, user.ID, providerID)`; if not found, call `s.oauthManager.StartOAuthFlow(...)` and return the authorization URL to the client (no PR creation)
- [ ] In `git_repository_service_pull_requests.go`: add `userID string` param to `CreatePullRequest`, `createGitHubPullRequest`, and `getGitHubClient`; in `getGitHubClient`, use `s.store.GetOAuthConnectionByUserAndProvider(ctx, userID, "github")` as the first (and only) OAuth lookup when `userID` is non-empty
- [ ] In `git_repository_service_pull_requests.go`: apply the same `userID` threading to `createGitLabMergeRequest` and `getGitLabClient`
- [ ] In `spec_task_workflow_handlers.go`: add `userID string` param to `ensurePullRequestForTask`; check for acting user's OAuth connection upfront; surface an error requiring account connection if missing; pass `userID` through to `CreatePullRequest`; update `approveImplementation` to pass `user.ID`
- [ ] Write new unit test: repo `OAuthConnectionID` = User A's connection; PR created on behalf of User B who has a GitHub OAuth connection → User B's token is used
- [ ] Verify all existing unit tests in `git_repository_service_pull_requests.go` pass (update call sites to pass empty `userID` where needed)
