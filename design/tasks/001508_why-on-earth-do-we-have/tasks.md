# Implementation Tasks

## Fix 1: Stop injecting built-in models into `available_models`

- [x] In `injectAvailableModels()` in `api/cmd/settings-sync-daemon/main.go`, add a check at the top: if the model's provider has native Zed support (i.e. `APIType == "anthropic"`), skip injection entirely and return early. Zed already has built-in definitions for all Claude models with correct context lengths, cache config, beta headers, thinking mode, etc. Injecting a `Custom` model from `available_models` is strictly worse.
- [x] For non-built-in models (e.g. `helix/qwen3:8b` via OpenAI-compatible API), keep the existing injection logic — these truly need `available_models` entries for Zed to recognize them.
- [x] Change the hardcoded fallback from `128000` to `200000` (for the custom model path). Update the comment to: `// Default context window for custom models if not found in model_info (200K matches most current frontier models)`.
- [x] Verify: `cd api && go build ./cmd/settings-sync-daemon/`

## Fix 2: Complete `normalizeModelIDForZed` for all Anthropic model IDs

The actual model IDs from Anthropic's `/v1/models` endpoint are: `claude-sonnet-4-6`, `claude-opus-4-6`, `claude-opus-4-5-20251101`, `claude-haiku-4-5-20251001`, `claude-sonnet-4-5-20250929`, `claude-opus-4-1-20250805`, `claude-opus-4-20250514`, `claude-sonnet-4-20250514`, `claude-3-haiku-20240307`. These are the model IDs the user selects in the Helix UI.

- [x] In `normalizeModelIDForZed()` in `api/pkg/external-agent/zed_config.go`, add the missing cases. Order matters — more specific prefixes must come before less specific ones to avoid false matches:
  - `claude-opus-4-6*` → `claude-opus-4-6-latest` (NEW)
  - `claude-sonnet-4-6*` → `claude-sonnet-4-6-latest` (NEW)
  - `claude-opus-4-1*` → `claude-opus-4-1-latest` (NEW)
  - `claude-opus-4*` (after 4-1, 4-5, 4-6 checks) → `claude-opus-4-latest` (NEW — handles `claude-opus-4-20250514`)
  - `claude-sonnet-4*` (after 4-5, 4-6 checks) → `claude-sonnet-4-latest` (NEW — handles `claude-sonnet-4-20250514`)
  - (existing 4.5 and 3.x cases remain unchanged)
- [x] Add corresponding test cases in `api/pkg/external-agent/zed_config_test.go` covering every Anthropic model ID listed above.
- [x] Verify: `cd api && go test ./pkg/external-agent/ -count=1 -run Normalize`

## Fix 3: Remove `injectLanguageModelAPIKey` from settings-sync-daemon

- [~] Delete the `injectLanguageModelAPIKey()` method from `api/cmd/settings-sync-daemon/main.go` (lines 246–264).
- [~] Remove the call `d.injectLanguageModelAPIKey()` in `syncFromHelix()` (around line 737).
- [~] Remove the call `d.injectLanguageModelAPIKey()` in `checkHelixUpdates()` (around line 1113).
- [~] Delete any test for `injectLanguageModelAPIKey` from `api/cmd/settings-sync-daemon/main_test.go`.
- [~] Search for any other tests asserting `api_key` presence in settings output and update them to assert absence instead.
- [ ] Verify: `cd api && go build ./cmd/settings-sync-daemon/ && go test ./cmd/settings-sync-daemon/ -count=1`

## Verification

- [ ] Run `cd api && go build ./cmd/settings-sync-daemon/ ./pkg/external-agent/` — compilation passes.
- [ ] Run `cd api && go test ./cmd/settings-sync-daemon/ -count=1` — daemon tests pass.
- [ ] Run `cd api && go test ./pkg/external-agent/ -count=1` — zed_config tests pass including new 4.6 normalization cases.
- [ ] Deploy and start a new session with `claude-opus-4-6` — inspect `~/.config/zed/settings.json` inside the container and verify:
  - No `claude-opus-4-6` entry exists in `available_models` under `language_models.anthropic` (Zed uses its built-in definition instead).
  - No `api_key` field exists under `language_models.anthropic`.
  - `agent.default_model` uses `claude-opus-4-6-latest` (from the `normalizeModelIDForZed` fix).
- [ ] Verify Zed shows the model with correct 200K context window (from its built-in definition, not a degraded Custom model).
- [ ] Verify LLM calls still work — `ANTHROPIC_API_KEY` env var provides authentication.
- [ ] Start a session with a non-built-in model (e.g. `helix/qwen3:8b`) and verify it IS injected into `available_models` as before.