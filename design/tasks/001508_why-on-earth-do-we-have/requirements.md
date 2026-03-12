# Requirements: Fix Context Length and API Key in Settings-Sync-Daemon

## Background

The settings-sync-daemon injects model configuration into Zed's `settings.json`. Three problems exist:

1. **Wrong context length**: Models like `claude-opus-4-6` show `max_tokens: 128000` in settings.json. The real context window for claude-opus-4.6 is 1,000,000 tokens (per model_info.json from OpenRouter). The 128K value is a hardcoded fallback default that kicks in when the model_info lookup fails or returns 0. The lookup fails because model names use Anthropic's dash format (`claude-opus-4-6`) while model_info.json slugs use OpenRouter's dot format (`anthropic/claude-opus-4.6`), and no `provider_model_id` entry exists for the direct Anthropic endpoint of this model.

2. **api_key in settings.json is dead config**: The daemon's `injectLanguageModelAPIKey()` writes `api_key` into `language_models.anthropic` in settings.json, but Zed's `AnthropicSettings` struct (in `zed/crates/language_models/src/provider/anthropic.rs`) only has two fields: `api_url` and `available_models`. There is no `api_key` field — Zed reads API keys exclusively from the `ANTHROPIC_API_KEY` env var (via `ApiKeyState` in `zed/crates/language_model/src/api_key.rs`) or the system keychain. The env var is already correctly set by `DesktopAgentAPIEnvVars()` in `api/pkg/types/types.go`. Writing the API key to settings.json is dead code that leaks a token into a config file for no reason.

3. **The API server comment is misleading but correct**: The zed-config API handler in `zed_config_handlers.go` line 166 correctly says "api_key is NOT included here" and only sends `api_url`. But then the settings-sync-daemon re-injects `api_key` in its `injectLanguageModelAPIKey()` function, undoing that deliberate omission.

## User Stories

1. **As a user**, I want the correct context window reported for my model so that Zed can use the model's full capability (e.g. 1M tokens for claude-opus-4.6, 200K for claude-sonnet-4, not a truncated 128K).

2. **As a security-conscious operator**, I want API keys to NOT be written into settings.json, since Zed doesn't read them from there and it unnecessarily exposes tokens in a file.

## Acceptance Criteria

### Context Length Fix
- [ ] When the model_info lookup succeeds, `max_tokens` in `available_models` reflects the model's real `context_length` from model_info.json (e.g. 1,000,000 for claude-opus-4.6, 200,000 for claude-sonnet-4).
- [ ] When the model_info lookup succeeds, `max_output_tokens` in `available_models` reflects the model's real `max_completion_tokens`.
- [ ] The 128,000 hardcoded fallback default in `injectAvailableModels()` is updated to 200,000 (matches most current frontier models as a safer fallback).
- [ ] The model_info lookup in `GetModelInfo()` correctly resolves model names with dash-formatted version segments (e.g. `claude-opus-4-6` should find the same model as `anthropic/claude-opus-4.6`). This follows the same pattern as the existing `trimAnthropicDateSuffix` normalization.

### API Key Removal from settings.json
- [ ] `injectLanguageModelAPIKey()` is removed from the settings-sync-daemon.
- [ ] `api_key` no longer appears in the generated settings.json.
- [ ] Zed still authenticates successfully via the `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` env vars (already set by `DesktopAgentAPIEnvVars()`).

### Non-Regression
- [ ] Existing tests in `main_test.go` are updated to reflect the changes.
- [ ] New tests added in `model_info_test.go` for dash-format model name resolution.
- [ ] `go build` passes for the settings-sync-daemon and model package.
- [ ] E2E: starting a new session with an Anthropic model results in correct context length in settings.json and successful LLM calls.