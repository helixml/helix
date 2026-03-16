# Design: Contextual OAuth — Remove Generic Connect, Add Pre-Session Prompt

## Part 1: Remove Connect from Connected Services Page

**File:** `frontend/src/components/account/OAuthConnections.tsx`

Remove:
- The "Available Integrations" section (~lines 782-816) with Connect buttons
- The `openConnectDialog()`, `startOAuthFlow()`, `connectProvider` functions (~lines 222-282) and any state they own

Keep:
- The "Connected Services" list showing existing connections with disconnect and refresh

---

## Part 2: Pre-Session OAuth Prompt

### Where OAuth requirements live

OAuth is defined at the **tool level** inside an agent's config, not at the agent level:

```
App
  └── assistants[]
        └── apis[]                 ← ToolAPIConfig
              ├── oauth_provider   string
              └── oauth_scopes     []string
        └── mcp_servers[]          ← ToolMCPClientConfig
              ├── oauth_provider   string
              └── oauth_scopes     []string
```

The frontend already loads the full app config via `GET /api/v1/apps/{id}`. The user's existing connections are available via `GET /api/v1/oauth/connections`.

### Backend: new endpoint to aggregate OAuth requirements

Add `GET /api/v1/apps/{id}/oauth-requirements` that:
1. Loads the app config
2. Iterates `assistants[].apis[]` and `assistants[].mcp_servers[]`
3. Collects unique `{provider, scopes}` pairs across all tools
4. Returns them as a list

This keeps the aggregation logic server-side and avoids duplicating the nested traversal in the frontend. The backend already does equivalent traversal in `getAppOAuthTokenEnv()` (`app_handlers.go:1167`).

Response shape:
```json
[
  { "provider_name": "github", "scopes": ["repo", "user:read"] },
  { "provider_name": "gitlab", "scopes": ["read_repository"] }
]
```

### Frontend: check requirements before session starts

In the agent chat/session component, before allowing the user to send the first message (or on page load when an agent is loaded):

1. Call `GET /api/v1/apps/{id}/oauth-requirements`
2. Call `GET /api/v1/oauth/connections` to get the user's current connections
3. For each required `{provider, scopes}`, check if the user has a connection for that provider that covers all required scopes (reuse `getMissingScopes` logic, or implement equivalent client-side)
4. If any requirements are unmet, render a banner or dialog listing the missing providers with a Connect button per provider
5. Each Connect button calls `GET /api/v1/oauth/flow/start/{provider_id}?scopes=<scopes>` with the exact scopes from the requirement — same pattern as `BrowseProvidersDialog`
6. On successful OAuth callback, re-check requirements and hide the prompt if all are now satisfied

### Scope matching

A connection satisfies a requirement if every required scope is present in the connection's stored `scopes` field. The backend already has `getMissingScopes()` (`oauth/manager.go`); the frontend can replicate this simple set-containment check or the new endpoint can optionally accept the user's token and return which requirements are unmet.

---

## Key Files

| File | Change |
|------|--------|
| `frontend/src/components/account/OAuthConnections.tsx` | Remove connect UI; keep list + disconnect + refresh |
| `api/pkg/server/app_handlers.go` | Add `GET /api/v1/apps/{id}/oauth-requirements` handler |
| `api/pkg/server/server.go` or router file | Register new route |
| `frontend/src/components/` (session/chat component) | Add pre-session OAuth requirement check and prompt |

## Notes for Implementors

- The new endpoint should only be callable by authenticated users and should verify the app is accessible to the requesting user (same auth check as `getApp`).
- Skill-based OAuth (YAML skills with `oauth.provider`) is already resolved server-side into `ToolAPIConfig` fields before session execution, so the endpoint can work entirely from the stored app config without needing to read YAML skill files.
- The scope check in the frontend needs to handle the case where `provider_name` refers to a configured provider instance (e.g. a named GitHub provider), not just a raw type string — look at how `BrowseProvidersDialog` resolves provider IDs to confirm the right identifier to use.
- The popup/callback OAuth mechanism from `useOAuthFlow.ts` is reusable as-is.
