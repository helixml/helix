# Design: Code Intelligence Skill

## Architecture Overview

The existing YAML skill system handles API-based skills. MCP-based skills are user-configured separately via `AddMcpSkillDialog`. This task bridges the gap by making Kodit MCP discoverable in the skills marketplace while still following the MCP tool configuration path.

### Approach

Extend the YAML skill format to support an `mcp` spec section (alongside the existing `api` section). When the skill manager loads a YAML skill with `spec.mcp`, it marks the skill as `configurable: true` and MCP-type. The frontend skill marketplace handles these differently: instead of OAuth flow, it opens a URL-input dialog pre-populated with defaults.

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
    Connect to a Kodit MCP server to give your agent code intelligence capabilities.
  systemPrompt: |
    You have access to code intelligence tools that let you search repositories,
    read files, and understand codebases. Use these tools to answer questions
    about code, find implementations, and trace logic.
  icon:
    type: material-ui
    name: Code
  configurable: true
  mcp:
    transport: http
    urlPlaceholder: "https://your-kodit-instance/mcp"
    authHeader: Authorization
    authPrefix: "Bearer "
```

The `spec.mcp` section tells the frontend this skill needs an MCP URL + auth token to be configured (as opposed to the OAuth flow used by API skills).

## Type Changes

**`api/pkg/types/skill.go`** — Add `MCPSpec` to `YAMLSkillSpec`:

```go
type YAMLSkillSpec struct {
    // existing fields...
    MCP *YAMLSkillMCPSpec `yaml:"mcp,omitempty" json:"mcp,omitempty"`
}

type YAMLSkillMCPSpec struct {
    Transport      string `yaml:"transport" json:"transport"`             // "http" or "sse"
    URLPlaceholder string `yaml:"urlPlaceholder" json:"urlPlaceholder"`   // hint for UI
    AuthHeader     string `yaml:"authHeader" json:"authHeader"`
    AuthPrefix     string `yaml:"authPrefix" json:"authPrefix"`
}
```

## Skill File

**`api/pkg/agent/skill/api_skills/code-intelligence.yaml`** — new file (see format above).

## Frontend Changes

**`frontend/src/components/app/Skills.tsx`** (or equivalent skill marketplace component):
- Detect skills where `skill.spec.mcp` is set
- When user clicks "Enable" on such a skill, open a simple dialog asking for:
  - MCP Server URL (pre-filled with `urlPlaceholder`)
  - API Key / Bearer token
- On confirm, create an `AssistantMCP` entry and add it to `app.mcpTools`

This reuses the existing `validate` endpoint (`POST /api/v1/skills/validate`) to confirm the connection works before saving.

## Data Flow

```
User enables "Code Intelligence" in marketplace
    → Dialog: enter Kodit URL + API key
    → POST /api/v1/skills/validate (AssistantMCP) → confirm tools available
    → Save as mcpTools entry on app config
    → At inference time: NewDirectMCPClientSkills() wraps Kodit tools
    → Agent calls semantic_search, grep, read_file, etc. via MCPClientTool.Execute()
```

## Key Decisions

- **No new runtime code**: All MCP execution infrastructure already exists. Only skill discovery and UI configuration are new.
- **YAML extension over new file type**: Keeps a single skill definition format; `spec.mcp` presence signals MCP-type skill.
- **URL per org/instance**: Kodit URLs are not centrally known at skill definition time, so `urlPlaceholder` guides users rather than hard-coding.
- **Reuse validate endpoint**: No new API endpoint needed; `POST /api/v1/skills/validate` already tests MCP connections and returns available tools.

## Codebase Patterns Found

- Skills are in `api/pkg/agent/skill/api_skills/*.yaml`, loaded by `skill/manager.go`
- `YAMLSkill` and `SkillDefinition` types are in `api/pkg/types/skill.go`
- MCP skill execution is in `api/pkg/agent/skill/mcp/mcp_skill.go` and `mcp_client.go`
- Skill validation endpoint is in `api/pkg/server/skills.go`
- Frontend MCP dialog is `frontend/src/components/app/AddMcpSkillDialog.tsx`
- Frontend skill hooks: `frontend/src/hooks/useSkills.ts`
