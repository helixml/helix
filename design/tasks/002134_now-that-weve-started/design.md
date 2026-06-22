# Design: Configurable Model Selection for Claude Subscription Mode

## How the Claude Code model is actually controlled (corrected)

Helix runs the Claude Code agent inside a sandbox container, launched through a headless Zed (Claude ACP). The model is driven by `CodeAgentConfig.Model`, which the settings-sync-daemon writes into the container's **`/etc/claude-code/managed-settings.json`**. The `claude-agent-acp` package reads that file via `resolveModelPreference()` (substring-matches `claude-opus-4-6` → its canonical value id). The daemon also mirrors it into Zed settings `agent_servers.claude-acp.default_model`.

1. `buildCodeAgentConfigFromAssistant()` (`api/pkg/server/zed_config_handlers.go`, `claude_code`+subscription branch) previously hard-cleared `model = ""` in subscription mode → daemon wrote `{}` → claude-agent-acp defaulted to **Sonnet**. That's the behaviour we change.
2. The daemon (`api/cmd/settings-sync-daemon/main.go`, `writeClaudeManagedSettings()`) writes `{"model": codeAgentConfig.Model}` and sets `agent_servers.claude-acp.default_model` when `Model != ""`.

### ⚠️ Correction from the first implementation pass

The first pass injected `ANTHROPIC_MODEL` via `subscriptionEnvForSession()`. **That is the wrong layer** — claude-agent-acp resolves its model from `managed-settings.json`, not that env var, so the env injection was effectively a no-op against Sonnet. It was reverted. The correct lever is `CodeAgentConfig.Model` → `managed-settings.json` (the pathway the codebase is already built around; the daemon's own comment uses `claude-opus-4-6` as the worked example).

We also deliberately do **not** touch Zed's ACP per-agent model-selection plumbing (`crates/agent_servers/`) — it's mid-merge/inconsistent and the wrong layer for a Helix container concern.

## Changes Required

### 1. Backend — `AssistantConfig` type (`api/pkg/types/types.go:1525`)

Add one new field after `CodeAgentCredentialType` (line 1564), following the struct's existing `json` + `yaml` snake_case tag convention:

```go
// ClaudeSubscriptionModel is the Anthropic model to use when CodeAgentCredentialType
// is "subscription". Defaults to "claude-opus-4-6" when empty.
ClaudeSubscriptionModel string `json:"claude_subscription_model,omitempty" yaml:"claude_subscription_model,omitempty"`
```

### 2. Backend — `buildCodeAgentConfigFromAssistant()` (`api/pkg/server/zed_config_handlers.go`)

In the `claude_code` + subscription branch, set the model (default Opus) instead of clearing it. The daemon writes it to `managed-settings.json`:

```go
if isSubscription {
    providerName = ""; baseURL = ""; apiType = ""
    model = assistant.ClaudeSubscriptionModel
    if model == "" { model = "claude-opus-4-6" }
}
```

Also guard `injectAvailableModels()` in the daemon to skip `claude_code` (the now-set model must not be injected as a bogus `openai` Custom model when `APIType` is empty in subscription mode).

<details><summary>Superseded first-pass approach (ANTHROPIC_MODEL env injection — reverted)</summary>

```go
model := asst.ClaudeSubscriptionModel
if model == "" {
    model = "claude-opus-4-6"
}
out = append(out, "ANTHROPIC_MODEL="+model)
```

This was reverted — it does not actually change the model (claude-agent-acp ignores `ANTHROPIC_MODEL` in favour of managed-settings.json).

</details>

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

## Implementation Notes (discovered during build)

- **Two UI surfaces, not one.** Agent *creation* goes through the shared `CodingAgentForm` (used by Onboarding, CreateProjectDialog, AgentSelectionModal, ProjectSettings, NewSpecTaskForm). Agent *editing* uses a separate UI in `AppSettings.tsx` (its own `claudeCodeMode` radio + `onUpdate(...)` → `useApp.ts`). Both need the dropdown.
- **Create-form parents do NOT spread `...nextValue`.** Each parent rebuilds `CodingAgentForm`'s `value` from individual `useState` pieces and only extracts specific fields in `onChange` (e.g. `setClaudeCodeMode(nextValue.claudeCodeMode)`). So threading a new field through the shared `value` would require editing all 5 parents. Instead, the subscription-model dropdown is kept as **local state inside `CodingAgentForm`** (default `claude-opus-4-6`), read directly by its own `handleCreateAgent` when building `ICreateAgentParams`. Zero parent changes; correct because create forms never load an existing model.
- **Edit path** (`AppSettings.tsx`): dropdown initialized from `app.claude_subscription_model` (fallback Opus), persisted via `onUpdate({ claude_subscription_model })`; `useApp.ts` applies it to `assistants[0]`.
- **Backend default is the real guarantee.** Even with no UI selection, `subscriptionEnvForSession()` injects `ANTHROPIC_MODEL=claude-opus-4-6` when the field is empty, so subscription agents default to Opus regardless of the form.

## Learned patterns (verified against the codebase)

- Provider/model fields (`GenerationModel`, `GenerationModelProvider`) on `AssistantConfig` are deliberately blanked for `claude_code` + subscription in `buildCodeAgentConfigFromAssistant()` (`zed_config_handlers.go:661–678`); a separate `ClaudeSubscriptionModel` field avoids coupling and keeps that clearing logic intact.
- `subscriptionEnvForSession()` (`external_agent_handlers.go:130`) is the single place to inject container env vars for Claude subscription sessions — and it's already correctly gated (claude_code + `IsSubscription()` + active subscription). Add new per-session overrides here.
- Env injection order: `subscriptionEnvForSession()` → `agent.Env` → `buildEnvVars()` appends `agent.Env` **last** (`hydra_executor.go:1248`), so it wins over the base env. This is the same path that already makes `ANTHROPIC_BASE_URL`/`ANTHROPIC_API_KEY=` overrides take effect.
- Credential-type enum lives in `api/pkg/types/task_management.go` (`CodeAgentCredentialTypeSubscription = "subscription"`, `IsSubscription()`), not `types.go`.
- The `listClaudeModels` endpoint (`api/pkg/server/claude_subscription_handlers.go:302`, `GET /api/v1/claude-subscriptions/models`) already returns exactly these three IDs — `claude-opus-4-6`, `claude-sonnet-4-5-latest`, `claude-haiku-4-5-latest`. The frontend hardcodes them for simplicity (avoids an extra API call on form render); switching to the endpoint later is a drop-in if the tiers change.
- Avoid the Zed fork's ACP per-agent model plumbing (`crates/agent_servers/claude.rs` etc.) — it is mid-merge/inconsistent and not the right layer for a Helix container concern.
