# Design: Code Intelligence Skill

## Architecture Overview

The existing YAML skill system handles API-based skills. MCP-based skills are user-configured separately via `AddMcpSkillDialog`. This task bridges the gap by making Kodit MCP discoverable in the skills marketplace while still following the MCP tool configuration path.

### Approach

Extend the YAML skill format to support an `mcp` spec section (alongside the existing `api` section). When the skill manager loads a YAML skill with `spec.mcp`, it marks the skill as auto-configurable — no user input needed.

When the user enables the skill, the API generates the `AssistantMCP` config automatically: the Kodit MCP URL comes from the platform config (same source used by the settings-sync-daemon) and the auth token is the user's existing Helix API key. This is exactly how the settings-sync-daemon injects Kodit auth for Zed IDE today.

This reuses all existing MCP infrastructure (`AssistantMCP`, `NewDirectMCPClientSkills`, `MCPClientTool`, etc.) — no new agent/runtime code needed.

## New YAML Skill Format: `mcp` Section

```yaml
apiVersion: v1
kind: Skill
metadata:
  name: code-intelligence
  displayName: Code Intelligence
  provider: kodit
  category: Development
spec:
  description: |
    Search and navigate your codebase using semantic search, grep, and file browsing.
    Powered by Kodit MCP — enabled automatically using your Helix account.
  systemPrompt: |
    You have access to code intelligence tools that let you search repositories,
    read files, and understand codebases. Use these tools to answer questions
    about code, find implementations, and trace logic.
  icon:
    type: material-ui
    name: Code
  configurable: false
  mcp:
    transport: http
    autoProvision: true   # Helix generates URL + auth internally; no user input
```

The `spec.mcp.autoProvision: true` flag tells the backend to generate the `AssistantMCP` config server-side when the skill is enabled, rather than asking the user for a URL.

## Type Changes

**`api/pkg/types/skill.go`** — Add `MCPSpec` to `YAMLSkillSpec`:

```go
type YAMLSkillSpec struct {
    // existing fields...
    MCP *YAMLSkillMCPSpec `yaml:"mcp,omitempty" json:"mcp,omitempty"`
}

type YAMLSkillMCPSpec struct {
    Transport     string `yaml:"transport" json:"transport"`           // "http" or "sse"
    AutoProvision bool   `yaml:"autoProvision" json:"autoProvision"`   // generate URL+auth server-side
}
```

## Skill File

**`api/pkg/agent/skill/api_skills/code-intelligence.yaml`** — new file (see format above).

## API: Enable Skill Endpoint

Add a new endpoint (or extend an existing one) that handles enabling a marketplace skill on an app/assistant:

`POST /api/v1/apps/{id}/skills/{skillName}/enable`

For skills where `spec.mcp.autoProvision == true`, the handler:
1. Looks up the Kodit MCP base URL from platform config (same config used by the settings-sync-daemon's `codeAgentConfig`)
2. Uses the requesting user's Helix API key as the `Authorization: Bearer <key>` header
3. Constructs an `AssistantMCP` config and appends it to the app's `mcpTools`
4. Returns the updated app

## Frontend Changes

**`frontend/src/components/app/Skills.tsx`** (or equivalent skill marketplace component):
- When user clicks "Enable" on an MCP skill with `autoProvision: true`, call the enable endpoint directly — no dialog needed
- On success, the skill appears as enabled (MCP tool is now in the app config)

## Data Flow

```
User clicks "Enable" on Code Intelligence skill in marketplace
    → POST /api/v1/apps/{id}/skills/code-intelligence/enable
    → API looks up Kodit URL from platform config + user's API key
    → API constructs AssistantMCP{URL: koditURL, Headers: {"Authorization": "Bearer <userKey>"}}
    → Saved as mcpTools entry on app config (no user input)
    → At inference time: NewDirectMCPClientSkills() wraps Kodit tools
    → Agent calls semantic_search, grep, read_file, etc. via MCPClientTool.Execute()
```

## Key Decisions

- **No new runtime code**: All MCP execution infrastructure already exists. Only skill discovery and the enable endpoint are new.
- **YAML extension over new file type**: Keeps a single skill definition format; `spec.mcp` presence signals MCP-type skill.
- **Auto-provision over user input**: Kodit URL and auth are derived server-side from existing platform config and the user's API key — matching the pattern already used by the settings-sync-daemon.
- **New enable endpoint**: A dedicated `POST /api/v1/apps/{id}/skills/{name}/enable` keeps the logic server-side and avoids leaking internal URLs to the frontend.

## Codebase Patterns Found

- Skills are in `api/pkg/agent/skill/api_skills/*.yaml`, loaded by `skill/manager.go`
- `YAMLSkill` and `SkillDefinition` types are in `api/pkg/types/skill.go`
- MCP skill execution is in `api/pkg/agent/skill/mcp/mcp_skill.go` and `mcp_client.go`
- Skill validation endpoint is in `api/pkg/server/skills.go`
- Frontend MCP dialog is `frontend/src/components/app/AddMcpSkillDialog.tsx`
- Frontend skill hooks: `frontend/src/hooks/useSkills.ts`
