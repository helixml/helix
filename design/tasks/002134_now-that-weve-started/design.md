# Design: Configurable Model Selection for Claude Subscription Mode

## How Subscription Mode Currently Works

Helix runs the Claude Code agent inside a sandbox container, launched through a headless Zed (Claude ACP). The model the agent uses is therefore controlled by the **container environment**, not by anything Helix sends over ACP.

1. In `buildCodeAgentConfigFromAssistant()` (`api/pkg/server/zed_config_handlers.go:611`, branch at lines 661–678), when `isSubscription=true` and `runtime=claude_code`, the model fields are explicitly cleared (`model = ""`). So the ACP-side `CodeAgentConfig.Model` is empty in subscription mode — no model directive flows that way.
2. `subscriptionEnvForSession()` (`api/pkg/server/external_agent_handlers.go:130`, returns `[]string`) injects `ANTHROPIC_BASE_URL=https://api.anthropic.com`, clears `ANTHROPIC_API_KEY=`, and (for setup-token creds) sets `CLAUDE_CODE_OAUTH_TOKEN` — but sets **no model**.
3. Without `ANTHROPIC_MODEL` in the environment, Claude Code picks its own built-in default (currently Sonnet) — which is the behaviour we want to change.

### Why the env var (and not Zed's ACP model selector)

`ANTHROPIC_MODEL` is the documented Claude Code env var for the default model. Env vars returned by `subscriptionEnvForSession()` are appended to `agent.Env` (`external_agent_handlers.go:105`) and applied **last** in `buildEnvVars()` (`api/pkg/external-agent/hydra_executor.go:1248`), so they win over the base env and reach the in-container `claude` process. This is the single cleanest injection point.

We deliberately do **not** use Zed's ACP per-agent model-selection plumbing. That code path in the Zed fork (`crates/agent_servers/`) is currently mid-merge and inconsistent — the `default_model`/`favorite_models` settings fields referenced by `claude.rs` no longer exist on `CustomAgentServerSettings`, and `AcpConnection` does not implement a model selector. The env-var approach is layer-correct (it's a Helix container concern), simpler, and avoids that broken surface entirely.

## Changes Required

### 1. Backend — `AssistantConfig` type (`api/pkg/types/types.go:1525`)

Add one new field after `CodeAgentCredentialType` (line 1564), following the struct's existing `json` + `yaml` snake_case tag convention:

```go
// ClaudeSubscriptionModel is the Anthropic model to use when CodeAgentCredentialType
// is "subscription". Defaults to "claude-opus-4-6" when empty.
ClaudeSubscriptionModel string `json:"claude_subscription_model,omitempty" yaml:"claude_subscription_model,omitempty"`
```

### 2. Backend — `subscriptionEnvForSession()` (`api/pkg/server/external_agent_handlers.go:130`)

The function already loads the parent app's assistant config (`asst`) and only returns a non-empty slice when `runtime=claude_code` + `IsSubscription()` + an active subscription. After building the base env slice (`ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY=`), inject the model env var:

```go
model := asst.ClaudeSubscriptionModel
if model == "" {
    model = "claude-opus-4-6"
}
out = append(out, "ANTHROPIC_MODEL="+model)
```

This goes right after setting `ANTHROPIC_API_KEY=` so Opus is the default and user overrides persist. Because this is gated on subscription mode, API-key mode is untouched.

### 3. Frontend — `CodingAgentForm.tsx` (`frontend/src/components/agent/CodingAgentForm.tsx`)

The form already hides the full model picker when `claudeCodeMode === 'subscription'` (`showModelPicker = value.codeAgentRuntime !== 'claude_code' || value.claudeCodeMode === 'api_key'`). Add a simple `<Select>` just below the subscription/api-key toggle, visible only when `isClaudeCodeSubscription`:

- Three `<MenuItem>` entries, hardcoded:
  - `claude-opus-4-6` → "Claude Opus 4.6 (recommended)"
  - `claude-sonnet-4-5-latest` → "Claude Sonnet 4.5"
  - `claude-haiku-4-5-latest` → "Claude Haiku 4.5"
- Default selection: `claude-opus-4-6`.
- Selected value stored in `value.claudeSubscriptionModel` (new field on the form value type).

### 4. Frontend — form value type and `apps.tsx`

Naming convention to follow: **camelCase** in the form value (`CodingAgentFormValue`) and `ICreateAgentParams`; **snake_case** in `types.ts` (`IAssistantConfig`) and the API payload. `createAgent()` in `apps.tsx` does the camelCase→snake_case mapping.

- `CodingAgentForm.tsx`: add `claudeSubscriptionModel?: string` to `CodingAgentFormValue`; the `<Select>` updates it via `onChange({ ...value, claudeSubscriptionModel })`.
- `apps.tsx`: add `claudeSubscriptionModel?: string` to `ICreateAgentParams` and map it to `claude_subscription_model` in `createAgent()`.
- `utils/app.ts`: add the corresponding mapping for flat-state round-tripping (if a Claude subscription agent edit path exists there).

## Key Files

| File | Change |
|------|--------|
| `api/pkg/types/types.go` | Add `ClaudeSubscriptionModel` field to `AssistantConfig` |
| `api/pkg/server/external_agent_handlers.go` | Inject `ANTHROPIC_MODEL` env var in `subscriptionEnvForSession()` |
| `frontend/src/components/agent/CodingAgentForm.tsx` | Add 3-option model `<Select>` for subscription mode |
| `frontend/src/contexts/apps.tsx` | Pass `claudeSubscriptionModel` through to assistant config |
| `frontend/src/utils/app.ts` | Map `claude_subscription_model` in flat-state helpers |
| `frontend/src/types.ts` | Add `claude_subscription_model` to `IAssistantConfig` and related types |

## Learned patterns (verified against the codebase)

- Provider/model fields (`GenerationModel`, `GenerationModelProvider`) on `AssistantConfig` are deliberately blanked for `claude_code` + subscription in `buildCodeAgentConfigFromAssistant()` (`zed_config_handlers.go:661–678`); a separate `ClaudeSubscriptionModel` field avoids coupling and keeps that clearing logic intact.
- `subscriptionEnvForSession()` (`external_agent_handlers.go:130`) is the single place to inject container env vars for Claude subscription sessions — and it's already correctly gated (claude_code + `IsSubscription()` + active subscription). Add new per-session overrides here.
- Env injection order: `subscriptionEnvForSession()` → `agent.Env` → `buildEnvVars()` appends `agent.Env` **last** (`hydra_executor.go:1248`), so it wins over the base env. This is the same path that already makes `ANTHROPIC_BASE_URL`/`ANTHROPIC_API_KEY=` overrides take effect.
- Credential-type enum lives in `api/pkg/types/task_management.go` (`CodeAgentCredentialTypeSubscription = "subscription"`, `IsSubscription()`), not `types.go`.
- The `listClaudeModels` endpoint (`api/pkg/server/claude_subscription_handlers.go:302`, `GET /api/v1/claude-subscriptions/models`) already returns exactly these three IDs — `claude-opus-4-6`, `claude-sonnet-4-5-latest`, `claude-haiku-4-5-latest`. The frontend hardcodes them for simplicity (avoids an extra API call on form render); switching to the endpoint later is a drop-in if the tiers change.
- Avoid the Zed fork's ACP per-agent model plumbing (`crates/agent_servers/claude.rs` etc.) — it is mid-merge/inconsistent and not the right layer for a Helix container concern.
