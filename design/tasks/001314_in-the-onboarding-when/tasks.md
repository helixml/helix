# Implementation Tasks

- [~] Add `desktopResolution` state to `Onboarding.tsx` (default: `'1080p'`)
- [ ] Add resolution dropdown (Select) in Step 3 after `CodingAgentForm` (only visible when `agentMode === 'create'`)
- [ ] Style dropdown to match existing onboarding Select components (use same `sx` props as other selects in the file)
- [ ] MenuItem text: "1080p" with caption "Run more agents in parallel", "4K (2x scaling)" with caption "Sharper display quality"
- [ ] Update `handleCreateProject` to call `apps.updateApp()` after agent creation with `external_agent_config`
- [ ] Set `resolution` and `zoom_level` (100 for 1080p, 200 for 4K) in the config update
- [ ] Test: Create agent via onboarding, verify `external_agent_config` is saved correctly
- [ ] Test: Start a session with the new agent, verify resolution is applied