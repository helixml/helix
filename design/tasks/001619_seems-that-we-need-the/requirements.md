# Requirements: Add `workflow` Scope to GitHub Auth

## Background

When Helix pushes a branch to GitHub that includes changes to `.github/workflows/` files (e.g. from an upstream merge), GitHub rejects the push with:

> refusing to allow an OAuth App to create or update workflow `.github/workflows/...` without `workflow` scope

The Helix middle git server then rolls back the local ref too. The `workflow` scope is separate from `repo` and must be explicitly requested.

## Upgrading Existing Connections

Existing GitHub OAuth connections will not automatically gain the `workflow` scope. The Connected Services page does not have connect buttons (it can't know what scopes to request), so users can't upgrade from there. The upgrade must happen **inline** during GitHub integration use — specifically in the repo browser (`BrowseProvidersDialog`).

When the repo browser detects the existing connection is missing required scopes, it should show an inline prompt offering to reconnect with the correct scopes. The reconnect starts a new OAuth flow (same as first-time connect, scopes come from the dialog) and replaces the old connection.

## User Stories

**US1: PAT Validation**
As a user connecting via a classic PAT, when I save my token, Helix should require the `workflow` scope (in addition to `repo`) and show a clear error if it's missing.

**US2: New OAuth Connections**
As a user authenticating via GitHub OAuth for the first time, the `workflow` scope should be included in the authorization request so the resulting token can push to `.github/workflows/`.

**US3: Existing Connection Upgrade**
As a user with an existing GitHub OAuth connection that is missing the `workflow` scope, when I open the repo browser, I should see an inline prompt to reconnect with the required scopes rather than a silent failure.

**US4: GitHub Skill OAuth**
As a user using the GitHub skill (or GitHub Issues skill), the OAuth scopes requested should include `workflow`.

## Acceptance Criteria

- PAT validation in `validateAndFetchUserInfo` checks for both `repo` AND `workflow` scopes; missing `workflow` produces a descriptive error referencing `https://github.com/settings/tokens`
- Frontend PAT helper text updated to mention `workflow` scope requirement
- `github.yaml` and `github_issues.yaml` skill definitions include `workflow` in their `oauth.scopes` list
- `BrowseProvidersDialog` includes `workflow` in the GitHub scopes parameter when starting OAuth
- When the repo browser opens and the existing GitHub connection is missing required scopes, an inline prompt is shown allowing the user to reconnect (re-run the OAuth flow with the correct scopes)
