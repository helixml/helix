# Requirements: Fix Context Length and API Key in Settings-Sync-Daemon

## Background

The settings-sync-daemon injects model configuration into Zed's `settings.json`. Three problems exist:

1. **Wrong context length**: Models like `claude-opus-4-6` show `max_tokens: 128000` in settings.json. The real context window is **200,000 tokens** (per Zed's built-in model definitions in `zed/crates/anthropic/src/anthropic.rs` — `ClaudeOpus4_6.max_token_count() = 200_000`). There is also an explicit 1M context variant (`claude-opus-4-6-1m-context`) that Zed selects separately as `ClaudeOpus4_6_1mContext`. The root cause is that `injectAvailableModels()` injects a `Custom` model entry into settings.json for models that Zed already has correct built-in definitions for. This Custom entry has a wrong 128K fallback context length (from a failed model_info.json lookup) and is missing cache config, beta headers, thinking mode support, etc. **The fix is to stop injecting built-in models entirely** — Zed already knows the right values.

2. **api_key in settings.json is dead config**: The daemon's `injectLanguageModelAPIKey()` writes `api_key` into `language_models.anthropic` in settings.json, but Zed's `AnthropicSettings` struct only has two fields: `api_url` and `available_models`. There is no `api_key` field — Zed reads API keys exclusively from the `ANTHROPIC_API_KEY` env var or the system keychain. The env var is already correctly set by `DesktopAgentAPIEnvVars()`. Writing the API key to settings.json leaks a token into a config file for no reason.

3. **`normalizeModelIDForZed` is missing 4.6 models**: The function in `zed_config.go` converts model names to `-latest` format for Zed's `default_model` config. It has cases for 4.5 models but no cases for 4.6 models (`claude-opus-4-6`, `claude-sonnet-4-6`). These should be added for consistency, even though Zed's `Model::from_id()` can resolve them via prefix matching today.

## User Stories

1. **As a user**, I want Zed to use its built-in model definitions (with correct 200K context, cache config, etc.) rather than a degraded Custom model injected by the settings-sync-daemon with wrong values.

2. **As a security-conscious operator**, I want API keys to NOT be written into settings.json, since Zed doesn't read them from there and it unnecessarily exposes tokens in a file.

## Acceptance Criteria

### Context Length Fix
- [ ] For models with native Zed provider support (anthropic), `injectAvailableModels()` does NOT inject into `available_models` — Zed's built-in definitions are used instead.
- [ ] For truly custom models (e.g. `helix/qwen3:8b` via OpenAI-compatible API), `injectAvailableModels()` still injects into `available_models` as before.
- [ ] The 128,000 hardcoded fallback default for custom models is updated to 200,000.

### Model ID Normalization
- [ ] `normalizeModelIDForZed()` handles `claude-opus-4-6*` → `claude-opus-4-6-latest` and `claude-sonnet-4-6*` → `claude-sonnet-4-6-latest`.
- [ ] The `default_model` in agent settings uses canonical `-latest` IDs that exactly match Zed's built-in model keys.

### API Key Removal from settings.json
- [ ] `injectLanguageModelAPIKey()` is removed from the settings-sync-daemon.
- [ ] `api_key` no longer appears in the generated settings.json.
- [ ] Zed still authenticates successfully via the `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` env vars (already set by `DesktopAgentAPIEnvVars()`).

### Non-Regression
- [ ] Existing tests in `main_test.go` are updated to reflect the changes.
- [ ] New test cases added for 4.6 model normalization.
- [ ] `go build` passes for the settings-sync-daemon and external-agent packages.
- [ ] E2E: starting a new session with an Anthropic model results in Zed using its built-in model definition (correct context length, cache config) with no `api_key` in settings.json, and LLM calls succeed.