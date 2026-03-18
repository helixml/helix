# Implementation Tasks

- [ ] Add `userID string` parameter to `GitRepositoryService.CreatePullRequest` in `git_repository_service_pull_requests.go` and thread it to provider-specific helpers
- [ ] Update `getGitHubClient(ctx, repo, userID)` to resolve the acting user's GitHub OAuth connection first (via `store.ListOAuthConnections` filtered by `OAuthProviderTypeGitHub`), before falling back to GitHub App → repo `OAuthConnectionID` → PAT → password
- [ ] Update `getGitLabClient(ctx, repo, userID)` with same acting-user-first resolution for GitLab
- [ ] Update `createGitRepositoryPullRequest` handler in `git_repository_handlers.go` to pass `user.ID` to `CreatePullRequest`
- [ ] Update `ensurePullRequestForTask` in `spec_task_workflow_handlers.go` to accept `userID string` and pass it to `CreatePullRequest`; update `approveImplementation` to pass `user.ID`
- [ ] Update `git_http_server.go` (agent-triggered PR path, line 1068) to pass `""` as `userID`
- [ ] Write a unit test in `git_repository_service_pull_requests.go` test file: repo has `OAuthConnectionID` = user A's connection; `CreatePullRequest` called with user B's ID who also has a GitHub OAuth connection → user B's token is used
- [ ] Run `go build ./pkg/services/ ./pkg/server/` to confirm no compile errors
