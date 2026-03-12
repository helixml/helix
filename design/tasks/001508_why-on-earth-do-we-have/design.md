# Design: Fix Context Length and API Key in Settings-Sync-Daemon

## Root Cause Analysis

### Problem 1: Wrong context length (128K instead of real value)

**Chain of events:**

1. `buildCodeAgentConfigFromAssistant()` in `zed_config_handlers.go` looks up model info:
   ```go
   modelInfo, err := apiServer.modelInfoProvider.GetModelInfo(ctx, &modelPkg.ModelInfoRequest{
       Provider: providerName,  // "anthropic"
       Model:    modelName,     // e.g. "claude-opus-4-6" (dashes)
   })
   if err == nil {
       maxTokens = modelInfo.ContextLength        // Would be 1,000,000
       maxOutputTokens = modelInfo.MaxCompletionTokens  // Would be 128,000
   }
   ```

2. The lookup in `BaseModelInfoProvider.GetModelInfo()` constructs slug `anthropic/claude-opus-4-6` and iterates all models comparing against `model.Slug` (`anthropic/claude-opus-4.6` — dots). **These don't match** because dashes ≠ dots.

3. `GetModelInfo` returns an error → `maxTokens` stays 0 → goes into `CodeAgentConfig.MaxTokens` as 0.

4. In the settings-sync-daemon's `injectAvailableModels()`:
   ```go
   maxTokens := d.codeAgentConfig.MaxTokens
   if maxTokens == 0 {
       maxTokens = 128000 // Hardcoded fallback
   }
   ```

5. Zed's `AnthropicAvailableModel.max_tokens` field means **context window size**, not max output. So Zed thinks the model has a 128K context window.

**The real context lengths (from model_info.json):**
- `claude-opus-4.6`: context_length=1,000,000, max_completion_tokens=128,000
- `claude-opus-4.5`: context_length=200,000, max_completion_tokens=128,000
- `claude-sonnet-4`: context_length=200,000

### Problem 2: api_key written to settings.json is dead config

**How Zed reads API keys (in priority order):**
1. Check `ANTHROPIC_API_KEY` env var → if set, use it (see `ApiKeyState.load_if_needed()` in `zed/crates/language_model/src/api_key.rs`)
2. Fall back to system keychain lookup

**Zed's `AnthropicSettings` struct only has two fields:** `api_url` and `available_models`. There is no `api_key` field. Any `api_key` in settings.json is silently ignored.

**The env var is already set correctly** by `DesktopAgentAPIEnvVars()` in `types.go`:
```go
func DesktopAgentAPIEnvVars(apiKey string) []string {
    return []string{
        "USER_API_TOKEN=" + apiKey,
        "ANTHROPIC_API_KEY=" + apiKey,  // ← Already handled
        "OPENAI_API_KEY=" + apiKey,
        "ZED_HELIX_TOKEN=" + apiKey,
    }
}
```

So `injectLanguageModelAPIKey()` writes a token to a file where nothing reads it.

## Solution Design

### Fix 1: Model name normalization in GetModelInfo

**Location:** `api/pkg/model/model_info.go` → `GetModelInfo()`

Add a normalization step that converts dashes to dots for version-like suffixes in model names before slug comparison. The pattern is specific: model names like `claude-opus-4-6` should match `claude-opus-4.6` — trailing numeric segments separated by dashes should be tried with dots.

A simpler and more robust approach: after the existing slug-based lookups fail, try normalizing the slug by replacing the last `-` before a digit sequence with `.` (e.g. `anthropic/claude-opus-4-6` → `anthropic/claude-opus-4.6`). This avoids changing behavior for models that legitimately use dashes.

**Alternatively** (and probably better): fix it at the source in `buildCodeAgentConfigFromAssistant()` — the model name stored in the assistant config should already use the canonical format matching model_info.json. However, model names come from user/UI input and provider APIs using different formats, so normalizing at the lookup layer is more defensive.

**Recommended approach:** Add normalization in `GetModelInfo()` as a fallback, similar to how `trimAnthropicDateSuffix` already exists for stripping date suffixes. This keeps the fix localized and handles all callers.

### Fix 2: Remove injectLanguageModelAPIKey

**Location:** `api/cmd/settings-sync-daemon/main.go`

1. Delete the `injectLanguageModelAPIKey()` method entirely.
2. Remove calls to it in `syncFromHelix()` (line ~737) and `checkHelixUpdates()` (line ~1113).
3. Remove the `TestInjectLanguageModelAPIKey` test.

No replacement needed — `ANTHROPIC_API_KEY` env var already handles auth.

### Fix 3: Update hardcoded 128K fallback

**Location:** `api/cmd/settings-sync-daemon/main.go` → `injectAvailableModels()`

The `128000` default should be bumped to `200000` as a safer fallback since most current frontier models have at least 200K context. However, with Fix 1, this fallback should rarely be hit.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Where to normalize model names | `GetModelInfo()` in model_info.go | Single fix point for all callers; same pattern as existing `trimAnthropicDateSuffix` |
| What to do with api_key in settings.json | Remove entirely | Zed ignores it; writing tokens to disk is unnecessary exposure |
| Fallback context length | 200,000 (from 128,000) | Matches most frontier models; still conservative |

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/model/model_info.go` | Add dash-to-dot normalization in `GetModelInfo()` fallback |
| `api/pkg/model/model_info_test.go` | Add test for `claude-opus-4-6` (dashes) resolving correctly |
| `api/cmd/settings-sync-daemon/main.go` | Remove `injectLanguageModelAPIKey()`, update fallback default |
| `api/cmd/settings-sync-daemon/main_test.go` | Remove `TestInjectLanguageModelAPIKey`, update relevant tests |

## Codebase Patterns Discovered

- **model_info.json** is sourced from OpenRouter and embedded via `go:embed`. Model slugs use dots for versions (e.g. `anthropic/claude-opus-4.6`), but Anthropic's API uses dashes (`claude-opus-4-6`).
- **`trimAnthropicDateSuffix`** already exists as a model name normalization helper — the new normalization follows the same pattern.
- **Zed settings.json** has a specific schema — `api_key` is not part of any provider's settings struct. Auth is always env var or keychain.
- **`DesktopAgentAPIEnvVars()`** is the canonical place where container env vars for LLM auth are set. All code paths use it.
- **`CodeAgentConfig`** is the bridge between the API server (which looks up model info) and the settings-sync-daemon (which writes settings.json). Token limits flow through its `MaxTokens`/`MaxOutputTokens` fields.