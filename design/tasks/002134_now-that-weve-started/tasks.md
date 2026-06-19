# Implementation Tasks: Configurable Model Selection for Claude Subscription Mode

- [x] Add `ClaudeSubscriptionModel string` field to `AssistantConfig` in `api/pkg/types/types.go` (after `CodeAgentCredentialType`, with `json:"claude_subscription_model,omitempty"`)
- [x] In `subscriptionEnvForSession()` in `api/pkg/server/external_agent_handlers.go`, read `asst.ClaudeSubscriptionModel` and append `ANTHROPIC_MODEL=<value>` (defaulting to `claude-opus-4-6`) to the env slice
- [x] Add `claude_subscription_model` to the `IAssistantConfig` interface and related types in `frontend/src/types.ts`
- [x] Add `claudeSubscriptionModel?: string` to `ICreateAgentParams` in `frontend/src/contexts/apps.tsx` and map it to `claude_subscription_model` in `createAgent()`
- [x] Update `frontend/src/utils/app.ts` (`getAppFlatState`) to include `claude_subscription_model` in the flat-state mapping
- [x] In `frontend/src/hooks/useApp.ts` (`mergeFlatStateIntoApp`), apply `claude_subscription_model` updates to `assistants[0]`
- [~] In `frontend/src/components/agent/CodingAgentForm.tsx` (create path), add a model `<Select>` visible when `isClaudeCodeSubscription` with three hardcoded options (`claude-opus-4-6`, `claude-sonnet-4-5-latest`, `claude-haiku-4-5-latest`, default Opus), held in local state and passed into `ICreateAgentParams` in `handleCreateAgent`. (Local state, not shared `value` — parents don't spread `nextValue`; see design.md.)
- [ ] In `frontend/src/components/app/AppSettings.tsx` (edit path), add the same dropdown for subscription mode, initialized from `app.claude_subscription_model` (fallback Opus), persisted via `onUpdate({ claude_subscription_model })`
- [ ] Regenerate the OpenAPI client (`./stack update_openapi`) so `claude_subscription_model` appears in `frontend/src/api/api.ts`
- [ ] Test: create or edit a Claude Code subscription agent, confirm Opus is preselected, save, start a session, verify `ANTHROPIC_MODEL=claude-opus-4-6` is set in the container (`docker exec ... env | grep ANTHROPIC_MODEL`)
- [ ] Test: change dropdown to Haiku, save, start a session, verify `ANTHROPIC_MODEL=claude-haiku-4-5-latest` in container env
- [ ] Test: API key mode agent — confirm no regression (model picker still works, no new fields shown)
