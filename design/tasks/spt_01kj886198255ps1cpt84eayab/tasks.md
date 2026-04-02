# Implementation Tasks

- [x] In `git_repository_service_pull_requests.go`: add `userID string` param to `CreatePullRequest`, `createGitHubPullRequest`, and `getGitHubClient`; in `getGitHubClient`, list user's connections (ListOAuthConnections) and find one with `Provider.Type == OAuthProviderTypeGitHub`; if userID non-empty and no connection found, return error; if userID empty, fall through to existing repo-level creds
- [x] In `git_repository_service_pull_requests.go`: apply the same `userID` threading to `createGitLabMergeRequest` and `getGitLabClient`
- [x] In `git_repository_handlers.go`: in `createGitRepositoryPullRequest`, check if repo is GitHub/GitLab; list user's connections; if no matching connection, find provider and call `s.oauthManager.StartOAuthFlow(...)`, return 401 with `{oauth_required, auth_url}`; if connection found, pass `user.ID` to `CreatePullRequest`
- [x] In `spec_task_workflow_handlers.go`: check user OAuth connection upfront in `approveImplementation`; return 400 if missing; add `userID` param to `ensurePullRequestForTask`; pass `user.ID` through
- [x] Write new unit test: repo `OAuthConnectionID` = User A's connection; PR created on behalf of User B who has a GitHub OAuth connection → User B's token is used
- [x] Verify all existing unit tests in `git_repository_service_pull_requests.go` pass (update call sites to pass empty `userID` where needed)
