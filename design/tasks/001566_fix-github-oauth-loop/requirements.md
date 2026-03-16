# Requirements: Fix GitHub OAuth — Contextual Connect Only

## Problem

The "Connected Services" page lets users initiate an OAuth connection to any provider without knowing what scopes are needed. Whatever scope choice is made there is a guess. The required scopes are only known by the feature initiating the connection (a specific agent, a repo browser, etc.).

Additionally, when a user starts a session with an agent whose tools declare OAuth requirements, the system does nothing proactive — it attempts to get a token at tool-execution time, fails silently, and the user sees a generic error.

## Desired Behaviour

### 1. Remove Connect buttons from the Connected Services page

The Available Integrations section stays, so users can see which providers exist, but the Connect buttons are removed. In their place, show an info banner explaining: *"Use an integration in an agent when creating it and specify the scopes in order to connect to it as a user."*

Users can still view, disconnect, and refresh existing connections. New connections are always initiated at the point of actual use (agent session, project dialog, etc.) where the required scopes are known.

### 2. Pre-session OAuth prompt for agents

Before a user starts a session with an agent — whether that is a new chat session or a new spec task — the frontend reads the agent's tool configurations and determines which OAuth providers and scopes are required. It then checks the user's existing connections against those requirements. If any required connection is missing or lacks the necessary scopes, the user is shown a prompt listing the missing providers with a "Connect" button for each. Each connect action initiates the OAuth flow with exactly the scopes that agent's tools require.

This must be generic — driven entirely by what the agent spec declares, not hardcoded to any specific provider or scope set.

## Acceptance Criteria

- [ ] The Connected Services page still shows the Available Integrations section but without Connect buttons. An info banner reads: "Use an integration in an agent when creating it and specify the scopes in order to connect to it as a user." Existing connections are still shown with disconnect and refresh options.
- [ ] When starting any agent session (new chat or new spec task), if the agent's tools require OAuth connections the user does not have (or has with insufficient scopes), the user sees a prompt naming each missing provider with a Connect button before the session starts.
- [ ] The OAuth flow triggered from that prompt uses the exact provider and scopes declared in the agent's tool config, not hardcoded values.
- [ ] If all required connections are already present with correct scopes, no prompt is shown and the session starts normally.
- [ ] The existing contextual OAuth flows in `CreateProjectDialog` and `BrowseProvidersDialog` are unaffected.
