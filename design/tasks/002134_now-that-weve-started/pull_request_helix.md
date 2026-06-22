# Configurable model for Claude subscription mode (default Opus)

## Summary

In Claude Code **subscription** mode, Helix never told Claude Code which model to use, so it fell back to its built-in default (Sonnet). This makes **Opus the default** and adds a per-agent model picker (Opus / Sonnet / Haiku).

The model flows through the existing, documented pathway: `buildCodeAgentConfigFromAssistant` sets `CodeAgentConfig.Model`, the settings-sync-daemon writes it to the container's `/etc/claude-code/managed-settings.json`, and the `claude-agent-acp` package reads it via `resolveModelPreference()`. (An earlier iteration injected `ANTHROPIC_MODEL`, but that's the wrong layer — the adapter resolves its model from managed-settings, not that env var — so it was reverted.)

## Changes

- **`api/pkg/types/types.go`** — add `AssistantConfig.ClaudeSubscriptionModel`.
- **`api/pkg/server/zed_config_handlers.go`** — in the `claude_code` + subscription branch of `buildCodeAgentConfigFromAssistant`, set `Model` to `ClaudeSubscriptionModel`, defaulting to `claude-opus-4-6`. (api_key mode unchanged.)
- **`api/cmd/settings-sync-daemon/main.go`** — guard `injectAvailableModels()` to skip `claude_code` (it uses managed-settings, not Zed `language_models`; without the guard, the now-set model would be injected as a bogus `openai` Custom model in subscription mode where `APIType` is empty).
- **Frontend** — hardcoded Opus / Sonnet / Haiku dropdown for subscription mode in the create form (`CodingAgentForm.tsx`) and edit form (`AppSettings.tsx`), defaulting to Opus. `claude_subscription_model` threaded through `types.ts`, `apps.tsx`, `utils/app.ts`, `useApp.ts`.
- **`Dockerfile.qwen-code-build`** — copy root `tsconfig.json` before the qwen workspace build (it extends `../../tsconfig.json`; without it the build fails `TS5083`). Unblocks `./stack build-ubuntu`. *(Drive-by fix for a pre-existing main bug; can be split into its own PR if preferred.)*
- Regenerated OpenAPI client; backend unit tests.

## Verification

- `go build ./cmd/... ./pkg/...`, `CGO_ENABLED=1 go test ./pkg/server/` (subscription→Opus default, Haiku override, api_key untouched), and `yarn tsc` / `yarn build` all pass.
- **Live, end-to-end in the inner Helix** with a real Claude subscription token: created a `claude_code`+subscription agent (no explicit model), started a real session container, and confirmed:
  - `/etc/claude-code/managed-settings.json` → `{"model":"claude-opus-4-6"}`
  - Zed `agent_servers.claude-acp.default_model` → `"claude-opus-4-6"`
- Confirmed the three model IDs match the live `/api/v1/claude-subscriptions/models` endpoint.

## Model IDs

`claude-opus-4-6`, `claude-sonnet-4-5-latest`, `claude-haiku-4-5-latest`.
