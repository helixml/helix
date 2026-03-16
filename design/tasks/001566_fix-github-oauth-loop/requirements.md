# Requirements: Fix GitHub OAuth — Remove Generic Connect from Connected Services

## Problem

The "Connected Services" page (`OAuthConnections.tsx`) lets users initiate an OAuth connection to any provider (e.g. GitHub) without knowing what scopes are needed. This is the wrong place to connect: the required scopes depend entirely on what the user is trying to do (which agent, which repo feature), so any scope choice here would be a guess or a hack.

The correct pattern — already used by `CreateProjectDialog` and `BrowseProvidersDialog` — is to trigger the OAuth flow at the point of actual use, where the required scopes are known.

Additionally, when an agent uses a skill that requires OAuth and the user doesn't have a valid connection, there is currently **no pre-session or in-session prompt** to initiate the OAuth flow — tool execution silently fails with a generic error.

## Desired Behaviour

### 1. Remove generic "Connect" from Connected Services page

The Connected Services page should be **read-only for connections initiated elsewhere**. Users can:
- View their existing OAuth connections (provider, profile, date connected)
- Disconnect (delete) an existing connection
- Refresh an expired token

Users **cannot** initiate a new OAuth connection from this page. The "Connect" button / available integrations section is removed or disabled.

### 2. (Out of scope for this ticket — document as a gap) Pre-session OAuth prompt for agents

When a user starts a session with an agent whose skills declare OAuth requirements, and the user has no valid connection with the required scopes, the system currently does nothing proactive — the tool call fails silently at runtime.

A future ticket should implement a pre-session (or in-session) OAuth prompt: detect that an agent needs GitHub with `repo` scope, check the user's connections, and if missing, surface a "Connect GitHub" action with the correct scopes before the session runs.

## Acceptance Criteria

- [ ] The Connected Services page no longer shows a "Connect" button or "Available Integrations" section that lets users initiate OAuth flows.
- [ ] Existing connections are still displayed with disconnect and refresh options.
- [ ] Users connecting GitHub for a specific purpose (repo browsing, project creation, agent skill) are prompted via the existing contextual dialogs (`BrowseProvidersDialog`, `CreateProjectDialog`, etc.) which already pass the correct scopes.
- [ ] No regression in the project/agent OAuth flows that already work correctly.
