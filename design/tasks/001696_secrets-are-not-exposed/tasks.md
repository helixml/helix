# Implementation Tasks

- [ ] Find where human/exploratory desktop environment is constructed (different from spec task path)
- [ ] Add call to `DesktopAgentAPIEnvVars(apiKey)` in human desktop startup
- [ ] Ensure user's API token is available at that code location
- [ ] Test human desktop: verify `env | grep API` shows expected keys
- [ ] Test AI agent functionality works in human desktop (Claude Code, MCP tools)
