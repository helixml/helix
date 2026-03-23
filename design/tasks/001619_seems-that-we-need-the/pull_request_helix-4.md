# Add workflow scope to GitHub OAuth and PAT auth

## Summary

GitHub requires the `workflow` scope to push changes to `.github/workflows/` files. Without it, pushes are rejected with "refusing to allow an OAuth App to create or update workflow ... without `workflow` scope". This adds the scope in all the right places.

## Changes

- **PAT validation** (`git_provider_connection_handlers.go`): require both `repo` and `workflow` scopes; missing scopes are reported together in one clear error message
- **Repo browser** (`BrowseProvidersDialog.tsx`):
  - Add `workflow` to the GitHub scopes string when starting an OAuth flow
  - Add an inline warning alert when an existing GitHub connection is missing the `workflow` scope, with a prompt to reconnect (starts a new OAuth flow with all required scopes)
- **PAT helper text** (`ExternalRepoForm.tsx`): mention `workflow` scope alongside `repo`
