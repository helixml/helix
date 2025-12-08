# Agent Selection UX Flows

## Overview

Projects need a default agent (Helix App) for spec tasks. Users must select or create an agent in several flows. All agent creation assumes **external agent** type (`zed_external`) since that's what projects use.

When creating a new agent inline, users configure:
1. **Code Agent Runtime** - `zed_agent` (Zed's built-in) or `qwen_code` (Qwen Code CLI)
2. **Code Agent Model** - via AdvancedModelPicker with recommended models at top

## The Four Flows

### 1. Fork Sample Project (COMPLETED)
**Location:** `Projects.tsx`, `AgentSelectionModal.tsx`

- User clicks sample project to fork
- AgentSelectionModal appears showing agents (zed_external first)
- If no agents: inline form with runtime + model picker
- Fork proceeds with selected/created agent ID
- Passes `helix_app_id` in `ForkSimpleProjectRequest`

### 2. Create New Project (COMPLETED)
**Location:** `CreateProjectDialog.tsx`

- User opens "New Project" dialog
- Agent selection section shows existing agents (zed_external first)
- If no agents: inline form with runtime + model picker
- Creates project with `default_helix_app_id`

### 3. Project Settings (COMPLETED)
**Location:** `ProjectSettings.tsx`

- User opens project settings
- "Default Agent" section shows dropdown of existing agents
- Can change default agent for the project (auto-saves)
- "+ Create new agent" button shows inline form with runtime + model picker
- Updates project with new `default_helix_app_id`

### 4. Create Spec Task (COMPLETED)
**Location:** `SpecTasksPage.tsx`

- Dropdown to select agent, with project's default agent **pre-selected**
- User can override the default agent for this specific task
- If project has no `default_helix_app_id`, auto-selects first zed_external agent
- If no agents exist: shows option to create one inline

## Key Requirements

1. **Default is pre-selected**: When creating a spec task, the project's default agent should be pre-selected in the dropdown
2. **Override allowed**: User can choose a different agent for a specific task if desired
3. **External agents only**: All inline agent creation assumes zed_external type
4. **Two configuration fields**: Code Agent Runtime + Code Agent Model (not agent_type like Basic/Multi-turn)
5. **No cloud provider references**: UI text should be generic (not mention Claude/Anthropic specifically) for on-prem customers
6. **No hardcoded model defaults**: Start with empty model selection, let AdvancedModelPicker auto-select first available model
7. **Auto-generated agent names**: Generate name from model + runtime (e.g., "Opus 4.5 in Zed Agent"), but stop auto-generating if user modifies the name field

## Agent Name Auto-Generation

The agent name is automatically generated based on the selected model and runtime:

```typescript
// In apps.tsx
export function generateAgentName(modelId: string, runtime: CodeAgentRuntime): string {
  if (!modelId) return '-'  // Show dash when model not yet selected
  const modelName = getModelDisplayName(modelId)
  const runtimeName = CODE_AGENT_RUNTIME_DISPLAY_NAMES[runtime]
  return `${modelName} in ${runtimeName}`
}

// Example outputs:
// "claude-opus-4-5-20251101" + "zed_agent" -> "Opus 4.5 in Zed Agent"
// "openai/gpt-5.1-codex" + "qwen_code" -> "GPT-5.1 in Qwen Code"
// "" + "zed_agent" -> "-" (model not loaded yet)
```

Implementation pattern for tracking user modifications:
```typescript
const [newAgentName, setNewAgentName] = useState('-')
const [userModifiedName, setUserModifiedName] = useState(false)

// Auto-generate when model/runtime changes (unless user modified)
useEffect(() => {
  if (!userModifiedName && showCreateAgentForm) {
    setNewAgentName(generateAgentName(selectedModel, codeAgentRuntime))
  }
}, [selectedModel, codeAgentRuntime, userModifiedName, showCreateAgentForm])

// In TextField onChange:
onChange={(e) => {
  setNewAgentName(e.target.value)
  setUserModifiedName(true)  // User took control
}}
```

## Key Types

```go
// Project has default agent
type Project struct {
    DefaultHelixAppID string `json:"default_helix_app_id,omitempty"`
}

// Fork request includes agent
type ForkSimpleProjectRequest struct {
    HelixAppID string `json:"helix_app_id,omitempty"`
}
```

```typescript
// Code agent runtime options
type CodeAgentRuntime = 'zed_agent' | 'qwen_code'

// Display names for runtimes
const CODE_AGENT_RUNTIME_DISPLAY_NAMES: Record<CodeAgentRuntime, string> = {
  'zed_agent': 'Zed Agent',
  'qwen_code': 'Qwen Code',
}

// ICreateAgentParams includes codeAgentRuntime
interface ICreateAgentParams {
  codeAgentRuntime?: CodeAgentRuntime;
  // ... other fields
}
```

## Recommended Models

State-of-the-art coding models for zed_external agents (starred and shown at top of model picker):

**Anthropic:**
- `claude-opus-4-5-20251101`
- `claude-sonnet-4-5-20250929`
- `claude-haiku-4-5-20251001`

**OpenAI:**
- `openai/gpt-5.1-codex`
- `openai/gpt-oss-120b`

**Google Gemini:**
- `gemini-2.5-pro`
- `gemini-2.5-flash`

**Zhipu GLM:**
- `glm-4.6`

**Qwen (Coder + Large):**
- `Qwen/Qwen3-Coder-480B-A35B-Instruct`
- `Qwen/Qwen3-Coder-30B-A3B-Instruct`
- `Qwen/Qwen3-235B-A22B-fp8-tput`

**Note**: These are NOT hardcoded as defaults. The AdvancedModelPicker auto-selects the first available enabled model when it loads. The recommended list just affects display order (starred and shown at top).

## MCP Hint

All agent selection UIs include plain text hint: "You can configure MCP servers in the agent settings after creation."

(Not a link, since the agent may not exist yet when creating inline.)

## Components

- `AgentSelectionModal.tsx` - Modal for fork flow (runtime + model picker)
- `CreateProjectDialog.tsx` - Agent section in project creation (runtime + model picker)
- `ProjectSettings.tsx` - Default agent section (runtime + model picker)
- `SpecTasksPage.tsx` - Pre-selects project's default agent
- `AdvancedModelPicker` - Model selection with recommended models at top, auto-selects first available
- `apps.tsx` - `CodeAgentRuntime` type, `generateAgentName()`, and `ICreateAgentParams`

## Zed Model ID Compatibility

**Problem:** Zed's `agent.default_model` only accepts model IDs from its built-in enum, not arbitrary upstream model IDs. For example, Zed accepts `claude-3-5-haiku-latest` but NOT `claude-3-5-haiku-20241022`.

**Current Workaround:** `normalizeModelIDForZed()` in `api/pkg/external-agent/zed_config.go` converts dated Anthropic model IDs to their `-latest` equivalents before sending to Zed.

**Better Solution (TODO):** Zed supports `available_models` in language_models settings:

```json
{
  "language_models": {
    "anthropic": {
      "api_url": "http://api:8080",
      "available_models": [
        {
          "name": "claude-3-5-haiku-20241022",
          "display_name": "Claude 3.5 Haiku",
          "max_tokens": 8192
        }
      ]
    },
    "openai": {
      "api_url": "http://api:8080/v1",
      "available_models": [
        {
          "name": "Qwen/Qwen3-Coder-480B-A35B-Instruct",
          "display_name": "Qwen 3 Coder 480B",
          "max_tokens": 32768
        }
      ]
    }
  },
  "agent": {
    "default_model": {
      "provider": "openai",
      "model": "Qwen/Qwen3-Coder-480B-A35B-Instruct"
    }
  }
}
```

This would let us use any model ID and have it appear in Zed's model dropdown. We could populate `available_models` from Helix's provider configuration to expose all configured models to Zed.

**Current Status:**
- Anthropic models: Normalized to `-latest` format (works)
- OpenAI-compatible models (Qwen, etc.): Pass through unchanged via OpenAI provider with custom api_url (should work)
- Future: Consider using `available_models` to expose all Helix-configured models to Zed
