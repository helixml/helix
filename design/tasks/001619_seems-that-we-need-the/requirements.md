# Requirements: Add `workflow` Scope to GitHub Auth

## Background

When Helix pushes a branch to GitHub that includes changes to `.github/workflows/` files (e.g. from an upstream merge), GitHub rejects the push with:

> refusing to allow an OAuth App to create or update workflow `.github/workflows/...` without `workflow` scope

GitHub then rolls back the local ref on the middle git server too. The `workflow` scope is separate from `repo` and must be explicitly requested.

## User Stories

**US1: PAT Validation**
As a user connecting via a classic PAT, when I save my token, Helix should require the `workflow` scope (in addition to `repo`) and show a clear error if it's missing.

**US2: OAuth Flow**
As a user authenticating via GitHub OAuth, when Helix requests authorization, the `workflow` scope should be included in the requested scopes so the resulting token can push to `.github/workflows/`.

**US3: GitHub Skill OAuth**
As a user using the GitHub skill (or GitHub Issues skill), the OAuth scopes requested should include `workflow` so workflow-related operations work.

## Acceptance Criteria

- PAT validation in `validateAndFetchUserInfo` checks for both `repo` AND `workflow` scopes; missing `workflow` produces a descriptive error referencing `https://github.com/settings/tokens`
- Frontend PAT helper text updated to mention `workflow` scope requirement
- `github.yaml` and `github_issues.yaml` skill definitions include `workflow` in their `oauth.scopes` list
- OAuth flows initiated for GitHub include `workflow` in the scope request
