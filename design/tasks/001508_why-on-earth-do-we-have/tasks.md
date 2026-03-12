# Implementation Tasks

## Fix 1: Model name normalization in GetModelInfo

- [ ] Add a dash-to-dot normalization function in `api/pkg/model/model_info.go` that converts version-like segments from dashes to dots (e.g. `claude-opus-4-6` → `claude-opus-4.6`, `claude-sonnet-4-5` → `claude-sonnet-4.5`). Follow the same pattern as the existing `trimAnthropicDateSuffix` helper.
- [ ] In `GetModelInfo()`, after existing slug lookups fail, try the normalized slug as a final fallback before returning "not found".
- [ ] Add unit tests in `api/pkg/model/model_info_test.go` for dash-format model names: `claude-opus-4-6` should resolve to the same `ModelInfo` as `anthropic/claude-opus-4.6`, with correct `ContextLength` (1,000,000) and `MaxCompletionTokens` (128,000).
- [ ] Verify `go build ./pkg/model/` and `go test ./pkg/model/` pass.

## Fix 2: Remove injectLanguageModelAPIKey from settings-sync-daemon

- [ ] Delete the `injectLanguageModelAPIKey()` method from `api/cmd/settings-sync-daemon/main.go`.
- [ ] Remove the call to `d.injectLanguageModelAPIKey()` in `syncFromHelix()` (~line 737).
- [ ] Remove the call to `d.injectLanguageModelAPIKey()` in `checkHelixUpdates()` (~line 1113).
- [ ] Delete `TestInjectLanguageModelAPIKey` from `api/cmd/settings-sync-daemon/main_test.go`.
- [ ] Verify `go build ./cmd/settings-sync-daemon/` passes.

## Fix 3: Update hardcoded fallback default

- [ ] In `injectAvailableModels()` in `api/cmd/settings-sync-daemon/main.go`, change the fallback from `128000` to `200000` and update the comment to clarify it's a last-resort default.

## Verification

- [ ] Run `go build ./pkg/model/ ./cmd/settings-sync-daemon/` to confirm compilation.
- [ ] Run `go test ./pkg/model/ -count=1` to confirm model lookup tests pass.
- [ ] Run `go test ./cmd/settings-sync-daemon/ -count=1` to confirm daemon tests pass.
- [ ] Deploy and start a new session with an Anthropic model — verify settings.json has correct `max_tokens` matching the model's real context length and no `api_key` field.
- [ ] Verify LLM calls still work (auth via env var is unaffected).