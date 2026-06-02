# Rename Skills tab to "MCPs & APIs" and add session-restart notice

## Summary

Two user-visible UX fixes for the helix "Skills" editor:

1. **Renamed "Skills" → "MCPs & APIs"** in the user-facing UI. Helix's
   "Skills" predates Anthropic's Skills feature and the name collision was
   causing users to overlook the tab where they configure MCP servers and
   API integrations. Putting "MCP" in the label makes it discoverable.

2. **Added a persistent info notice** at the top of the editor (both in
   Project Settings and Agent Settings) telling users that already-running
   sessions need to be restarted to pick up MCP / API configuration
   changes.

This is **Phase A** of the broader rename. The internal `?tab=skills`
URL slug, component/type/variable names, the Go package
`api/pkg/agent/skill/`, and the `/api/v1/skills` REST endpoint are
intentionally left unchanged in this PR. Those follow in Phase B (frontend
internal rename) and Phase C (backend rename + API alias).

## Changes

- `frontend/src/components/app/AppSidebar.tsx` — agent sidebar tab label.
- `frontend/src/components/project/ProjectSettingsSidebar.tsx` — project
  settings sidebar tab label.
- `frontend/src/pages/ProjectSettings.tsx` — tab heading and description.
- `frontend/src/components/apps/AppsTable.tsx` — table column header.
- `frontend/src/components/app/Skills.tsx`:
  - Heading "💡 Skills" → "💡 MCPs & APIs", plus paragraph wording.
  - Search placeholder.
  - "Add MCP Skill" button → "Add MCP".
  - "Can become a skill" → "can become a tool" in the New API tile copy.
  - Added a permanent `<Alert severity="info">` above the editor body.

## Screenshots

![Project Settings — MCPs & APIs tab with notice](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002066_the-skills-mcp-and-apis/screenshots/01-project-settings-mcps-apis-tab.png)

![Agents table — column renamed](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002066_the-skills-mcp-and-apis/screenshots/02-agents-table-column.png)

![Agent Settings — MCPs & APIs tab with notice](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002066_the-skills-mcp-and-apis/screenshots/03-agent-settings-mcps-apis-tab.png)

## Test plan

- [x] `yarn build` in `frontend/` succeeds (no new TS errors).
- [x] Browser check: Project Settings → MCPs & APIs tab renders the new
      heading and notice.
- [x] Browser check: Agent Settings → MCPs & APIs tab renders the new
      heading and notice.
- [x] Browser check: agents listing table shows "MCPs & APIs" column.
- [x] No new browser console errors.
- [ ] Reviewer to confirm wording is acceptable before Phase B/C kick off.

## Deferred (out of this PR)

See `design/tasks/002066_the-skills-mcp-and-apis/tasks.md` in the
`helix-specs` branch for the full plan. Short version:

- **Phase B** — Rename frontend internals: `Skills.tsx` → `MCPsAndAPIs.tsx`,
  constants `SKILL_TYPE_*`, hook `useSkills`, type `IAgentSkill`, ~12
  individual `skill*Api.tsx` files, dialog component renames, the URL slug
  `?tab=skills` → `?tab=mcps-and-apis` with redirect.
- **Phase C** — Rename Go package `api/pkg/agent/skill/`, types, handlers,
  and add `/api/v1/mcps-and-apis` endpoint with `/api/v1/skills` kept as a
  deprecated alias for one release.
