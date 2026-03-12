# Design: Fix Context Length and API Key in Settings-Sync-Daemon

## Root Cause Analysis

### Problem 1: Wrong context length (128K instead of real value)

**Chain of events:**

1. `buildCodeAgentConfigFromAssistant()` in `zed_config_handlers.go` looks up model info:
   ```go
   modelInfo, err := apiServer.modelInfoProvider.GetModelInfo(ctx, &modelPkg.ModelInfoRequest{
       Provider: providerName,  // "anthropic"
       Model:    modelName,     // e.g. "claude-opus-4-6" (Anthropic's dash format)
   })
   if err == nil {
       maxTokens = modelInfo.ContextLength
       maxOutputTokens = modelInfo.MaxCompletionTokens
   }
   ```

2. `BaseModelInfoProvider.GetModelInfo()` (in `api/pkg/model/model_info.go`) tries several lookup strategies:
   - Direct map lookup by `provider_model_id` — fails because the map is keyed by provider-specific IDs like `global.anthropic.claude-opus-4-6-v1` (Bedrock), not `claude-opus-4-6`.
   - Strip prefix and retry — `claude-opus-4-6` still not in the map.
   - Construct slug `anthropic/claude-opus-4-6` and iterate all models comparing against `model.Slug` (`anthropic/claude-opus-4.6` — dots) and `model.Permaslug` (`anthropic/claude-4.6-opus-20260205`). **None match** because dashes ≠ dots.
   - `trimAnthropicDateSuffix` only strips trailing `-YYYYMMDD` patterns — doesn't help here since `4-6` is not a date suffix.

3. `GetModelInfo` returns an error → `maxTokens` stays 0 → goes into `CodeAgentConfig.MaxTokens` as 0.

4. In the settings-sync-daemon's `injectAvailableModels()` (`api/cmd/settings-sync-daemon/main.go`):
   ```go
   maxTokens := d.codeAgentConfig.MaxTokens
   if maxTokens == 0 {
       maxTokens = 128000 // Hardcoded fallback — THIS IS THE BUG
   }
   ```

5. Zed's `AnthropicAvailableModel.max_tokens` field means **context window size** (the comment in `zed/crates/settings_content/src/language_model.rs` says "The model's context window size"). So Zed thinks the model has a 128K context window when it should be 1,000,000 (for claude-opus-4.6) or 200,000 (for most other Claude models).

**Actual context lengths from model_info.json (OpenRouter source):**
- `anthropic/claude-opus-4.6`: context_length=1,000,000
- `anthropic/claude-opus-4.5`: context_length=200,000
- `anthropic/claude-opus-4`: context_length=200,000
- `anthropic/claude-sonnet-4`: context_length=200,000
- `anthropic/claude-sonnet-4.5`: context_length=200,000
- `anthropic/claude-sonnet-4.6`: context_length=200,000

### Problem 2: api_key written to settings.json is dead config

**How Zed reads API keys (priority order in `ApiKeyState.load_if_needed()` in `zed/crates/language_model/src/api_key.rs`):**
1. Check env var (`ANTHROPIC_API_KEY`) → if non-empty, use it immediately (returns `Task::ready(Ok(()))`)
2. Fall back to system keychain lookup via `CredentialsProvider`

**Zed's `AnthropicSettings` struct (`zed/crates/language_models/src/provider/anthropic.rs`) only has two fields:**
```rust
pub struct AnthropicSettings {
    pub api_url: String,
    pub available_models: Vec<AvailableModel>,
}
```
There is **no `api_key` field**. Any `api_key` in settings.json is silently ignored by Zed's settings deserialization.

**The env var is already set correctly** by `DesktopAgentAPIEnvVars()` in `api/pkg/types/types.go`:
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

### Problem 3: Contradictory intent between API handler and daemon

The zed-config API handler (`zed_config_handlers.go` line 166) deliberately omits `api_key`:
```go
// api_key is NOT included here — Zed reads ANTHROPIC_API_KEY / OPENAI_API_KEY from
// container env vars (set by DesktopAgentAPIEnvVars). Only api_url is needed in settings.
```

But then the settings-sync-daemon's `injectLanguageModelAPIKey()` re-injects it, undoing that deliberate omission. This is dead code that contradicts the API handler's security-conscious design.

## Solution Design

### Fix 1: Model name normalization in GetModelInfo

**Location:** `api/pkg/model/model_info.go` → `GetModelInfo()`

The core issue: Anthropic model IDs use dashes for version numbers (`claude-opus-4-6`) but OpenRouter slugs use dots (`anthropic/claude-opus-4.6`). The existing `trimAnthropicDateSuffix` helper already handles one class of normalization (stripping `-YYYYMMDD`). We need a similar normalization for dash-vs-dot version segments.

**Approach:** After existing slug-based lookups fail, apply a normalization that converts trailing version-like dash segments to dots. The pattern is: when the model name ends with segments like `-4-6` or `-4-5`, try replacing the last separating dash with a dot (e.g. `claude-opus-4-6` → `claude-opus-4.6`). This is specifically for the pattern where a model family version and a minor version are both numeric and dash-separated.

A reasonable regex: match `-(\d+)-(\d+)` at the end of the model name portion of the slug and try replacing with `-$1.$2`. This converts `anthropic/claude-opus-4-6` → `anthropic/claude-opus-4.6`.

This follows the same pattern as `trimAnthropicDateSuffix` — a targeted normalization that fires as a fallback only when exact matches fail.

### Fix 2: Remove injectLanguageModelAPIKey

**Location:** `api/cmd/settings-sync-daemon/main.go`

1. Delete the `injectLanguageModelAPIKey()` method entirely.
2. Remove calls to it in `syncFromHelix()` (line ~737) and `checkHelixUpdates()` (line ~1113).
3. Remove the corresponding test.

No replacement needed — `ANTHROPIC_API_KEY` env var already handles auth correctly.

### Fix 3: Update hardcoded 128K fallback

**Location:** `api/cmd/settings-sync-daemon/main.go` → `injectAvailableModels()`

Change the fallback from `128000` to `200000`. This is a safer default since most current frontier models have at least 200K context. With Fix 1 in place, this fallback should rarely be hit — only for models completely absent from model_info.json.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Where to normalize model names | `GetModelInfo()` in `model_info.go` | Single fix point for all callers; same pattern as existing `trimAnthropicDateSuffix` |
| Normalization strategy | Regex-based dash-to-dot for trailing version segments | Targeted and safe — only fires when exact lookups fail, only affects numeric version patterns |
| What to do with `api_key` in settings.json | Remove `injectLanguageModelAPIKey()` entirely | Zed ignores the field; writing tokens to disk is unnecessary exposure; contradicts the API handler's deliberate omission |
| Fallback context length | 200,000 (from 128,000) | Matches most frontier models; conservative; rarely hit after Fix 1 |

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/model/model_info.go` | Add dash-to-dot version normalization in `GetModelInfo()` as a fallback, following the `trimAnthropicDateSuffix` pattern |
| `api/pkg/model/model_info_test.go` | Add test for `claude-opus-4-6` (dashes) resolving correctly to the same ModelInfo as `anthropic/claude-opus-4.6` |
| `api/cmd/settings-sync-daemon/main.go` | Delete `injectLanguageModelAPIKey()` method and its two call sites; update `128000` fallback to `200000` |
| `api/cmd/settings-sync-daemon/main_test.go` | Delete `TestInjectLanguageModelAPIKey`; update any tests that assert `api_key` presence in output |

## Codebase Patterns Discovered

- **model_info.json** is sourced from OpenRouter and embedded via `go:embed`. Model slugs use dots for versions (e.g. `anthropic/claude-opus-4.6`). The data map is keyed by `provider_model_id` (e.g. `global.anthropic.claude-opus-4-6-v1` for Bedrock, `claude-sonnet-4-6` for Anthropic direct). Not all models have an Anthropic-direct endpoint entry — claude-opus-4.6 only has a Bedrock endpoint in the current data.
- **`trimAnthropicDateSuffix`** already exists as a model name normalization helper in `model_info.go` — the new normalization follows the same fallback pattern.
- **Zed settings.json schema** is strict — `api_key` is not part of any provider's settings struct (`AnthropicSettings`, `OpenAiSettings`). Auth is always env var or system keychain. The `available_models[].max_tokens` field means **context window size**, not max output tokens.
- **`DesktopAgentAPIEnvVars()`** in `api/pkg/types/types.go` is the canonical place where container env vars for LLM auth are set. All code paths (`addUserAPITokenToAgent`, `buildEnvWithLocale`, `GoldenBuildService`) use it consistently.
- **`CodeAgentConfig`** is the bridge between the API server (which looks up model info) and the settings-sync-daemon (which writes settings.json). Token limits flow through its `MaxTokens`/`MaxOutputTokens` fields. When these are 0, the daemon falls back to hardcoded defaults.
- **The zed-config API handler** deliberately omits `api_key` from the response (comment at line 166 of `zed_config_handlers.go`), but the daemon re-injects it — this is the inconsistency being fixed.