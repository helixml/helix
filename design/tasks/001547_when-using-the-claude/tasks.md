# Implementation Tasks

- [x] Replace `generateAgentServerConfig` in `main.go`: instead of returning `agent_servers.claude` config, write `/etc/claude-code/managed-settings.json` with `env` (including `ANTHROPIC_BASE_URL`) and `permissions.defaultMode`
- [x] Stop writing `agent_servers` key to Zed settings for the `claude_code` runtime — let Zed use its built-in Claude agent defaults
- [x] Keep existing credentials sync (`~/.claude/.credentials.json`) and the `os.Stat` gate unchanged
- [ ] Verify bypass-permissions toggle appears in Zed Claude agent panel (confirms `IS_SANDBOX=1` is working)
- [ ] Verify model selector appears and lists models (confirms auth is working via managed settings env)
