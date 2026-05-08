# Design: Fix public org repos still missing in GitHub repo picker

## Problem

The 4-step `ListRepositories()` fix from task 1725 is deployed but ineffective because Step 2 (`GET /user/orgs`) silently fails without `read:org` scope. The token scope checks don't require or warn about this scope.

## Key Files

| File | What to change |
|------|----------------|
| `frontend/src/components/project/BrowseProvidersDialog.tsx:74` | Add `read:org` to `GITHUB_REQUIRED_SCOPES` |
| `frontend/src/components/project/BrowseProvidersDialog.tsx:1065` | Update PAT helper text to mention `read:org` |
| `api/pkg/server/git_repository_handlers.go:1710-1719` | Add `read:org` scope warning in PAT validation |
| `api/pkg/agent/skill/github/client.go:132-137` | Log warning when org listing fails instead of silent fallback |

## Changes

### 1. Frontend: Add `read:org` to required scopes (BrowseProvidersDialog.tsx)

```typescript
// Line 74: was ["repo", "workflow"]
const GITHUB_REQUIRED_SCOPES = ["repo", "workflow", "read:org"];
```

This triggers the existing scope upgrade flow for OAuth connections that have `repo` + `workflow` but not `read:org`. The user sees a warning banner and can click "Connect via OAuth" to re-authorize with the full scope set (which already includes `read:org` at line 423).

No other changes needed for the OAuth scope upgrade — the existing mechanism handles it.

### 2. Frontend: Update PAT helper text (BrowseProvidersDialog.tsx)

Update the GitHub PAT helper text (line ~1065) to mention `read:org`:

```typescript
"Create a classic token with 'repo' and 'read:org' scopes at GitHub → Settings → Developer settings → Personal access tokens"
```

### 3. Backend: Warn when `read:org` is missing from PAT (git_repository_handlers.go)

After the existing `repo` scope check (line ~1719), add a non-blocking warning log when `read:org` is absent. Don't block the request — the user can still see Step 1 repos. But return a header or response field so the frontend can optionally show a notice.

Actually, simpler approach: just log a server-side warning. The PAT helper text update (change 2) handles the user-facing guidance. No need for a new response field.

```go
hasReadOrg := false
for _, s := range scopes {
    if s == "read:org" {
        hasReadOrg = true
        break
    }
}
if !hasReadOrg {
    log.Warn().Msg("GitHub PAT missing 'read:org' scope — org repo listing may be incomplete")
}
```

### 4. Backend: Log warning on org listing failure (client.go)

Change line 136 from a silent return to a logged warning:

```go
orgs, err := c.listUserOrganizations(ctx)
if err != nil {
    log.Warn().Err(err).Msg("Failed to list user organizations — org-level repo listing will be skipped")
    return allRepos, nil
}
```

This preserves the non-fatal behavior but makes the failure visible in server logs for debugging.

## What this does NOT change

- The 4-step `ListRepositories()` logic itself — it's correct, just not reaching Steps 2-3
- The OAuth auth URL — it already requests `read:org` (line 423)
- The org filter dropdown UI — it works correctly once repos are populated
- Fine-grained PAT handling — already rejected with a clear message

## Testing

1. **OAuth with old connection**: Connect GitHub via OAuth, verify scope upgrade banner appears, re-authorize, confirm public org repos now show
2. **OAuth with new connection**: Fresh OAuth connection should work immediately (auth URL already includes `read:org`)
3. **PAT with `repo` only**: Connect with PAT that has only `repo` scope — user should see helper text mentioning `read:org`, and server logs should show the warning
4. **PAT with `repo` + `read:org`**: Should see all repos including public org repos
5. **Org filter**: After fixing scopes, select an org from dropdown — only that org's repos should show

## Codebase Patterns Found

- Scope upgrade mechanism: `GITHUB_REQUIRED_SCOPES` array + `hasRequiredScopes()` utility in `frontend/src/utils/oauthProviders.ts` — checks if connection has all required scopes, triggers re-auth banner if not
- PAT scope validation: `GetAuthenticatedUserWithScopes()` in `client.go:296` reads `X-OAuth-Scopes` response header to determine classic PAT scopes
- Fine-grained PATs don't return `X-OAuth-Scopes` header — detected by `scopeHeaderPresent` flag and already rejected
