# Requirements: Local MCPs appear doubled after enabling

## Problem

When a user enables a local MCP skill (e.g., Drone CI or GitHub), two copies of the skill tile appear in the Skills UI, both showing as enabled.

## Root Cause

`Skills.tsx` builds its skill list from two sources that overlap:

1. **`BASE_SKILLS`** — hardcoded entries for "Drone CI" and "GitHub" with `SKILL_CATEGORY_LOCAL_MCP`
2. **`localMcpSkills`** — dynamically generated from `app.mcpTools` filtering for `transport === 'stdio' || mcp.command`

Both are merged into `allSkills`:
```
const skills = [...baseSkillsFiltered, ...customApiSkills, ...backendSkills, ...mcpSkills, ...];
skills.push(...localMcpSkills, CUSTOM_LOCAL_MCP_SKILL);
```

When a user enables "Drone CI", it adds an entry to `app.mcpTools` with `transport: 'stdio'`. This causes `localMcpSkills` to generate a *second* dynamic tile for the same MCP, alongside the original hardcoded one. Both show as enabled because `isSkillEnabled` checks `app.mcpTools` by name.

## User Stories

1. As a user, when I enable a local MCP skill (Drone CI, GitHub), I should see exactly one tile for that skill, shown as enabled.
2. As a user, when I disable a local MCP skill, I should see exactly one tile for that skill, shown as disabled.
3. As a user, when I add a custom local MCP (not one of the hardcoded ones), it should appear as a single tile.

## Acceptance Criteria

- [ ] Enabling Drone CI shows one "Drone CI" tile (enabled), not two
- [ ] Enabling GitHub shows one "GitHub" tile (enabled), not two
- [ ] Disabling either returns to one tile (disabled)
- [ ] Custom local MCPs (added via "New Local MCP") still appear correctly as a single tile
- [ ] The Local MCP category badge count is correct (no double-counting)
- [ ] No regression to HTTP/SSE MCP skills or other skill categories