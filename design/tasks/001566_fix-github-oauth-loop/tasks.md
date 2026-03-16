# Implementation Tasks

## Part 1: Remove Connect from Connected Services page

- [ ] In `frontend/src/components/account/OAuthConnections.tsx`, remove the Connect button from each provider card in the "Available Integrations" section; keep the section itself so users can see available providers
- [ ] Add an info banner to the Available Integrations section with the text: "Use an integration in an agent when creating it and specify the scopes in order to connect to it as a user."
- [ ] Delete the `openConnectDialog`, `startOAuthFlow`, and `connectProvider` functions and any state they own (now unreferenced)
- [ ] Verify the Connected Services list (existing connections with disconnect/refresh) still renders correctly

## Part 2: Backend — OAuth requirements endpoint

- [ ] Add handler `GET /api/v1/apps/{id}/oauth-requirements` in `api/pkg/server/app_handlers.go` that iterates the app's `assistants[].apis[]` and `assistants[].mcp_servers[]`, collects unique `{provider_name, scopes}` pairs, and returns them as a JSON array
- [ ] Register the new route in the server router (same auth middleware as `getApp`)

## Part 3: Frontend — Pre-session OAuth prompt

- [ ] Identify both session entry points where an agent is loaded: new chat sessions and new spec tasks; the OAuth check must run in both
- [ ] In each entry point, on load (when an agent app is set), call `GET /api/v1/apps/{id}/oauth-requirements` and `GET /api/v1/oauth/connections`
- [ ] Compare requirements against existing connections: for each required `{provider, scopes}`, check whether the user has a connection for that provider whose stored scopes cover all required scopes
- [ ] If any requirements are unmet, render a prompt (banner or dialog) listing each missing provider with a Connect button; block or discourage starting the session until all are satisfied
- [ ] Each Connect button triggers the OAuth flow via `GET /api/v1/oauth/flow/start/{provider_id}?scopes=<scopes>` using the exact scopes from the requirement (reuse `useOAuthFlow` hook or the popup pattern from `BrowseProvidersDialog`)
- [ ] On successful OAuth callback, re-check requirements and dismiss the prompt if all are now met

## Verification

- [ ] Open an agent that has a GitHub API tool with `repo` scope — the pre-session prompt should appear if not connected, and disappear after connecting
- [ ] Open an agent with no OAuth tools — no prompt shown
- [ ] Account > Connected Services shows existing connections only, no Connect button
