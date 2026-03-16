# Design: Remove Generic OAuth Connect from Connected Services Page

## Root Cause

The OAuth scope problem is architectural, not a missing parameter. The Connected Services page is a generic account-settings view that has no knowledge of what any given agent or feature needs. Adding scope defaults there would be a hack that breaks for any provider or use case that needs different scopes.

The correct fix is to remove the connect capability from that page entirely. OAuth connections should always be initiated by the feature that needs them, because only that feature knows the required scopes.

## Current Architecture

```
Connected Services page (OAuthConnections.tsx)
  └── "Connect" button → GET /api/v1/oauth/flow/start/{provider}   ← NO scopes = bug

CreateProjectDialog.tsx
  └── "Connect GitHub" → GET /api/v1/oauth/flow/start/{provider}?scopes=repo,...  ✓

BrowseProvidersDialog.tsx
  └── "Connect GitHub" → GET /api/v1/oauth/flow/start/{provider}?scopes=repo,...  ✓

Agent skill execution (tools_api_run_action.go)
  └── GetTokenForTool() checks scopes → silent error if missing  ← no user prompt yet
```

## Change: Simplify OAuthConnections.tsx to Read-Only

**File:** `frontend/src/components/account/OAuthConnections.tsx`

Remove or hide:
- The "Available Integrations" section (lines ~782-816) that lists providers and shows Connect buttons
- The `openConnectDialog()` / `startOAuthFlow()` functions (~lines 222-282)
- Any UI that lets users initiate a new connection

Keep:
- The "Connected Services" section listing existing connections
- Disconnect (delete) button per connection
- Refresh token button per connection
- Status/profile info per connection

The page becomes a view for managing connections that were created elsewhere.

## Known Gap: No Pre-Session OAuth Prompt for Agents

Skill YAML files declare OAuth requirements (`oauth.provider`, `oauth.scopes`). The backend reads these and validates scopes at tool execution time (`GetTokenForTool` in `oauth/manager.go`). However:

- There is no pre-session check that reads the agent's skill definitions and verifies the user has a valid connection before starting.
- There is no in-session UI prompt that triggers OAuth when a tool call fails due to missing scopes.
- The `ScopeError` type exists in `oauth/manager.go` but is not surfaced to the frontend.

This is a separate, larger feature. This ticket only removes the broken generic connect path. A follow-up ticket should implement contextual OAuth prompts during agent sessions.

## Key Files

| File | Change |
|------|--------|
| `frontend/src/components/account/OAuthConnections.tsx` | Remove "Available Integrations" / connect flow; keep list + disconnect + refresh |

No backend changes needed.

## Notes for Implementors

- The "Connected Services" list (rendering existing connections) is a separate section from the "Available Integrations" connect UI — they can be separated cleanly.
- The `connectProvider` / `startOAuthFlow` / `openConnectDialog` functions can be deleted entirely from this component once the connect UI is gone.
- Check if `OAuthConnectionsPage.tsx` (the page wrapper) references anything that also needs cleanup.
- The OAuth popup/callback mechanism used by the contextual dialogs is unaffected.
