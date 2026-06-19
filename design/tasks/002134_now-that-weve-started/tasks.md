# Implementation Tasks: Configurable Model Selection for Claude Subscription Mode

- [x] Add `ClaudeSubscriptionModel string` field to `AssistantConfig` in `api/pkg/types/types.go` (after `CodeAgentCredentialType`, with `json:"claude_subscription_model,omitempty"`)
- [~] In `subscriptionEnvForSession()` in `api/pkg/server/external_agent_handlers.go`, read `asst.ClaudeSubscriptionModel` and append `ANTHROPIC_MODEL=<value>` (defaulting to `claude-opus-4-6`) to the env slice
- [ ] Add `claude_subscription_model` to the `IAssistantConfig` interface and related types in `frontend/src/types.ts`
- [ ] Add `claudeSubscriptionModel?: string` to the form params type in `frontend/src/contexts/apps.tsx` and pass it through to the assistant config as `claude_subscription_model`
- [ ] Update `frontend/src/utils/app.ts` to include `claude_subscription_model` in the flat-state round-trip mapping
- [ ] In `frontend/src/components/agent/CodingAgentForm.tsx`, add a `<Select>` dropdown visible when `isClaudeCodeSubscription` with three hardcoded options: `claude-opus-4-6`, `claude-sonnet-4-5-latest`, `claude-haiku-4-5-latest` (default: Opus)
- [ ] Wire the dropdown value to `onChange` so it updates `value.claudeSubscriptionModel` and propagates to the parent
- [ ] Regenerate the OpenAPI client (`./stack update_openapi`) if the type change surfaces in the API schema
- [ ] Test: create or edit a Claude Code subscription agent, confirm Opus is preselected, save, start a session, verify `ANTHROPIC_MODEL=claude-opus-4-6` is set in the container (`docker exec ... env | grep ANTHROPIC_MODEL`)
- [ ] Test: change dropdown to Haiku, save, start a session, verify `ANTHROPIC_MODEL=claude-haiku-4-5-latest` in container env
- [ ] Test: API key mode agent — confirm no regression (model picker still works, no new fields shown)
