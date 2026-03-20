# Implementation Tasks

- [ ] Add `YAMLSkillMCPSpec` struct and `MCP` field to `YAMLSkillSpec` in `api/pkg/types/skill.go`
- [ ] Create `api/pkg/agent/skill/api_skills/code-intelligence.yaml` with `spec.mcp.autoProvision: true`
- [ ] Update skill manager (`api/pkg/agent/skill/manager.go`) to include `spec.mcp` in the loaded `SkillDefinition` returned by the API
- [ ] Add `POST /api/v1/apps/{id}/skills/{skillName}/enable` endpoint that, for `autoProvision` MCP skills, constructs an `AssistantMCP` config from platform Kodit URL + user's API key and appends it to `app.mcpTools`
- [ ] Update frontend skill marketplace to call the enable endpoint on click (no dialog for `autoProvision` skills)
- [ ] Verify end-to-end: Code Intelligence skill listed → one-click enable → agent can call Kodit tools during inference
