# Requirements: Claude Agent UX Parity in Zed

## Background

Helix's settings-sync-daemon writes `agent_servers.claude` with env overrides (`ANTHROPIC_BASE_URL`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`, `DISABLE_TELEMETRY`) and `default_mode: bypassPermissions` into Zed's `settings.json`. The result is that the model selector and bypass-permissions toggle do not appear in Zed's Claude agent panel for Helix-managed sessions, even though they appear in a standard Zed install.

The `claude-agent-acp` package (github.com/zed-industries/claude-agent-acp) is an ACP adapter for the **Claude Agent SDK** — not the Claude Code CLI. It advertises available models and session modes (including bypass permissions) dynamically. The exact reason these are suppressed in the Helix configuration needs to be confirmed during implementation, but the most likely culprits are the env var overrides interfering with SDK initialisation or model fetching.

The package also supports a dedicated managed settings file (`/etc/claude-code/managed-settings.json` on Linux) which is a cleaner injection point than `agent_servers.claude.env` in Zed settings.

## User Stories

**US-1:** As a Helix user, I want to see the bypass-permissions mode toggle in the Zed Claude agent panel.

**US-2:** As a Helix user, I want to see the model selector in the Zed Claude agent panel.

**US-3:** As a Helix user, model and mode preferences I set in the UI should not be overwritten by the daemon's next 30-second sync cycle.

## Acceptance Criteria

- **AC-1:** Bypass-permissions mode toggle appears in the Zed Claude agent panel for Helix-managed sessions.
- **AC-2:** Model selector appears and lists available Claude models.
- **AC-3:** The daemon does not overwrite user UI changes to model or mode on each sync cycle.
