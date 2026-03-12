# Implementation Tasks

## Backend: GitHub token scope validation

- [~] Add `GetAuthenticatedUserWithScopes(ctx) (*github.User, []string, error)` method to `api/pkg/agent/skill/github/client.go` that calls `c.client.Users.Get(ctx, "")` and parses the `X-OAuth-Scopes` response header into a string slice
- [ ] Update the `ExternalRepositoryTypeGitHub` case in `validateAndFetchUserInfo()` in `api/pkg/server/git_provider_connection_handlers.go` to call `GetAuthenticatedUserWithScopes` instead of `GetAuthenticatedUser`
- [ ] After getting scopes, if the scopes list is non-empty (classic PAT) and `repo` is not present, return an error with a message listing the token's actual scopes and linking to `https://github.com/settings/tokens`
- [ ] If scopes list is empty (fine-grained PAT or GitHub App), skip scope validation — accept the token if `GetAuthenticatedUser` succeeded
- [ ] Verify: `go build ./api/pkg/server/ ./api/pkg/agent/skill/github/`

## Frontend: Unlink/disconnect saved PAT connection

- [ ] In `BrowseProvidersDialog.tsx`, add state: `const [patToDisconnect, setPatToDisconnect] = useState<string | null>(null)`
- [ ] In the `choose-method` view, next to the PAT "Saved" chip (around L640-645), add an `IconButton` with a `Trash` icon that sets `patToDisconnect` to the connection ID
- [ ] Add a confirmation `Dialog` (follow the pattern in `ClaudeSubscriptionConnect.tsx`): title "Disconnect Token", body "Remove this saved token? You can re-enter it later.", Cancel + Disconnect buttons
- [ ] On confirm, call `deletePatConnection.mutateAsync(patToDisconnect)`, show snackbar success/error, and clear `patToDisconnect`
- [ ] Verify: `cd frontend && yarn build`

## Manual testing

- [ ] Enter a random string (e.g. `abc123`) as a GitHub PAT → confirm backend returns 400 and error is displayed in the UI
- [ ] Enter a valid-format GitHub classic PAT that lacks `repo` scope → confirm the scope-specific error message appears
- [ ] Save a valid PAT connection → confirm it works, then click Disconnect → confirm the connection is removed and UI returns to PAT entry form