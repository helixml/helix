# Design: Fix GitHub OAuth Missing Repo Scope

## Root Cause

Two independent issues compound the problem:

1. **`OAuthConnections.tsx` calls the OAuth start endpoint with no scope parameter.**
   `GET /api/v1/oauth/flow/start/{provider}` — no `?scopes=` query string.
   The backend (`oauth.go:203`) treats scopes as optional and passes an empty slice to `GetAuthorizationURL`, which sets `oauth2.Config.Scopes = []string{}`. GitHub then applies its own defaults (public-only access).

2. **Scope definitions are duplicated across frontend components rather than centralised.**
   `CreateProjectDialog.tsx:193` and `BrowseProvidersDialog.tsx:408` each hardcode `repo,read:org,read:user,user:email` inline, so new entry points tend to omit them.

## Fix

### Minimal fix (OAuthConnections.tsx)

Update the `connectProvider` call in `OAuthConnections.tsx` (~line 282) to append scopes for GitHub (and GitLab, mirroring the pattern in `BrowseProvidersDialog`):

```typescript
// Before
const response = await api.get(`/api/v1/oauth/flow/start/${normalizedProviderId}`)

// After — determine scopes by provider type
const scopesByProvider: Record<string, string> = {
  github: 'repo,read:org,read:user,user:email',
  gitlab: 'read_repository,write_repository,read_user',
}
const scopesParam = scopesByProvider[providerType]
  ? `?scopes=${scopesByProvider[providerType]}`
  : ''
const response = await api.get(`/api/v1/oauth/flow/start/${normalizedProviderId}${scopesParam}`)
```

Where `providerType` is the normalised provider type string already available in the component context.

### Optional: centralise scope defaults (recommended follow-up)

Move the scope map into a shared helper (e.g. `src/utils/oauthScopes.ts`) so all three call sites import from one place. The `useOAuthFlow` hook is the natural home if it is used consistently.

## Key Files

| File | Change |
|------|--------|
| `frontend/src/components/account/OAuthConnections.tsx` | Add `?scopes=` parameter when starting OAuth (primary fix) |
| `frontend/src/utils/oauthScopes.ts` *(optional new file)* | Centralise provider→scope mapping |
| `frontend/src/hooks/useOAuthFlow.ts` | Already handles optional scopes correctly — no change needed |

## Notes for Implementors

- The backend already supports per-request scopes and the `getMissingScopes` / re-auth flow works correctly. No backend changes needed.
- Scope strings must exactly match what GitHub accepts (lowercase, comma-separated, no spaces after encoding).
- Existing connections without the repo scope will not be automatically re-authorised — users need to disconnect and reconnect, or the re-auth prompt triggered by `GetTokenForTool` will handle it on next use.
- Pattern used by `BrowseProvidersDialog.tsx:400-408` is the reference implementation.
