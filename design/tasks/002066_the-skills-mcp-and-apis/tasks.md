# Implementation Tasks: Rename Skills → MCPs & APIs and Add Session-Restart Notice

## Phasing decision (made during implementation)

The full rename across ~2,950 occurrences (frontend + backend + generated swagger) is realistically several hours and high risk of leaving the build broken. Splitting per the design's phasing fallback:

- **Phase A (this PR):** Frontend UI text rename + session-restart notice + URL slug compat. Delivers the user-visible discoverability win cleanly.
- **Phase B (follow-up):** Frontend code identifiers + component file renames.
- **Phase C (follow-up):** Backend Go package rename + API alias + swagger regeneration.

After Phase A is shipped, we'll check in with the user before starting Phase B/C.

---

## Phase A — User-visible rename + notice

### Frontend: UI text

- [x] Find all user-visible occurrences of "Skill" / "Skills" in the frontend.
- [x] Update sidebar label in `AppSidebar.tsx` ("Skills" → "MCPs & APIs").
- [x] Update sidebar label in `ProjectSettingsSidebar.tsx`.
- [x] Update project-settings tab heading + description in `ProjectSettings.tsx`.
- [x] Update column header "Skills" → "MCPs & APIs" in `AppsTable.tsx`.
- [x] Update agent-settings heading inside `Skills.tsx` ("💡 Skills" → "💡 MCPs & APIs") and the description paragraph below it.
- [x] Update search placeholder ("Search skills..." → "Search MCPs and APIs...").
- [x] Update "Add MCP Skill" button → "Add MCP".

Deferred to Phase B (deeper internal-ish strings):
- [ ] "Disable Skill" / "this skill" copy in the disable-confirmation dialog inside `Skills.tsx`.
- [ ] "OAuth Provider Required" dialog copy ("This skill requires…").
- [ ] `SkillConfigurationStep.tsx` wizard strings ("Configure: …", "this skill", "this skill requires …").
- [ ] `LLMCallTimelineChart.tsx` "Skill execution:" label.
- [ ] `AddApiSkillDialog.tsx` "Skill System Prompt" field label.
- [ ] `EvaluationTab.tsx` "Skill Used" / "Skill name" labels (these may genuinely refer to the concept; review semantics in Phase B).

### Session-restart notice

- [x] In `frontend/src/components/app/Skills.tsx`, add an `<Alert severity="info">` at the top of the rendered output with wording: *"Changes to MCPs and APIs take effect in new sessions. Restart any active session to pick up updates."*
- [x] `Alert` was already imported from `@mui/material`.

### Frontend: routing compat

URL slug `?tab=skills` is **not** changed in Phase A — it's purely internal and changing it requires touching a typed union plus all read/write sites. Users won't notice. Deferred to Phase B.

### Verification (Phase A)

- [ ] Type-check / lint the frontend (`pnpm tsc` or equivalent) — no new errors.
- [ ] Start the frontend dev server. Open **Project Settings**; confirm the tab now reads "MCPs & APIs" and the info notice shows above the editor body.
- [ ] Open an **Agent (App)**; confirm the same tab name and notice.
- [ ] Visit a URL with the old slug `?tab=skills`; confirm the URL auto-rewrites to `?tab=mcps-and-apis`.
- [ ] Take before/after screenshots and commit them under `design/tasks/002066_the-skills-mcp-and-apis/screenshots/`.

### Ship Phase A

- [ ] Write PR descriptions (`pull_request_helix.md`, `pull_request_helix-next.md` if applicable).
- [ ] Merge origin/main into the feature branch.
- [ ] Push `feature/002066-rename-skills-mcps-apis`.
- [ ] Tell user: Phase A shipped; ask whether to proceed with B/C in this session or as a follow-up.

---

## Phase B (deferred) — Frontend internal rename

- [ ] Rename `frontend/src/components/app/Skills.tsx` → `MCPsAndAPIs.tsx` and exported component.
- [ ] Rename `AddApiSkillDialog.tsx`, `AddMcpSkillDialog.tsx`, `AddLocalMcpSkillDialog.tsx`, `SkillExecutionDialog.tsx`, ~12 `skill*Api.tsx` files.
- [ ] Rename constants: `SKILL_TYPE_*`, `SKILL_CATEGORY_*`.
- [ ] Rename variables: `apiSkill`, `mcpSkill`, `skillManager`, `handleSkillsUpdate`.
- [ ] Rename hook `useSkills` → `useMCPsAndAPIs`.
- [ ] Rename `IAgentSkill` type.

## Phase C (deferred) — Backend rename + API alias

- [ ] Rename Go package `api/pkg/agent/skill/` → `api/pkg/agent/mcpandapi/`.
- [ ] Rename types in `api/pkg/types/skill.go`.
- [ ] Rename HTTP handlers in `api/pkg/server/skills.go`.
- [ ] Add `/api/v1/mcps-and-apis` endpoint; keep `/api/v1/skills` as deprecated alias for one release.
- [ ] Dual JSON field names (`skills` + `mcpsAndApis`) in response.
- [ ] Regenerate swagger; regenerate frontend client.
- [ ] Update README.md (5 mentions).
