# Design: Unlink GitHub Token & Validate Token Scopes

## Overview

Two changes: (1) add a "Disconnect" button for saved PAT connections in the BrowseProvidersDialog, and (2) add server-side GitHub token scope validation before accepting a PAT.

## Codebase Context

### Existing patterns discovered

- **PAT connections** are managed via `GitProviderConnection` (type in `api/pkg/types/oauth.go`), with CRUD handlers in `api/pkg/server/git_provider_connection_handlers.go`.
- **Frontend service layer**: `frontend/src/services/gitProviderConnectionService.ts` already exports `useDeleteGitProviderConnection()` — it's imported in `BrowseProvidersDialog.tsx` but never used in any UI element.
- **Token validation** happens in `validateAndFetchUserInfo()` which calls `github.NewClientWithPATAndBaseURL(token, baseURL).GetAuthenticatedUser(ctx)`. This validates the token is real but does **not** check scopes.
- **GitHub scope header**: Every GitHub API response includes an `X-OAuth-Scopes` header listing the token's granted scopes (comma-separated). The `go-github/v57` library does not expose this in its `Response` struct, but the underlying `*http.Response` is embedded, so `resp.Header.Get("X-OAuth-Scopes")` works.
- **Disconnect pattern**: The `ClaudeSubscriptionConnect.tsx` component implements a disconnect flow with confirmation dialog — good reference for the UX pattern.
- **Error display**: `BrowseProvidersDialog` already has a `snackbar` instance and uses `patReposError` state for showing errors in the PAT flow.

### Required scopes

The codebase requests `repo,read:org,read:user,user:email` during OAuth flows (`BrowseProvidersDialog.tsx` L298, `CreateProjectDialog.tsx` L227). For PAT validation, the minimum required scope is `repo` — this is what's needed to list and clone private repositories. The GitHub MCP skill link also requests `repo,read:org,read:user`.

## Design

### Change 1: Unlink PAT connection (frontend only)

**File**: `frontend/src/components/project/BrowseProvidersDialog.tsx`

In the `choose-method` view (around line 620-650), when `patConnection` exists, add a small "Disconnect" icon button next to the "Saved" chip. Clicking it opens a confirmation dialog. On confirm, call `deletePatConnection.mutateAsync(patConnection.id)`, which already invalidates the query cache via `onSuccess` in the service hook.

```
// Pseudocode for the UI addition
{patConnection && (
  <IconButton onClick={() => setPatToDisconnect(patConnection.id)}>
    <Trash size={16} />
  </IconButton>
)}

// Confirmation dialog (reuse MUI Dialog pattern from ClaudeSubscriptionConnect)
<Dialog open={!!patToDisconnect} onClose={() => setPatToDisconnect(null)}>
  <DialogTitle>Disconnect Token</DialogTitle>
  <DialogContent>
    <DialogContentText>Remove this saved token? You can re-enter it later.</DialogContentText>
  </DialogContent>
  <DialogActions>
    <Button onClick={() => setPatToDisconnect(null)}>Cancel</Button>
    <Button color="error" onClick={handleDisconnectPat}>Disconnect</Button>
  </DialogActions>
</Dialog>
```

New state: `const [patToDisconnect, setPatToDisconnect] = useState<string | null>(null)`

Handler:
```
const handleDisconnectPat = async () => {
  if (!patToDisconnect) return
  try {
    await deletePatConnection.mutateAsync(patToDisconnect)
    snackbar.success('Token disconnected')
    setPatToDisconnect(null)
  } catch (err) {
    snackbar.error('Failed to disconnect token')
  }
}
```

### Change 2: Validate GitHub token scopes (backend)

**File**: `api/pkg/server/git_provider_connection_handlers.go`

Modify `validateAndFetchUserInfo()` for the GitHub case. Currently:

```go
client := github.NewClientWithPATAndBaseURL(token, baseURL)
user, err := client.GetAuthenticatedUser(ctx)
```

The `GetAuthenticatedUser` call returns `(*github.User, *github.Response, error)` but the current `Client.GetAuthenticatedUser` method in `api/pkg/agent/skill/github/client.go` discards the `*github.Response`. We need to either:

**Option A** (chosen): Add a new method to the GitHub client that returns scopes along with the user.

**File**: `api/pkg/agent/skill/github/client.go`

```go
// GetAuthenticatedUserWithScopes gets the user profile and returns token scopes from the response header.
// Returns the user, a list of OAuth scopes, and any error.
// Fine-grained PATs may return empty scopes — callers should treat empty scopes as "unknown" rather than "none".
func (c *Client) GetAuthenticatedUserWithScopes(ctx context.Context) (*github.User, []string, error) {
    user, resp, err := c.client.Users.Get(ctx, "")
    if err != nil {
        return nil, nil, fmt.Errorf("failed to get authenticated user: %w", err)
    }
    var scopes []string
    if scopeHeader := resp.Header.Get("X-OAuth-Scopes"); scopeHeader != "" {
        for _, s := range strings.Split(scopeHeader, ",") {
            s = strings.TrimSpace(s)
            if s != "" {
                scopes = append(scopes, s)
            }
        }
    }
    return user, scopes, nil
}
```

Then in `validateAndFetchUserInfo()`:

```go
case types.ExternalRepositoryTypeGitHub:
    client := github.NewClientWithPATAndBaseURL(token, baseURL)
    user, scopes, err := client.GetAuthenticatedUserWithScopes(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to validate GitHub token: %w", err)
    }
    // Check scopes — only enforce if scopes were returned (classic PATs).
    // Fine-grained PATs don't return X-OAuth-Scopes, so scopes will be empty.
    if len(scopes) > 0 {
        hasRepo := false
        for _, s := range scopes {
            if s == "repo" {
                hasRepo = true
                break
            }
        }
        if !hasRepo {
            return nil, fmt.Errorf("GitHub token is missing required scope 'repo'. Your token has scopes: %s. Please create a token with the 'repo' scope at https://github.com/settings/tokens", strings.Join(scopes, ", "))
        }
    }
    return &types.OAuthUserInfo{...}, nil
```

**Why Option A over alternatives:**
- **Option B** (make a raw HTTP call to `https://api.github.com/user` separately) duplicates auth logic.
- **Option C** (only validate on frontend) is insecure — backend should be the source of truth.
- Option A keeps the change minimal and co-located with existing GitHub client code.

### Error propagation

The backend already returns the error message in the HTTP 400 response body (line ~100 of the handler: `http.Error(w, fmt.Sprintf("Invalid token or failed to connect: %s", err.Error()), http.StatusBadRequest)`). The frontend `createPatConnection.mutateAsync()` and `fetchReposWithPat()` both surface errors via `snackbar.error()` or `patReposError` state. No additional frontend error handling is needed — the error message from the backend will flow through naturally.

## Files to Modify

| File | Change |
|------|--------|
| `api/pkg/agent/skill/github/client.go` | Add `GetAuthenticatedUserWithScopes()` method |
| `api/pkg/server/git_provider_connection_handlers.go` | Use new method in `validateAndFetchUserInfo()`, check for `repo` scope |
| `frontend/src/components/project/BrowseProvidersDialog.tsx` | Add disconnect button + confirmation dialog for saved PAT connections |

## Testing Plan

1. **Invalid token string**: Enter `"abc123"` as a GitHub PAT → backend returns 400, frontend shows error.
2. **Valid token, missing `repo` scope**: Create a GitHub PAT with only `read:user` scope → backend returns 400 with scope-specific message.
3. **Valid token, correct scopes**: Create a GitHub PAT with `repo` scope → connection saved successfully.
4. **Fine-grained PAT**: If available, test with a fine-grained PAT → should be accepted (no scope header = skip check).
5. **Disconnect flow**: Save a PAT connection, click Disconnect, confirm → connection removed, UI shows PAT entry form again.
6. **Go build**: `go build ./api/pkg/server/ ./api/pkg/agent/skill/github/`
7. **Frontend build**: `cd frontend && yarn build`
