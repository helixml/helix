# Implementation Tasks: Rename Skills → MCPs & APIs and Add Session-Restart Notice

## Backend: Go rename

- [ ] Rename Go package `api/pkg/agent/skill/` → `api/pkg/agent/mcpandapi/`. Update all internal imports.
- [ ] Inside the new package, rename files where the `skill` part is redundant (e.g., `mcp/mcp_skill.go` → `mcp/mcp.go`, `api_calling_skill.go` → `api_calling.go`).
- [ ] Rename types in `api/pkg/types/skill.go` (now `mcpandapi.go`): `SkillDefinition` → `MCPAndAPIDefinition`, `SkillMetadata` → `MCPAndAPIMetadata`, `SkillSpec` → `MCPAndAPISpec`, `SkillsListResponse` → `MCPAndAPIListResponse`, etc.
- [ ] Rename the HTTP handler file `api/pkg/server/skills.go` → `mcps_and_apis.go` and its functions: `handleListSkills` → `handleListMCPsAndAPIs`, etc.

## Backend: API contract

- [ ] Add new endpoint `/api/v1/mcps-and-apis` (list / validate / reload) wired to the same handlers.
- [ ] Keep `/api/v1/skills` as an alias for one release; mark `@deprecated` in swagger annotations with a note pointing at the new path.
- [ ] Log a deprecation warning **once on process startup** when the alias is registered (avoid per-request log spam).
- [ ] In `MCPAndAPIListResponse`, include both `mcpsAndApis` and `skills` JSON keys for one release (same underlying slice).
- [ ] Regenerate swagger: `swag init` (or whatever this project uses) and check in the updated `api/pkg/server/swagger.json` and `swagger/docs.go`.

## Frontend: regenerated client

- [ ] Run `./stack update_openapi` to regenerate `frontend/src/api/api.ts` from the new swagger.
- [ ] Confirm `TypesMCPAndAPIDefinition` etc. now exist; remove or replace consumers of the old `TypesSkillDefinition`.

## Frontend: component / file renames

- [ ] Rename `frontend/src/components/app/Skills.tsx` → `MCPsAndAPIs.tsx` and the exported component name.
- [ ] Rename `AddApiSkillDialog.tsx` → `AddAPIDialog.tsx`, `AddMcpSkillDialog.tsx` → `AddMCPDialog.tsx`, `AddLocalMcpSkillDialog.tsx` → `AddLocalMCPDialog.tsx`, `SkillExecutionDialog.tsx` → `MCPAndAPIExecutionDialog.tsx`.
- [ ] Rename individual skill files where `skill` is redundant: `skillGmailApi.tsx` → `gmailAPI.tsx`, `skillGoogleDriveApi.tsx` → `googleDriveAPI.tsx`, etc. (~12 files).
- [ ] Update all imports of renamed components.

## Frontend: identifier renames

- [ ] Rename constants in the renamed `MCPsAndAPIs.tsx`: `SKILL_TYPE_*` → `MCP_AND_API_TYPE_*`, `SKILL_CATEGORY_*` → `MCP_AND_API_CATEGORY_*` (or shortened forms where unambiguous).
- [ ] Rename variables: `apiSkill` → `apiEntry`, `mcpSkill` → `mcpEntry`, `skillManager` → `mcpAndAPIManager`, `handleSkillsUpdate` → `handleMCPsAndAPIsUpdate`, etc.
- [ ] Rename hook `useSkills` → `useMCPsAndAPIs` and its file.
- [ ] Rename `IAgentSkill` in `frontend/src/types.ts` → `IAgentMCPAndAPI`.

## Frontend: UI text

- [ ] Replace user-visible "Skill" / "Skills" strings with "MCP & API" / "MCPs & APIs" across tab labels, headings, dialog titles, button text, sidebar entries (`AppSidebar.tsx`), empty-state copy, and tooltips.
- [ ] Update tab labels in `ProjectSettings.tsx` and `App.tsx` (the two tab-host pages).

## Frontend: routing

- [ ] Change the slug `?tab=skills` to `?tab=mcps-and-apis` everywhere it's written (links, internal navigation calls).
- [ ] Add a client-side redirect: if the parsed `tab` query param is `skills`, rewrite the URL to `mcps-and-apis` via `router.replace()` and continue rendering the tab. Keep this redirect for one release.

## Session-restart notice

- [ ] In the renamed `MCPsAndAPIs.tsx`, near the top of the rendered output (above the category/grid layout, near current `Skills.tsx` L1250), add `<Alert severity="info" sx={{ mb: 2 }}>` with wording: *"Changes to MCPs and APIs take effect in new sessions. Restart any active session to pick up updates."*
- [ ] Confirm `Alert` is imported from `@mui/material` (add if missing).
- [ ] Verify the notice always renders (no `Collapse`, no dismiss action).

## Docs

- [ ] Update `/helix/README.md` mentions of "skills" (5 occurrences) to "MCPs & APIs".
- [ ] If anything in `/home/retro/work/docs/` ends up referencing the old name, update it too.

## Verification

- [ ] Start the frontend dev server. Open **Project Settings**; confirm the tab is labelled "MCPs & APIs" and the info notice shows above the editor body.
- [ ] Open an **Agent (App)**; confirm the same tab name and notice.
- [ ] Visit a URL with the old slug `?tab=skills`; confirm the URL auto-rewrites to `?tab=mcps-and-apis` and the right tab renders.
- [ ] `curl /api/v1/skills` — confirm it still returns data (alias works) and that the response includes both `skills` and `mcpsAndApis` fields.
- [ ] `curl /api/v1/mcps-and-apis` — confirm it returns the same data.
- [ ] Confirm the swagger UI shows `/api/v1/skills` marked deprecated.
- [ ] Run the Go test suite and the frontend type-check; fix any remaining references.
- [ ] `grep -ri "skill" .` across the repo and triage remaining hits: expected leftovers are Anthropic-Skills references, the deprecation alias, code comments documenting the rename, and any genuine "skill" English word in comments/copy.
- [ ] Take screenshots of the renamed tab in both Project Settings and Agent Settings; attach to the PR.

## Ship

- [ ] Split the diff into reviewable commits by layer (Go rename / API alias / generated client / frontend component renames / frontend UI text / routing redirect / notice / docs).
- [ ] Open PR with a clear note about the API deprecation window so reviewers know to plan its removal in the next major.
