# Implementation Tasks: Subscription Mode Should Default to Latest Opus, Not Opus 4.6

## Verify — confirm shorthand resolution works

- [ ] In a real subscription container, write `{"model": "opus"}` to `/etc/claude-code/managed-settings.json` and confirm Claude Code resolves it to `claude-opus-4-8` (check the model in Claude Code's `/doctor` or session startup logs)
- [ ] Also verify `"sonnet"` → `claude-sonnet-4-6` and `"haiku"` → `claude-haiku-4-5`
- [ ] If shorthand resolution doesn't work, fall back to the version-bump approach (change `"opus"` to `"claude-opus-4-8"` everywhere below)

(These verification tasks require a live subscription container — will be done post-merge during QA.)

## Backend — switch to shorthand identifiers

- [~] Update default in `api/pkg/server/zed_config_handlers.go:677` — change `"claude-opus-4-6"` to `"opus"`
- [ ] Update `listClaudeModels` in `api/pkg/server/claude_subscription_handlers.go:303-307` — change model IDs to `"opus"`, `"sonnet"`, `"haiku"` and names to `"Claude Opus"`, `"Claude Sonnet"`, `"Claude Haiku"` (drop version numbers)
- [ ] Update doc comment on `ClaudeSubscriptionModel` in `api/pkg/types/types.go:1570` — change `"claude-opus-4-6"` to `"opus"`

## Backend — fix normalizer (API-key path)

- [ ] Add `claude-opus-4-8` and `claude-opus-4-7` prefix entries to `normalizeModelIDForZed` in `api/pkg/external-agent/zed_config.go` (before the generic `claude-opus-4` catch-all at line 562)
- [ ] Add test cases for `claude-opus-4-7` → `claude-opus-4-7-latest` and `claude-opus-4-8` → `claude-opus-4-8-latest` in `api/pkg/external-agent/zed_config_test.go`

## Frontend — update to tier-level labels

- [ ] In `CodingAgentForm.tsx`: change `CLAUDE_SUBSCRIPTION_MODELS` IDs to `"opus"`, `"sonnet"`, `"haiku"` and labels to `"Claude Opus (recommended)"`, `"Claude Sonnet"`, `"Claude Haiku"`; change `DEFAULT_CLAUDE_SUBSCRIPTION_MODEL` to `"opus"`
- [ ] In `AppSettings.tsx`: update the subscription model reference to use `"opus"` as default
- [ ] Update `DEFAULT_ONBOARDING_AGENT_MODEL` in `frontend/src/pages/Onboarding.tsx:62` from `claude-opus-4-6` to `claude-opus-4-8` (this is the API-key path — needs a concrete ID, not a shorthand)

## Tests

- [ ] Update `api/pkg/server/zed_config_handlers_test.go` — expected default model from `claude-opus-4-6` to `opus`
- [ ] Update `api/cmd/settings-sync-daemon/main_test.go:63` — test fixture model from `claude-opus-4-6` to `opus`
- [ ] Regenerate OpenAPI (`./stack update_openapi`) to update doc comment in generated files

## Build verification

- [ ] `go build ./pkg/...` and `CGO_ENABLED=1 go test ./pkg/server/ ./pkg/external-agent/` pass
- [ ] `cd frontend && yarn tsc && yarn build` pass
