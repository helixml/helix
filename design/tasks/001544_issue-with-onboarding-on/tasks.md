# Implementation Tasks

- [x] Create `frontend/src/components/agent/useCodingAgentProviderState.ts` — hook that calls `useListProviders`/`useClaudeSubscriptions`, computes `hasAnthropicProvider`/`hasClaudeSubscription`, and auto-selects the correct runtime/mode via `onChange` on first load (with `useRef` guard)
- [x] Update `CodingAgentForm.tsx` — call the new hook internally, remove `hasAnthropicProvider` and `hasClaudeSubscription` from `CodingAgentFormProps`
- [x] Remove now-redundant code from all five parent components (`Onboarding.tsx`, `ProjectSettings.tsx`, `CreateProjectDialog.tsx`, `AgentSelectionModal.tsx`, `NewSpecTaskForm.tsx`): the `useListProviders`/`useClaudeSubscriptions` calls used for provider-state, the `hasAnthropicProvider`/`hasClaudeSubscription` derivations, the auto-select `useEffect`, and the two props passed to `CodingAgentForm`
- [x] Fix `AppSettings.tsx` — update the standalone `hasAnthropicProvider` check to include global providers (not in `CodingAgentForm`, so not covered by the hook)
- [x] Build frontend (`cd frontend && yarn build`) and verify no TypeScript errors
- [ ] Test: configure Anthropic as global/system provider only → open onboarding → Claude Code auto-selected with api_key mode, "(configured)" label shown
- [ ] Test: same scenario in project settings → Create New Agent modal
