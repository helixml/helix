# Implementation Tasks

## Fix 1: Model name normalization in GetModelInfo

- [ ] Add a `normalizeAnthropicVersionDashes` helper function in `api/pkg/model/model_info.go` that converts trailing dash-separated version segments to dots (e.g. `anthropic/claude-opus-4-6` → `anthropic/claude-opus-4.6`). Use a regex like `-(\d+)-(\d+)$` → `-$1.$2`. Follow the same pattern as the existing `trimAnthropicDateSuffix` helper.
- [ ] In `GetModelInfo()`, after the existing slug iteration loop fails (just before the `return nil, fmt.Errorf(...)` line), try the normalized slug as a final fallback: apply `normalizeAnthropicVersionDashes` to the slug and iterate models again comparing against `model.Slug` and `model.Permaslug`.
- [ ] Add unit tests in `api/pkg/model/model_info_test.go`:
  - `claude-opus-4-6` (dashes, no prefix) with provider `anthropic` should resolve to the same `ModelInfo` as `anthropic/claude-opus-4.6`, with correct `ContextLength` and `Pricing`.
  - `claude-sonnet-4-5` should resolve to `anthropic/claude-sonnet-4.5`.
  - `claude-sonnet-4-6` should resolve to `anthropic/claude-sonnet-4.6`.
  - Models without version dots (e.g. `claude-sonnet-4-20250514`) should still be handled by the existing `trimAnthropicDateSuffix` and not be affected by the new normalization.
- [ ] Verify: `cd api && go test -v -run "Test_Get" ./pkg/model/ -count=1`

## Fix 2: Remove `injectLanguageModelAPIKey` from settings-sync-daemon

- [ ] Delete the `injectLanguageModelAPIKey()` method from `api/cmd/settings-sync-daemon/main.go` (lines 246–264).
- [ ] Remove the call `d.injectLanguageModelAPIKey()` in `syncFromHelix()` (around line 737).
- [ ] Remove the call `d.injectLanguageModelAPIKey()` in `checkHelixUpdates()` (around line 1113).
- [ ] Delete `TestInjectLanguageModelAPIKey` from `api/cmd/settings-sync-daemon/main_test.go` (if it exists).
- [ ] Search for any other tests asserting `api_key` presence in settings output and update them to assert absence instead.
- [ ] Verify: `cd api && go build ./cmd/settings-sync-daemon/ && go test ./cmd/settings-sync-daemon/ -count=1`

## Fix 3: Update hardcoded fallback default in `injectAvailableModels`

- [ ] In `injectAvailableModels()` in `api/cmd/settings-sync-daemon/main.go` (around line 298), change `128000` to `200000`.
- [ ] Update the comment to: `// Default context window if not found in model_info (200K matches most current frontier models)`.
- [ ] Verify: `cd api && go build ./cmd/settings-sync-daemon/`

## Verification

- [ ] Run `cd api && go build ./pkg/model/ ./cmd/settings-sync-daemon/` — compilation passes.
- [ ] Run `cd api && go test ./pkg/model/ -count=1` — model lookup tests pass including new dash-format tests.
- [ ] Run `cd api && go test ./cmd/settings-sync-daemon/ -count=1` — daemon tests pass without `api_key` injection.
- [ ] Deploy and start a new session with `claude-opus-4-6` — inspect `~/.config/zed/settings.json` inside the container and verify:
  - `max_tokens` in `available_models` matches the model's real context length from model_info.json (1,000,000 for claude-opus-4.6).
  - No `api_key` field exists under `language_models.anthropic`.
- [ ] Verify LLM calls still work — the `ANTHROPIC_API_KEY` env var (set by `DesktopAgentAPIEnvVars`) provides authentication.