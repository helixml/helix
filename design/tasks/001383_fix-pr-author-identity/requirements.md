# Requirements: Fix PR Author Identity

## Problem

When a user clicks "Open PR" on a spec task, the resulting GitHub/GitLab pull request is authored by whoever *initialized* the external repository, not the user who clicked the button. This breaks PR attribution, notifications, and code review workflows.

**Root cause:** `getGitHubClient()` and `getGitLabClient()` in `git_repository_service_pull_requests.go` resolve credentials from `repo.OAuthConnectionID`, which is set at repo-creation time and never reflects the acting user.

## User Stories

**US-1:** As a developer who did not create the Helix repo connection, when I click "Open PR", the PR on GitHub/GitLab should be opened under *my* GitHub/GitLab account, not the repo initializer's.

**US-2:** As a developer with no GitHub OAuth connection, when I click "Open PR", the system should prompt me to connect my GitHub account via OAuth before proceeding. PRs must always be authored by the acting user -- never silently attributed to someone else via repo-level credentials.

**US-3:** As an automated agent creating a PR (no interactive user), the system should fall back to repo-level credentials (PAT / GitHub App / stored OAuth) since there is no user to prompt.

## Acceptance Criteria

1. When User A (not the repo initializer) clicks "Open PR", the resulting GitHub PR is authored by User A's GitHub account (their OAuth token is used).
2. If the acting user has no GitHub/GitLab OAuth connection, the backend returns a structured error (HTTP 422, `oauth_required` error code, with the provider type) instead of falling back to repo-level credentials. The frontend uses this to redirect the user to the OAuth connection flow.
3. After the user completes OAuth setup, they can retry "Open PR" and the PR is created under their account.
4. Agent-triggered PR paths (`git_http_server.go`, empty `userID`) continue to fall back to repo-level credentials -- no regression for automated workflows.
5. GitLab merge requests follow the same fix.
6. All existing unit tests in `git_repository_service_pull_requests.go` continue to pass.
7. A new test covers: repo has `OAuthConnectionID` = user A's connection; PR is created on behalf of user B who also has a GitHub OAuth connection -> user B's token is used, not user A's.
8. A new test covers: `CreatePullRequest` called with a `userID` that has no OAuth connection -> returns `oauth_required` error, does NOT fall back to repo-level credentials.
9. `git_http_server.go`'s internal PR creation path (agent-triggered) passes empty `userID` and stays on repo-level credentials.
