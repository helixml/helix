# Requirements: Claude Agent UX Parity in Zed

## Background

Helix's settings-sync-daemon writes `agent_servers.claude` with env overrides and `default_mode: bypassPermissions`. Two things are suppressing the model selector and bypass-permissions toggle in Zed's Claude agent panel:

1. **Bypass permissions is hidden when running as root.** The `claude-agent-acp` package only advertises the `bypassPermissions` session mode when `!IS_ROOT || !!process.env.IS_SANDBOX`. Helix containers can run as root.

2. **The daemon's env injection approach (`agent_servers.claude.env`) may interfere.** The cleaner mechanism is the managed settings file that `claude-agent-acp` already reads natively at startup: `/etc/claude-code/managed-settings.json`.

The `claude-agent-acp` package (github.com/zed-industries/claude-agent-acp) is an ACP adapter built on the **Claude Agent SDK** — not the Claude Code CLI. It dynamically reports available models from the SDK and advertises session modes (including bypass permissions) based on runtime conditions.

## User Stories

**US-1:** As a Helix user, I want to see the bypass-permissions mode toggle in the Zed Claude agent panel regardless of whether my container runs as root.

**US-2:** As a Helix user, I want to see the model selector in the Zed Claude agent panel so I can choose which Claude model to use.

**US-3:** As a Helix user, model/mode preferences I set in the UI should not be clobbered by the next daemon sync cycle.

## Acceptance Criteria

- **AC-1:** The bypass-permissions mode appears in the Zed Claude agent panel for Helix-managed sessions (root or not).
- **AC-2:** The model selector appears and lists available Claude models.
- **AC-3:** The daemon does not write `agent_servers.claude.env` overrides that fight with user settings changes.
