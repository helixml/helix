# Implementation Tasks

- [ ] In `Onboarding.tsx` line 312, replace `connectedProviderIds.has("anthropic")` with a `useMemo` that checks all providers (not just user-type), matching the fixed pattern in `AgentSelectionModal.tsx`
- [ ] In `Onboarding.tsx` lines 545-555, expand the auto-select `useEffect` to also trigger Claude Code (api_key mode) when `hasAnthropicProvider` is true, with api_key taking priority over subscription mode
- [ ] In `AgentSelectionModal.tsx` lines 133-139, expand the auto-select `useEffect` to also trigger Claude Code (api_key mode) when `hasAnthropicProvider` is true (the `hasAnthropicProvider` check itself was already fixed in the recent PR)
- [ ] Build frontend (`cd frontend && yarn build`) and verify no TypeScript errors
- [ ] Test in app: configure Anthropic as global/system provider only → open onboarding → verify Claude Code is auto-selected with api_key mode and "(configured)" label shown
- [ ] Test in app: go to project settings → Create New Agent → verify same auto-select behavior
