# Requirements: Claude Agent UX Parity in Zed

## Background

When a Helix session uses the `claude_code` runtime, the settings-sync-daemon writes `agent_servers.claude` to Zed's `settings.json`. The model selector and bypass-permissions toggle that appear when using the Claude agent normally in Zed do not appear in Helix sessions.

After tracing the Zed source code:

- `ExternalAgentSource` is **always `Builtin`** for Claude regardless of what's in `agent_servers.claude` — so this is not the gate on the UI.
- The model selector and mode selector (including bypass permissions) in `thread_view.rs` are only rendered when the ACP server's session response contains `modes`/`models`. If the server returns `config_options` instead, those selectors are suppressed and a different config options UI is shown. If the session fails to establish, nothing appears.
- The mechanism suppressing the UI in Helix sessions needs to be confirmed during implementation. One likely candidate: Zed explicitly sets `ANTHROPIC_API_KEY=""` before applying `settings_env`. An empty-string API key may behave differently from an unset one in the Claude Agent SDK, potentially causing session init to fail or the agent to not report modes and models.

## User Stories

**US-1:** As a Helix user, I want to see the bypass-permissions mode toggle in the Zed Claude agent panel.

**US-2:** As a Helix user, I want to see the model selector in the Zed Claude agent panel.

**US-3:** As a Helix user, model and mode preferences I set in the UI should not be overwritten by the daemon's next 30-second sync cycle.

## Acceptance Criteria

- **AC-1:** Bypass-permissions mode toggle appears in the Zed Claude agent panel for Helix-managed sessions.
- **AC-2:** Model selector appears and lists available Claude models.
- **AC-3:** The daemon does not overwrite user UI changes on each sync cycle.
