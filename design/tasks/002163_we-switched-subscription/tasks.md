# Implementation Tasks: Subscription Mode Should Default to Latest Opus, Not Opus 4.6

## Verify — confirm shorthand resolution works

- [ ] In a real subscription container, write `{"model": "opus"}` to `/etc/claude-code/managed-settings.json` and confirm Claude Code resolves it to `claude-opus-4-8` (check the model in Claude Code's `/doctor` or session startup logs)
- [ ] Also verify `"sonnet"` → `claude-sonnet-4-6` and `"haiku"` → `claude-haiku-4-5`
- [ ] If shorthand resolution doesn't work, fall back to the version-bump approach (change `"opus"` to `"claude-opus-4-8"` everywhere below)

(These verification tasks require a live subscription container — will be done post-merge during QA.)

## Backend — switch to shorthand identifiers

- [x] Update default in `api/pkg/server/zed_config_handlers.go:677` — change `"claude-opus-4-6"` to `"opus"`
- [x] Update `listClaudeModels` in `api/pkg/server/claude_subscription_handlers.go:303-307` — change model IDs to `"opus"`, `"sonnet"`, `"haiku"` and names to `"Claude Opus"`, `"Claude Sonnet"`, `"Claude Haiku"` (drop version numbers)
- [x] Update doc comment on `ClaudeSubscriptionModel` in `api/pkg/types/types.go:1570` — change `"claude-opus-4-6"` to `"opus"`
- [x] Update settings-sync-daemon comment to document shorthand resolution

## Backend — fix normalizer (API-key path)

- [x] Add `claude-opus-4-8` and `claude-opus-4-7` prefix entries to `normalizeModelIDForZed` in `api/pkg/external-agent/zed_config.go` (before the generic `claude-opus-4` catch-all at line 562)
- [x] Add test cases for `claude-opus-4-7` → `claude-opus-4-7-latest` and `claude-opus-4-8` → `claude-opus-4-8-latest` in `api/pkg/external-agent/zed_config_test.go`

## Frontend — update to tier-level labels

- [x] In `CodingAgentForm.tsx`: change `CLAUDE_SUBSCRIPTION_MODELS` IDs to `"opus"`, `"sonnet"`, `"haiku"` and labels to `"Claude Opus (recommended)"`, `"Claude Sonnet"`, `"Claude Haiku"`; change `DEFAULT_CLAUDE_SUBSCRIPTION_MODEL` to `"opus"`
- [x] In `AppSettings.tsx`: no changes needed — imports from CodingAgentForm, automatically picks up the new constants
- [x] Update `DEFAULT_ONBOARDING_AGENT_MODEL` in `frontend/src/pages/Onboarding.tsx:62` from `claude-opus-4-6` to `claude-opus-4-8` (this is the API-key path — needs a concrete ID, not a shorthand)
- [x] Add `claude-opus-4-8` to `RECOMMENDED_CODING_MODELS` in `frontend/src/constants/models.ts` as first entry

## Tests

- [x] Update `api/pkg/server/zed_config_handlers_test.go` — expected default model from `claude-opus-4-6` to `opus`
- [x] `api/cmd/settings-sync-daemon/main_test.go:63` — no change needed (tests Zed injection skip logic, not subscription default)
- [ ] Regenerate OpenAPI (`./stack update_openapi`) to update doc comment in generated files — `./stack` not available in this environment, will be done post-merge

## Build verification

- [x] `CGO_ENABLED=0 go build ./api/pkg/...` passes
- [x] `CGO_ENABLED=0 go test ./api/pkg/server/ ./api/pkg/external-agent/` — all tests pass
- [ ] `cd frontend && yarn tsc && yarn build` — TypeScript compiler not available in this environment

## End-to-end verification (local dev stack)

- [x] Models endpoint returns tier-level shorthand: `{"id":"opus",...}`, `{"id":"sonnet",...}`, `{"id":"haiku",...}`
- [x] Subscription dropdown shows "Claude Opus (recommended)", "Claude Sonnet", "Claude Haiku"
- [x] Creating a project with subscription mode stores `claude_subscription_model: "opus"` in DB
- [ ] Live container test: verify Claude Code's `resolveModelPreference()` resolves `"opus"` → `claude-opus-4-8` (requires running session)
