# Implementation Tasks

- [~] Fix field name in `api/cmd/settings-sync-daemon/main.go`: change `"default"` to `"default_mode"` on line ~189 in `generateAgentServerConfig()`
- [ ] Rebuild and deploy: `./stack build-ubuntu` then start a new session
- [ ] Test: Start new Claude Code session, ask it to create a file, verify no permission prompt appears
- [ ] Verify settings: Check `~/.config/zed/settings.json` inside container contains `"default_mode": "bypassPermissions"` under `agent_servers.claude`
