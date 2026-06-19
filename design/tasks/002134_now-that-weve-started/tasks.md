# Implementation Tasks: Configurable Model Selection for Claude Subscription Mode

- [x] Add `ClaudeSubscriptionModel string` field to `AssistantConfig` in `api/pkg/types/types.go` (after `CodeAgentCredentialType`, with `json:"claude_subscription_model,omitempty"`)
- [x] In `subscriptionEnvForSession()` in `api/pkg/server/external_agent_handlers.go`, read `asst.ClaudeSubscriptionModel` and append `ANTHROPIC_MODEL=<value>` (defaulting to `claude-opus-4-6`) to the env slice
- [x] Add `claude_subscription_model` to the `IAssistantConfig` interface and related types in `frontend/src/types.ts`
- [x] Add `claudeSubscriptionModel?: string` to `ICreateAgentParams` in `frontend/src/contexts/apps.tsx` and map it to `claude_subscription_model` in `createAgent()`
- [x] Update `frontend/src/utils/app.ts` (`getAppFlatState`) to include `claude_subscription_model` in the flat-state mapping
- [x] In `frontend/src/hooks/useApp.ts` (`mergeFlatStateIntoApp`), apply `claude_subscription_model` updates to `assistants[0]`
- [x] In `frontend/src/components/agent/CodingAgentForm.tsx` (create path), add a model `<Select>` visible when `isClaudeCodeSubscription` with three hardcoded options (`claude-opus-4-6`, `claude-sonnet-4-5-latest`, `claude-haiku-4-5-latest`, default Opus), held in local state and passed into `ICreateAgentParams` in `handleCreateAgent`. Exported `CLAUDE_SUBSCRIPTION_MODELS` + `DEFAULT_CLAUDE_SUBSCRIPTION_MODEL` for reuse.
- [x] In `frontend/src/components/app/AppSettings.tsx` (edit path), add the same dropdown for subscription mode, initialized from `app.claude_subscription_model` (fallback Opus), persisted via `onUpdate({ claude_subscription_model })`
- [x] Regenerate the OpenAPI client (`./stack update_openapi`) so `claude_subscription_model` appears in `frontend/src/api/api.ts`, `swagger.yaml/json`, `openapi.json`, `docs.go`
- [x] Verify Go build (`go build ./pkg/...`), Go tests (`CGO_ENABLED=1 go test ./pkg/server/`), frontend typecheck (`yarn tsc`), and full frontend build (`yarn build` / vite) all pass
- [x] Unit test: `subscriptionEnvForSession` injects `ANTHROPIC_MODEL=claude-opus-4-6` by default, honours an override (`claude-haiku-4-5-latest`), and emits nothing in api_key mode (`external_agent_handlers_subscription_model_test.go`, 3/3 passing with `CGO_ENABLED=1 go test ./pkg/server/`)
- [ ] NOT RUN (no running stack in this env — API :8080 down, only Vite :8081 up): e2e — create/edit a Claude Code subscription agent, confirm Opus preselected, start a session, `docker exec ... env | grep ANTHROPIC_MODEL` shows the chosen model. Needs verification by a reviewer with a live inner-Helix stack or in CI.
- [ ] NOT RUN (same reason): e2e — change dropdown to Haiku, start a session, confirm `ANTHROPIC_MODEL=claude-haiku-4-5-latest` in container env
- [x] Regression guard: api_key mode emits no `ANTHROPIC_MODEL` (covered by unit test above); the create/edit model picker for api_key mode is untouched (dropdown only renders when `claudeCodeMode === 'subscription'`)
