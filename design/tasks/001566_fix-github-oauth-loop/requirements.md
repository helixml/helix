# Requirements: Fix GitHub OAuth — Contextual Connect Only

## Problem

The "Connected Services" page lets users initiate an OAuth connection to any provider without knowing what scopes are needed. Whatever scope choice is made there is a guess. The required scopes are only known by the feature initiating the connection (a specific agent, a repo browser, etc.).

Additionally, when a user starts a session with an agent whose tools declare OAuth requirements, the system does nothing proactive — it attempts to get a token at tool-execution time, fails silently, and the user sees a generic error.

## Desired Behaviour

### 1. Remove Connect buttons from the Connected Services page

The Available Integrations section stays, so users can see which providers exist, but the Connect buttons are removed. In their place, show an info banner explaining: *"Use an integration in an agent when creating it and specify the scopes in order to connect to it as a user."*

Users can still view, disconnect, and refresh existing connections. New connections are always initiated at the point of actual use (agent session, project dialog, etc.) where the required scopes are known.

### 2. OAuth prompt when loading any agent session

Whenever a user loads an agent session — starting a new chat, starting a new spec task, or returning to an existing one — the frontend checks whether the required OAuth connections are present and valid. A token may have expired or been revoked since the session was last active. If any required connection is missing or has expired, the user is shown a prompt listing the affected providers with a "Connect" / "Reconnect" button for each.

This must be generic — driven entirely by what the agent spec declares, not hardcoded to any specific provider or scope set.

## Acceptance Criteria

- [ ] The Connected Services page still shows the Available Integrations section but without Connect buttons. An info banner reads: "Use an integration in an agent when creating it and specify the scopes in order to connect to it as a user." Existing connections are still shown with disconnect and refresh options.
- [ ] When loading any agent session — new chat, new spec task, or returning to an existing one — if required OAuth connections are missing or have expired, the user sees a prompt naming each affected provider with a Connect / Reconnect button.
- [ ] The OAuth flow triggered from that prompt uses the exact provider and scopes declared in the agent's tool config, not hardcoded values.
- [ ] If all required connections are present and valid, no prompt is shown and the session loads normally.
- [ ] The existing contextual OAuth flows in `CreateProjectDialog` and `BrowseProvidersDialog` are unaffected.
