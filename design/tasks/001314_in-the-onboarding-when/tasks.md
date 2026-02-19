# Implementation Tasks

- [ ] Add `desktopResolution` state to `Onboarding.tsx` (default: `'1080p'`)
- [ ] Add resolution picker UI in Step 3 after `CodingAgentForm` (only visible when `agentMode === 'create'`)
- [ ] Style picker as two clickable cards matching existing onboarding design (use `CARD_BG`, `CARD_BORDER_ACTIVE`, etc.)
- [ ] Card text: "1080p — run more agents in parallel" and "4K — sharper display (2x scaling)"
- [ ] Update `handleCreateProject` to call `apps.updateApp()` after agent creation with `external_agent_config`
- [ ] Set `resolution` and `zoom_level` (100 for 1080p, 200 for 4K) in the config update
- [ ] Test: Create agent via onboarding, verify `external_agent_config` is saved correctly
- [ ] Test: Start a session with the new agent, verify resolution is applied