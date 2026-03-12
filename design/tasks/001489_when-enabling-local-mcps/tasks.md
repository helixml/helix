# Implementation Tasks

- [ ] In `Skills.tsx` `localMcpSkills` useMemo (~line 658), add a `baseSkillNames` set built from `BASE_SKILLS.map(s => s.name)` and filter out any `app.mcpTools` entries whose name matches a base skill
- [ ] Verify fix: enable Drone CI → confirm only one tile appears (enabled)
- [ ] Verify fix: enable GitHub → confirm only one tile appears (enabled)
- [ ] Verify fix: disable Drone CI / GitHub → confirm one tile (disabled)
- [ ] Verify fix: add a custom local MCP via "New Local MCP" → confirm it still appears as a single tile
- [ ] Verify fix: check Local MCP category badge count is correct after enabling/disabling
- [ ] Run `cd frontend && yarn build` to confirm no build errors