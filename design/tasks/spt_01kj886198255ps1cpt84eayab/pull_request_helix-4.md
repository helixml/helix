# Use acting user's OAuth token when opening a PR

## Summary

PRs opened via "Open PR" were attributed to whoever originally connected the repository, not the user who clicked the button. This fixes that by requiring each user to have their own GitHub/GitLab OAuth connection and using their token for PR/MR creation.

## Changes

- `git_repository_service_pull_requests.go`: Added `userID string` to `CreatePullRequest`, `createGitHubPullRequest`, `createGitLabMergeRequest`, `getGitHubClient`, and `getGitLabClient`. When `userID` is non-empty, the acting user's OAuth connection is looked up via `ListOAuthConnections` and used; if none is found, an error is returned. When `userID` is empty (background/automated calls), existing repo-level credential fallback is preserved.
- `git_repository_handlers.go`: `createGitRepositoryPullRequest` now calls `ensureUserOAuthConnection` before creating the PR. If the user has no matching GitHub/GitLab OAuth connection, the handler returns HTTP 401 with `{oauth_required, auth_url, provider_type}` so the frontend can redirect the user to connect their account.
- `spec_task_workflow_handlers.go`: `approveImplementation` checks the user's OAuth connection before updating task status. Returns HTTP 400 with a clear message if missing. `ensurePullRequestForTask` accepts `userID` and threads it to `CreatePullRequest`.
- `git_http_server.go`: Updated the background PR creation call to pass empty `userID` (retains repo-level credentials for automated push-triggered PR creation).
- New unit tests in `git_repository_service_pull_requests_test.go` cover: acting user's token used when available, error when no user connection, and fallback to repo OAuth when `userID` is empty.
