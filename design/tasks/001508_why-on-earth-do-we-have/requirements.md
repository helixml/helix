# Requirements: Fix Context Length and API Key in Settings-Sync-Daemon

## Background

The settings-sync-daemon injects model configuration into Zed's `settings.json`. Two problems exist:

1. **Wrong context length**: Models like `claude-opus-4-6` show `max_tokens: 128000` in settings.json, but the actual context window is 1,000,000 tokens. The 128K value is a hardcoded fallback default that kicks in when the model_info lookup fails. The lookup fails because model names use dashes (`claude-opus-4-6`) while model_info.json uses dots (`anthropic/claude-opus-4.6`).

2. **api_key in settings.json is useless**: The daemon writes `api_key` into `language_models.anthropic` in settings.json, but Zed's `AnthropicSettings` struct only reads `api_url` and `available_models` from settings. Zed reads API keys exclusively from the `ANTHROPIC_API_KEY` env var (or system keychain). The env var is already set correctly by `DesktopAgentAPIEnvVars()`. Writing the API key to settings.json is dead code that leaks a token into a config file for no reason.

## User Stories

1. **As a user**, I want the correct context window reported for my model so that Zed can use the model's full capability (e.g. 200K or 1M tokens for Claude models, not a truncated 128K).

2. **As a security-conscious operator**, I want API keys to NOT be written into settings.json, since Zed doesn't read them from there and it unnecessarily exposes tokens in a file.

## Acceptance Criteria

### Context Length Fix
- [ ] When the model_info lookup succeeds, `max_tokens` in `available_models` reflects the model's real `context_length` from model_info.json (e.g. 1,000,000 for claude-opus-4.6).
- [ ] When the model_info lookup succeeds, `max_output_tokens` in `available_models` reflects the model's real `max_completion_tokens` (e.g. 128,000 for claude-opus-4.6).
- [ ] The 128,000 hardcoded fallback default in `injectAvailableModels()` is either removed or updated to a more reasonable value (200,000 matches most current models).
- [ ] The model_info lookup in `buildCodeAgentConfigFromAssistant()` correctly resolves model names regardless of dash vs dot formatting (e.g. `claude-opus-4-6` should find `anthropic/claude-opus-4.6`).

### API Key Removal from settings.json
- [ ] `injectLanguageModelAPIKey()` is removed from the settings-sync-daemon.
- [ ] `api_key` no longer appears in the generated settings.json.
- [ ] Zed still authenticates successfully via the `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` env vars (already set by `DesktopAgentAPIEnvVars()`).

### Non-Regression
- [ ] Existing tests in `main_test.go` are updated to reflect the changes.
- [ ] `go build` passes for the settings-sync-daemon.
- [ ] E2E: starting a new session with an Anthropic model results in correct context length and successful LLM calls.