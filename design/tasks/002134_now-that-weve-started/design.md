# Design: Configurable Model Selection for Claude Subscription Mode

## How Subscription Mode Currently Works

1. In `buildCodeAgentConfigFromAssistant()` (`zed_config_handlers.go`), when `isSubscription=true` and `runtime=claude_code`, the model fields are explicitly cleared (`model = ""`).
2. `subscriptionEnvForSession()` injects `ANTHROPIC_BASE_URL=https://api.anthropic.com`, clears `ANTHROPIC_API_KEY`, and sets the OAuth token — but sets no model.
3. Without `ANTHROPIC_MODEL` in the environment, Claude Code picks its own built-in default (currently Sonnet 4.x).

## Changes Required

### 1. Backend — `AssistantConfig` type (`api/pkg/types/types.go`)

Add one new field after `CodeAgentCredentialType`:

```go
// ClaudeSubscriptionModel is the Anthropic model to use when CodeAgentCredentialType
// is "subscription". Defaults to "claude-opus-4-6" when empty.
ClaudeSubscriptionModel string `json:"claude_subscription_model,omitempty" yaml:"claude_subscription_model,omitempty"`
```

### 2. Backend — `subscriptionEnvForSession()` (`api/pkg/server/external_agent_handlers.go`)

After building the base env slice (`ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY=`), inject the model env var:

```go
model := asst.ClaudeSubscriptionModel
if model == "" {
    model = "claude-opus-4-6"
}
out = append(out, "ANTHROPIC_MODEL="+model)
```

This goes right after setting `ANTHROPIC_API_KEY=` so Opus is the default and user overrides persist.

### 3. Frontend — `CodingAgentForm.tsx` (`frontend/src/components/agent/CodingAgentForm.tsx`)

The form already hides the full model picker when `claudeCodeMode === 'subscription'` (`showModelPicker = value.codeAgentRuntime !== 'claude_code' || value.claudeCodeMode === 'api_key'`). Add a simple `<Select>` just below the subscription/api-key toggle, visible only when `isClaudeCodeSubscription`:

- Three `<MenuItem>` entries, hardcoded:
  - `claude-opus-4-6` → "Claude Opus 4.6 (recommended)"
  - `claude-sonnet-4-5-latest` → "Claude Sonnet 4.5"
  - `claude-haiku-4-5-latest` → "Claude Haiku 4.5"
- Default selection: `claude-opus-4-6`.
- Selected value stored in `value.claudeSubscriptionModel` (new field on the form value type).

### 4. Frontend — form value type and `apps.tsx`

In `apps.tsx`, add `claudeSubscriptionModel?: string` to the form params type and pass it through to the assistant config as `claude_subscription_model`.

In `utils/app.ts`, add the corresponding mapping for flat-state round-tripping.

## Key Files

| File | Change |
|------|--------|
| `api/pkg/types/types.go` | Add `ClaudeSubscriptionModel` field to `AssistantConfig` |
| `api/pkg/server/external_agent_handlers.go` | Inject `ANTHROPIC_MODEL` env var in `subscriptionEnvForSession()` |
| `frontend/src/components/agent/CodingAgentForm.tsx` | Add 3-option model `<Select>` for subscription mode |
| `frontend/src/contexts/apps.tsx` | Pass `claudeSubscriptionModel` through to assistant config |
| `frontend/src/utils/app.ts` | Map `claude_subscription_model` in flat-state helpers |
| `frontend/src/types.ts` | Add `claude_subscription_model` to `IAssistantConfig` and related types |

## Why `ANTHROPIC_MODEL` env var, not Zed settings

In subscription mode the Claude Code process (`claude` CLI) inherits env vars set by `subscriptionEnvForSession()`. These take effect before Zed's `agent.default_model` in `settings.json` is applied. Using an env var is the cleanest injection point: the daemon writes Zed settings once at container start, but `ANTHROPIC_MODEL` is already in the process environment before `claude` even launches.

## Learned patterns

- Provider/model fields (`GenerationModel`, `GenerationModelProvider`) on `AssistantConfig` are deliberately blanked in subscription mode; a separate field avoids coupling.
- `subscriptionEnvForSession()` is the single place to inject container env vars for Claude subscription sessions — add new per-session overrides here.
- The `listClaudeModels` API endpoint already returns these three models with the correct IDs; the frontend hardcodes them for simplicity (avoids an extra API call on form render).
