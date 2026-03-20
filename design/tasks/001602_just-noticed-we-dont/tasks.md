# Implementation Tasks

- [ ] Add `YAMLSkillMCPSpec` struct and `MCP` field to `YAMLSkillSpec` in `api/pkg/types/skill.go`
- [ ] Create `api/pkg/agent/skill/api_skills/code-intelligence.yaml` with Kodit MCP skill definition
- [ ] Update skill manager (`api/pkg/agent/skill/manager.go`) to expose `spec.mcp` in the loaded `SkillDefinition` (so frontend can detect MCP-type skills)
- [ ] Update frontend skill marketplace to detect MCP-type skills (`spec.mcp` present) and show a URL+token config dialog instead of OAuth flow when enabling
- [ ] Reuse `POST /api/v1/skills/validate` in the frontend config dialog to test the connection before saving
- [ ] Save the configured skill as an `AssistantMCP` entry in `app.mcpTools` (same as manually adding an MCP tool)
- [ ] Verify end-to-end: Code Intelligence skill listed → user configures URL/key → agent can call Kodit tools during inference
