# Design: Add `workflow` Scope to GitHub Auth

## Context

GitHub's `workflow` scope is required to push files under `.github/workflows/`. It is NOT included in `repo` and must be explicitly requested. The Connected Services page has no connect buttons (it doesn't know what scopes to request), so scope upgrades must happen inline in the repo browser.

## Files to Change

### 1. Backend: PAT validation
**`api/pkg/server/git_provider_connection_handlers.go`** — `validateAndFetchUserInfo()` (~lines 308-317)

Currently checks only for `repo`. Add parallel check for `workflow`:

```go
hasRepo := false
hasWorkflow := false
for _, s := range scopes {
    if s == "repo" { hasRepo = true }
    if s == "workflow" { hasWorkflow = true }
}
var missing []string
if !hasRepo { missing = append(missing, "repo") }
if !hasWorkflow { missing = append(missing, "workflow") }
if len(missing) > 0 {
    return nil, fmt.Errorf("GitHub token is missing required scopes: %s. Your token has: %s. Create a classic token at https://github.com/settings/tokens", strings.Join(missing, ", "), strings.Join(scopes, ", "))
}
```

### 2. GitHub Skill YAML files
Not changed. These define OAuth scopes for LLM agent tool use (listing issues, creating PRs etc.) — a separate OAuth connection from the git integration. The git browsing/push flow uses the `OAuthConnection` created via `BrowseProvidersDialog`, not the skill connection.

### 3. Frontend: BrowseProvidersDialog scopes
**`frontend/src/components/project/BrowseProvidersDialog.tsx`** (~line 407)

Add `workflow` to the GitHub scopes parameter:
```ts
scopesParam = "?scopes=repo,workflow,read:org,read:user,user:email";
```

### 4. Frontend: Inline scope-upgrade prompt in BrowseProvidersDialog
**`frontend/src/components/project/BrowseProvidersDialog.tsx`**

When the dialog opens with an existing GitHub OAuth connection, check whether the connection's stored scopes include `workflow`. If not, show an inline alert instead of (or before) trying to list repos:

> "Your GitHub connection needs to be updated to include the `workflow` scope. [Reconnect with GitHub]"

Clicking "Reconnect" triggers the same `startOAuthFlow` call used for first-time connect (with `workflow` in the scopes). After OAuth completes and the callback is handled, the old connection is replaced and the dialog can proceed normally.

The existing connection's scopes are available from the `OAuthConnection` object returned by `useListOAuthConnections`. Use the existing `hasRequiredScopes()` utility in `frontend/src/utils/oauthProviders.ts` to check.

### 5. Frontend: PAT helper text
**`frontend/src/components/project/forms/ExternalRepoForm.tsx`** (~line 224)

Update helper text from `(needs repo scope for private repos)` to `(needs repo and workflow scopes)`.

## Notes

- The `workflow` scope grants write access to `.github/workflows/` files only; no broader permissions.
- Fine-grained PATs still get rejected (no `X-OAuth-Scopes` header), no change needed there.
- No changes to `oauth2.go` or `manager.go` — scope plumbing already works.
- The inline upgrade check in `BrowseProvidersDialog` is a client-side comparison of stored scopes; no new backend endpoint is needed.
