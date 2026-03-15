# Implementation Tasks

- [ ] In `generateAgentServerConfig` (settings-sync-daemon), add `favorite_models` to the `claude` config block for both subscription and API key modes
- [ ] In `generateAgentServerConfig`, add `default_model` to the `claude` config block (use the configured model in API key mode; use a sensible default like `claude-sonnet-4-6` in subscription mode)
- [ ] Add `agent_servers.claude.default_mode` to the daemon's user-owned / preserved fields so subsequent sync cycles do not overwrite the user's mode selection
- [ ] On first-write (no existing user preference for `default_mode`), still default to `bypassPermissions` for subscription mode
- [ ] Verify in Zed that the model selector and bypass-permissions toggle appear and are functional after the settings change
