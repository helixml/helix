# Configurable model for Claude subscription mode (default Opus)

## Summary

In Claude Code **subscription** mode, Helix never told Claude Code which model to use, so it fell back to its own built-in default (Sonnet). This adds a per-agent model choice and makes **Opus the default** for harder work.

The model is applied by injecting `ANTHROPIC_MODEL` into the sandbox container env in `subscriptionEnvForSession()` — the same layer that already sets `ANTHROPIC_BASE_URL`/`ANTHROPIC_API_KEY` for subscription sessions. Claude Code (launched via Zed/ACP inside the container) inherits it. This deliberately avoids Zed's ACP per-agent model plumbing, which is the wrong layer for a Helix container concern.

## Changes

- **`api/pkg/types/types.go`** — add `AssistantConfig.ClaudeSubscriptionModel`.
- **`api/pkg/server/external_agent_handlers.go`** — `subscriptionEnvForSession()` now appends `ANTHROPIC_MODEL=<model>`, defaulting to `claude-opus-4-6` when the field is empty. Only runs in claude_code + subscription mode, so api_key mode is unaffected.
- **Frontend** — a hardcoded Opus / Sonnet / Haiku dropdown for subscription mode in both the create form (`CodingAgentForm.tsx`) and the edit form (`AppSettings.tsx`), defaulting to Opus. `claude_subscription_model` is threaded through `types.ts`, `apps.tsx` (`createAgent`), `utils/app.ts` (flat-state), and `useApp.ts` (merge).
- **Generated artifacts** — regenerated OpenAPI client (`api.ts`, `swagger.yaml/json`, `openapi.json`, `docs.go`).
- **Tests** — `external_agent_handlers_subscription_model_test.go`: ANTHROPIC_MODEL defaults to Opus, honours an override, and is absent in api_key mode (3/3 passing).

## Verification

- `go build ./pkg/...` and `CGO_ENABLED=1 go test ./pkg/server/` (new tests) pass.
- `yarn tsc` passes.
- NOT run locally (no full inner-Helix stack in the build env): end-to-end create-session → `docker exec ... env | grep ANTHROPIC_MODEL`. Needs a reviewer with a live stack or CI to confirm the container actually receives the var.

## Model IDs

`claude-opus-4-6`, `claude-sonnet-4-5-latest`, `claude-haiku-4-5-latest` — match the existing `/api/v1/claude-subscriptions/models` endpoint.
