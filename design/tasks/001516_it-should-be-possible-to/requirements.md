# Requirements: Unlink GitHub Token & Validate Token Scopes

## Background

Users can connect GitHub via PAT (Personal Access Token) in the "Browse Providers" dialog (`BrowseProvidersDialog.tsx`). Once saved, there is **no way to unlink/remove a saved PAT connection from the UI** — the `deletePatConnection` hook is imported but never wired to any button. Additionally, tokens are validated by calling `GetAuthenticatedUser` on the GitHub API, but **no scope validation** is performed — a token with zero scopes would be accepted and silently fail later when trying to list repos.

## User Stories

### US-1: Unlink a saved GitHub PAT connection
**As a** user who previously saved a GitHub PAT connection,
**I want to** remove/unlink it from the UI,
**So that** I can disconnect or replace it without needing admin help.

### US-2: Validate token scopes before saving
**As a** user entering a GitHub PAT,
**I want to** be told immediately if my token lacks the required scopes (e.g., `repo`),
**So that** I don't save a broken token that silently fails.

## Acceptance Criteria

### AC-1: Unlink PAT connection
- In the `choose-method` view of `BrowseProvidersDialog`, when a PAT connection exists, a "Disconnect" / "Unlink" button is visible next to or below the saved connection info.
- Clicking "Disconnect" shows a confirmation dialog.
- On confirm, the connection is deleted via the existing `DELETE /api/v1/git-provider-connections/{id}` endpoint.
- After deletion, the UI returns to showing the PAT entry form (no saved connection).
- Query cache is invalidated so the provider list updates.

### AC-2: Validate GitHub token scopes on save
- When creating a GitHub PAT connection (`POST /api/v1/git-provider-connections`), the backend checks the `X-OAuth-Scopes` response header from the GitHub API call.
- If the `repo` scope is missing, the API returns a 400 error with a clear message like: `"GitHub token is missing required scope 'repo'. Please create a token with the 'repo' scope."`.
- The frontend displays this error to the user in the PAT entry form.
- Tokens with sufficient scopes (e.g., `repo` present) are accepted as before.
- Fine-grained PATs (which don't return `X-OAuth-Scopes`) should still be accepted if the `GetAuthenticatedUser` call succeeds — scope validation is best-effort.

### AC-3: Testability
- An invalid/random string as a token is rejected (GitHub API call fails → 400 error shown to user).
- A valid-format token missing `repo` scope is rejected with a scope-specific error message.
- The error messages are surfaced in the UI (not just console logs).

## Out of Scope
- OAuth connection unlinking (already has a delete button in `OAuthConnections.tsx`).
- Scope validation for GitLab, Bitbucket, or Azure DevOps PATs.
- Editing/updating an existing PAT (user can unlink and re-add).