# Requirements: Claude Agent UX Parity in Zed

## Background

Helix's settings-sync-daemon writes `agent_servers.claude` with env overrides (`ANTHROPIC_BASE_URL`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, `DISABLE_TELEMETRY=1`, `default_mode: bypassPermissions`). These override what Zed's built-in Claude Code ACP package would otherwise do, and they suppress the model selector and bypass-permissions toggle — the Claude Code ACP server only reports `modes`, `models`, and `config_options` (the things Zed renders UI for) when running in a normal/unmodified configuration. Zed's native "Claude Agent" works because it uses those defaults.

## User Stories

**US-1:** As a Helix user with a Claude subscription session, I want the Zed Claude agent panel to show the model selector and bypass-permissions toggle, exactly as it does in a non-Helix Zed install.

**US-2:** As a Helix user, I do not want the daemon to clobber Claude agent settings I've changed in the UI (e.g., model choice, permissions mode).

## Acceptance Criteria

- **AC-1:** The Zed Claude agent panel shows the model selector and bypass-permissions toggle for Helix-managed Claude subscription sessions.
- **AC-2:** The daemon does not write `agent_servers.claude` at all for subscription-mode sessions; Zed uses its built-in Claude Code defaults.
- **AC-3:** Claude Code authenticates successfully using OAuth credentials from `~/.claude/.credentials.json` (already written by the daemon).
- **AC-4:** Claude Code does not accidentally hit the Helix API proxy (which Hydra injects as `ANTHROPIC_BASE_URL` in all containers); it uses `https://api.anthropic.com` directly.
