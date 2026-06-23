# Design: Subscription Mode Should Default to Latest Opus, Not Opus 4.6

## Problem

`claude-opus-4-6` is hardcoded as the default subscription model in 7 places:

| File | What's hardcoded |
|------|-----------------|
| `api/pkg/server/zed_config_handlers.go:677` | Backend default when `ClaudeSubscriptionModel` is empty |
| `api/pkg/server/claude_subscription_handlers.go:304` | `listClaudeModels` endpoint return values |
| `api/pkg/types/types.go:1570` | Doc comment on `ClaudeSubscriptionModel` field |
| `frontend/src/components/agent/CodingAgentForm.tsx:16,20` | `CLAUDE_SUBSCRIPTION_MODELS` array + `DEFAULT_CLAUDE_SUBSCRIPTION_MODEL` |
| `frontend/src/components/app/AppSettings.tsx:269` | Edit-path fallback |
| `frontend/src/pages/Onboarding.tsx:62` | `DEFAULT_ONBOARDING_AGENT_MODEL` |

Meanwhile, the latest Opus is 4.8. The subscription path is stuck two versions behind. Every future Opus release would require updating all these hardcoded IDs again.

## Key insight: Claude Code resolves tier-level shorthand names

Claude Code's `resolveModelPreference()` function (in the claude-agent-acp package) performs substring matching on the model value from managed-settings.json. Claude Code's `--model` flag already accepts shorthand names like `"opus"`, `"sonnet"`, `"haiku"` and resolves them to the latest available version of that tier.

The settings-sync-daemon writes `CodeAgentConfig.Model` directly into managed-settings.json:

```go
settings["model"] = d.codeAgentConfig.Model  // whatever string we pass flows through unchanged
```

This means if we pass `"opus"` instead of `"claude-opus-4-6"`, Claude Code will resolve it to the current latest Opus (currently `claude-opus-4-8`). When Anthropic ships Opus 4.9, the resolution will automatically pick it up — no Helix code changes needed.

## Approach

Use tier-level shorthand names (`"opus"`, `"sonnet"`, `"haiku"`) instead of pinned version IDs. This is a much simpler change than bumping hardcoded versions, and it's future-proof.

### 1. Backend — Use shorthand identifiers

**`zed_config_handlers.go:677`** — Change the default:

```go
if model == "" {
    model = "opus"
}
```

**`claude_subscription_handlers.go`** — Update the model list to use shorthand IDs:

```go
func (apiServer *HelixAPIServer) listClaudeModels(...) ([]*ClaudeModel, *system.HTTPError) {
    models := []*ClaudeModel{
        {ID: "opus", Name: "Claude Opus", Description: "Most capable Claude model"},
        {ID: "sonnet", Name: "Claude Sonnet", Description: "Best balance of speed and capability"},
        {ID: "haiku", Name: "Claude Haiku", Description: "Fastest Claude model"},
    }
    return models, nil
}
```

**`types.go:1570`** — Update doc comment to say `"opus"` instead of `"claude-opus-4-6"`.

### 2. Frontend — Update labels (no version numbers)

**`CodingAgentForm.tsx`** — Update the hardcoded list to match:

```typescript
export const CLAUDE_SUBSCRIPTION_MODELS: { id: string; label: string }[] = [
  { id: 'opus', label: 'Claude Opus (recommended)' },
  { id: 'sonnet', label: 'Claude Sonnet' },
  { id: 'haiku', label: 'Claude Haiku' },
]
export const DEFAULT_CLAUDE_SUBSCRIPTION_MODEL = 'opus'
```

Apply the same change in `AppSettings.tsx`.

**`Onboarding.tsx:62`** — This is for the API-key path's auto-select, not subscription. Update from `claude-opus-4-6` to `claude-opus-4-8` (this path needs a concrete model ID since it doesn't go through `resolveModelPreference()`).

### 3. Fix `normalizeModelIDForZed` (`zed_config.go`)

The normalizer is missing entries for 4.7 and 4.8. This affects the API-key path only (not subscription), but is a pre-existing bug.

Add specific entries before the generic `claude-opus-4` fallback:

```go
if strings.HasPrefix(modelID, "claude-opus-4-8") {
    return "claude-opus-4-8-latest"
}
if strings.HasPrefix(modelID, "claude-opus-4-7") {
    return "claude-opus-4-7-latest"
}
```

### 4. Update tests

- `zed_config_handlers_test.go`: Update expected default model from `claude-opus-4-6` to `opus`.
- `settings-sync-daemon/main_test.go:63`: Update test fixture model.
- `zed_config_test.go`: Add normalizer test cases for 4.7 and 4.8.

## Key decisions

**Why shorthand names instead of bumping to `claude-opus-4-8`?** Bumping to 4.8 fixes the problem today but recreates it when Opus 4.9 ships. Using `"opus"` delegates version resolution to Claude Code, which already maintains this mapping. Zero maintenance on our side.

**Why not fetch models from the API in the frontend?** The original spec (002134) noted "switching to the endpoint later is a drop-in if the tiers change." But with tier-level shorthand names, the list doesn't change when a new version ships — `"opus"` always means latest Opus. The hardcoded frontend list is fine because it's now tier-based, not version-based. We can move to API-fetched models later if we need to add/remove tiers.

**Why not migrate existing agents from `claude-opus-4-6`?** Existing agents keep their stored value. `resolveModelPreference()` handles both shorthand (`"opus"`) and versioned (`"claude-opus-4-6"`) identifiers. No migration needed.

**What if `resolveModelPreference()` doesn't support bare shorthand?** The first implementation task is to verify this in a real subscription container. If `"opus"` doesn't resolve correctly, the fallback is using `"claude-opus-4-8"` (a simple version bump). The substring matching behavior is documented in the settings-sync-daemon code comments and aligns with Claude Code's `--model` flag behavior.

## Implementation notes

- `AppSettings.tsx` imports `CLAUDE_SUBSCRIPTION_MODELS` and `DEFAULT_CLAUDE_SUBSCRIPTION_MODEL` from `CodingAgentForm.tsx` — updating the constants in CodingAgentForm fixes both files automatically.
- `settings-sync-daemon/main_test.go:63` uses `claude-opus-4-6` but it's testing the Zed settings injection skip logic (API-key path), not the subscription default — no change needed.
- `docs.go` (generated OpenAPI spec) still has old doc comment — needs `./stack update_openapi` regeneration.
- `frontend/src/types.ts` and `frontend/src/api/api.ts` contain old doc comment from generated types — will update when OpenAPI is regenerated.
- Added `claude-opus-4-8` as the first entry in `RECOMMENDED_CODING_MODELS` (API-key path model list) — this was an additional gap not in the original task list.

## Codebase patterns (from 002134 spec)

- `ClaudeSubscriptionModel` flows through `CodeAgentConfig.Model` → `managed-settings.json` → `resolveModelPreference()` in claude-agent-acp. The settings-sync-daemon writes it; claude-agent-acp reads it.
- The `normalizeModelIDForZed` function is for Zed's built-in model list (API-key path); the subscription path bypasses it.
- `subscriptionEnvForSession()` injects env vars for subscription containers; the model goes through `CodeAgentConfig.Model`, not env vars (learned the hard way in 002134).
