# Implementation Tasks

- [ ] In `git_repository_service_pull_requests.go`: add `userID string` param to `getGitHubClient`; prepend acting-user OAuth lookup (`s.store.GetOAuthConnectionByUserAndProvider(ctx, userID, "github")`) before the existing `repo.OAuthConnectionID` check
- [ ] In `git_repository_service_pull_requests.go`: add `userID string` param to `createGitHubPullRequest` and thread it through to `getGitHubClient`
- [ ] In `git_repository_service_pull_requests.go`: add `userID string` param to `getGitLabClient`; prepend acting-user OAuth lookup (`s.store.GetOAuthConnectionByUserAndProvider(ctx, userID, "gitlab")`) before existing fallback chain
- [ ] In `git_repository_service_pull_requests.go`: add `userID string` param to `createGitLabMergeRequest` and thread it through to `getGitLabClient`
- [ ] In `git_repository_service_pull_requests.go`: add `userID string` param to `CreatePullRequest` and thread it through to `createGitHubPullRequest` / `createGitLabMergeRequest`
- [ ] In `git_repository_handlers.go`: extract acting user via `getRequestUser(r)` in `createGitRepositoryPullRequest` and pass `user.ID` to `CreatePullRequest`
- [ ] In `spec_task_workflow_handlers.go`: add `userID string` param to `ensurePullRequestForTask`; pass it to `CreatePullRequest`; update call site in `approveImplementation` to pass `user.ID`
- [ ] Write new unit test: repo `OAuthConnectionID` = User A's connection; PR created on behalf of User B who has a GitHub OAuth connection → User B's token is used
- [ ] Verify all existing unit tests in `git_repository_service_pull_requests.go` pass (update call sites to pass empty `userID` where needed)
