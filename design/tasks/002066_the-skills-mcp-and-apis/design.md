# Design: Rename Skills → MCPs & APIs and Add Session-Restart Notice

## TL;DR

Two changes shipped together:

1. **Rename** every user-facing and internal occurrence of "Skills" (the helix concept) to **"MCPs & APIs"**, including the public REST endpoint, the URL tab slug, frontend types, Go package paths, and all related identifiers.
2. **Add** a persistent `<Alert severity="info">` to the MCPs & APIs editor (both in Project Settings and Agent Settings) telling the user that running sessions need restarting to pick up changes.

Both changes are mostly mechanical. The rename is the bulk of the work.

## Naming decision

**User-visible label:** **"MCPs & APIs"** (display) — e.g. tab reads "MCPs & APIs", section header reads "Add MCP or API", etc.

**Code identifiers:**

| Context | Old | New |
| --- | --- | --- |
| camelCase var | `skill`, `skills`, `apiSkill`, `mcpSkill` | `mcpAndApi`, `mcpsAndApis`, `apiEntry`, `mcpEntry` |
| PascalCase type | `Skill`, `SkillDefinition`, `IAgentSkill`, `TypesSkillDefinition` | `MCPAndAPI`, `MCPAndAPIDefinition`, `IAgentMCPAndAPI`, `TypesMCPAndAPIDefinition` |
| SCREAMING_SNAKE constant | `SKILL_TYPE_MCP`, `SKILL_CATEGORY_CORE` | `MCP_AND_API_TYPE_MCP`, `MCP_AND_API_CATEGORY_CORE` (or shortened where unambiguous, e.g. just `CATEGORY_CORE`) |
| Go package | `api/pkg/agent/skill/` | `api/pkg/agent/mcpandapi/` |
| Go type | `types.SkillDefinition`, `types.SkillsListResponse` | `types.MCPAndAPIDefinition`, `types.MCPAndAPIListResponse` |
| File name | `Skills.tsx`, `AddMcpSkillDialog.tsx`, `skillGmailApi.tsx` | `MCPsAndAPIs.tsx`, `AddMCPDialog.tsx`, `gmailAPI.tsx` (where the "skill" prefix was redundant) |
| URL slug | `?tab=skills` | `?tab=mcps-and-apis` |
| REST path | `/api/v1/skills` | `/api/v1/mcps-and-apis` |
| Response field | `"skills": [...]` | `"mcpsAndApis": [...]` |

Where existing names already say "API" or "MCP" specifically (e.g. `AddApiSkillDialog`, `mcp_skill.go`), drop the redundant `Skill`/`skill` suffix rather than expanding it to `MCPAndAPI`. `AddApiSkillDialog` → `AddAPIDialog`. `mcp_skill.go` → `mcp.go` (inside the new package). This avoids meaningless verbosity like `AddApiMCPAndAPIDialog`.

## Surface area (from code survey)

| Area | Approx scale | Notes |
| --- | --- | --- |
| Frontend UI text (labels, headings) | ~200 occurrences, ~40 files | `Skills.tsx` is the big one (~1900 lines). |
| Frontend identifiers | ~447 occurrences, ~40 files | Many are generated from swagger and regenerated automatically. |
| Backend Go code | ~816 occurrences, ~78 files | Whole package `api/pkg/agent/skill/` and `server/skills.go` handler. |
| Public REST API | 3 endpoints + 1 response shape | `/api/v1/skills` (list, validate, reload). Renamed with one-release alias. |
| URL slug | 1 slug | `?tab=skills` (with redirect for one release). |
| DB schema | **None** | Skills are runtime objects from YAML / app config, not persisted in a "skills" table. **No migrations needed.** |
| Documentation | 5 mentions in README.md, 0 in `/docs/` | Trivial. |
| YAML skill configs | ~10 files in `api/pkg/agent/skill/api_skills/*.yaml` | File contents stay; only the package path moves. |

Generated frontend types (`TypesSkillDefinition`) come from the Go swagger spec, so renaming Go types automatically renames frontend types after `./stack update_openapi`.

## Backward compatibility

**Frontend-only callers** (the helix frontend itself): no compat needed — frontend ships in lockstep with backend.

**External callers** of the REST API: per the survey, `/api/v1/skills` is not documented in `/docs/` and no external integrations are known. But to be safe, ship a one-release deprecation window:

- Both `/api/v1/skills` and `/api/v1/mcps-and-apis` are wired to the same handler.
- The old path is marked `@deprecated` in swagger with a "renamed to /api/v1/mcps-and-apis" note.
- Old path logs a deprecation warning once per process startup (not per request — avoids log spam).
- Both response shapes include both field names (`{"skills": [...], "mcpsAndApis": [...]}`) for that release. (Cheap — same data, two key aliases.)
- Next major release: remove the aliases.

**URL slug** (`?tab=skills`): the tab routing code reads the slug and shows a tab. Easiest fix: at the routing entry, if `tab=skills` is seen, rewrite to `tab=mcps-and-apis` and update the URL via `router.replace()`. This is a 3-line change.

## Where the notice goes

Both editors render the same shared component (currently `Skills.tsx` → renamed `MCPsAndAPIs.tsx`):

| Page | File | Render site |
| --- | --- | --- |
| Project Settings | `frontend/src/pages/ProjectSettings.tsx` (≈L1837-1857) | `<MCPsAndAPIs … hideHeader compactGrid />` |
| Agent Settings | `frontend/src/pages/App.tsx` (≈L125-136) | `<MCPsAndAPIs … />` |

Editing the shared component once covers both pages.

Add near the top of the rendered output, above the category/grid layout:

```tsx
<Alert severity="info" sx={{ mb: 2 }}>
  Changes to MCPs and APIs take effect in <strong>new sessions</strong>.
  Restart any active session to pick up updates.
</Alert>
```

Precedent for the pattern: the OAuth Configuration Warning already in the same file uses `<Alert severity="warning">` inside `<Collapse>`. We omit `<Collapse>` + dismiss here because the notice should always be visible.

## Key decisions

1. **Bundle both changes in one PR (or one feature branch with sequential PRs).** They touch the same files. Shipping the notice separately would mean writing it against names that are about to disappear.
2. **`severity="info"`, not `"warning"`.** Expected behavior, not a misconfiguration. Reserve warnings for real problems.
3. **Notice is non-dismissible.** Users hit this every time they edit; a one-time dismiss would re-train them to ignore future banners.
4. **Code identifiers fully match the UI label.** The user explicitly asked for the rename "throughout the app, including in the source code". `mcpAndApi`/`MCPAndAPI` is verbose but unambiguous and search-friendly.
5. **Keep the unified editor.** Splitting into separate "MCPs" and "APIs" tabs was considered. Out of scope: the editor's internal structure already lets users filter by category (MCP Servers, etc.); a hard split is more work for arguably the same discoverability win as just having "MCP" in the tab title.
6. **One-release API alias.** Even though no known external clients use `/api/v1/skills`, adding a tiny alias buys safety at near-zero cost.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Sheer churn in PR review | Split commits by layer (UI text only / code identifiers / Go package move / API rename / docs) so reviewers can read one concern at a time. |
| Missed references after rename | After mechanical replace, `grep -i "skill"` the whole repo and triage what's left. Some will be legitimate (Anthropic Skills mentions, comments, the new alias). |
| Generated types out of date | Run `./stack update_openapi` after backend rename and commit the regenerated client in the same PR. |
| Hidden external API consumers | The deprecation alias and swagger `@deprecated` note give them a release to migrate. |
| Bookmark breakage on tab slug | Client-side redirect from `?tab=skills` to `?tab=mcps-and-apis`. |
| Anthropic-Skills mentions get falsely renamed | Spot-check: search for the word "Anthropic" near "skill" and any mentions of the Skill API in `claude-api` code. Leave those alone. |

## Phasing (if the whole thing in one PR is too big)

Acceptable to ship in this order if the bundled change is too unwieldy:

1. UI text only (tab label, headings) — fast win for discoverability.
2. Session-restart notice (against new labels).
3. Frontend code identifiers.
4. Backend rename + API alias.
5. Remove deprecated alias (next major version).

Each phase is independently shippable.

## Files NOT modified

- Anthropic Skills integration code (if any) — that genuinely refers to the Anthropic feature.
- YAML skill definitions in `api/pkg/agent/skill/api_skills/*.yaml` (contents) — they describe individual APIs; only the package path changes.

## Notes for future agents

- The shared `Skills.tsx` component (~1900 lines) is the single source of truth for both project- and agent-level editors. Parent pages pass props (`hideHeader`, `compactGrid`, `defaultCategory`) to customize. If you need to differentiate behavior, extend the prop set — don't fork the component.
- Generated frontend types live in `frontend/src/api/api.ts` (and similar); regenerated by `./stack update_openapi` from the Go swagger spec. Always rename the Go side first, then regenerate.
- "Integrations", "Capabilities", and "Tools" are all taken in this codebase as separate concepts. Don't use them as a generic term for MCPs + APIs. "Connectors" is free if you ever need another bucket.
- The OAuth Warning block in the same component is a good template for any future MCPs & APIs editor alert.
