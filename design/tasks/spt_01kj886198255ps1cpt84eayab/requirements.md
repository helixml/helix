# Requirements: PR/MR Authored by Acting User

## Problem

When a user clicks "Open PR", the GitHub PR is created using the OAuth token of whoever *initialized* the repository, not the user who clicked. This misattributes authorship, breaks notifications, and confuses code review workflows.

## User Stories

**Story 1 — PR authored by acting user**
> As a team member who did not connect the repository, when I click "Open PR" on a spec task, I want the resulting GitHub PR to be authored by my GitHub account, so that I receive notifications and the PR is correctly attributed to me.

**Story 2 — OAuth prompt when not connected**
> As a team member who has not yet linked my GitHub account, when I click "Open PR", I want to be taken through the GitHub OAuth flow so that my account is connected and the PR is correctly attributed to me.

**Story 3 — GitLab parity**
> As a GitLab user, when I click "Open PR" the resulting merge request should be authored by my GitLab account, following the same logic as GitHub.

## Acceptance Criteria

1. User B (non-initializer) clicks "Open PR" → PR on GitHub is authored by User B's GitHub account (their OAuth token is used).
2. If the acting user has no GitHub OAuth connection, the system initiates the GitHub OAuth flow for that user instead of silently falling back to repo-level credentials.
3. After the user completes OAuth and their account is linked, opening a PR succeeds and is attributed to them.
4. GitLab merge requests follow the same fix.
5. All existing unit tests in `git_repository_service_pull_requests.go` continue to pass.
6. New test: repo has `OAuthConnectionID` = User A's connection; PR created on behalf of User B who also has a GitHub OAuth connection → User B's token is used.
