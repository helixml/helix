# Requirements: Fix PR Author Identity

## Problem

When a user clicks "Open PR" on a spec task, the resulting GitHub/GitLab pull request is authored by whoever *initialized* the external repository, not the user who clicked the button. This breaks PR attribution, notifications, and code review workflows.

**Root cause:** `getGitHubClient()` and `getGitLabClient()` in `git_repository_service_pull_requests.go` resolve credentials from `repo.OAuthConnectionID`, which is set at repo-creation time and never reflects the acting user.

## User Stories

**US-1:** As a developer who did not create the Helix repo connection, when I click "Open PR", the PR on GitHub/GitLab should be opened under *my* GitHub/GitLab account, not the repo initializer's.

**US-2:** As a developer with no GitHub OAuth connection, when I click "Open PR", the system should fall back to repo-level credentials (PAT / GitHub App / stored OAuth) so existing setups keep working.

## Acceptance Criteria

1. When User A (not the repo initializer) clicks "Open PR", the resulting GitHub PR is authored by User A's GitHub account (their OAuth token is used).
2. If the acting user has no GitHub/GitLab OAuth connection, fall back to repo-level credentials — no regression for existing setups.
3. GitLab merge requests follow the same fix.
4. All existing unit tests in `git_repository_service_pull_requests.go` continue to pass.
5. A new test covers: repo has `OAuthConnectionID` = user A's connection; PR is created on behalf of user B who also has a GitHub OAuth connection → user B's token is used, not user A's.
6. `git_http_server.go`'s internal PR creation path (agent-triggered) also passes a user ID where available, or stays on repo-level credentials if none.
