# Implementation Tasks

- [ ] In `generateAgentServerConfig` subscription branch (`main.go`), return `nil` instead of the env block — stop writing `agent_servers.claude` for subscription-mode sessions
- [ ] Fix `ANTHROPIC_BASE_URL` leakage: either (a) have Hydra not set it in subscription-mode containers, or (b) write `ANTHROPIC_BASE_URL=https://api.anthropic.com` to `~/.claude/settings.json` instead of via Zed settings
- [ ] Keep the existing `os.Stat(ClaudeCredentialsPath)` gate and marker file logic — credentials sync and startup sequencing are still needed
- [ ] Verify that after the change, the Zed Claude agent panel shows the model selector and bypass-permissions toggle in a subscription-mode session
