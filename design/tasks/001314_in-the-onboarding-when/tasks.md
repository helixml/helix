# Implementation Tasks

- [x] Add `desktopResolution` state to `Onboarding.tsx` (default: `'1080p'`)
- [x] Add resolution dropdown (Select) in Step 3 after `CodingAgentForm` (only visible when `agentMode === 'create'`)
- [x] Style dropdown to match existing onboarding Select components (use same `sx` props as other selects in the file)
- [x] MenuItem text: "1080p" with caption "Run more agents in parallel", "4K (2x scaling)" with caption "Sharper display quality"
- [x] Update `handleCreateProject` to call `apps.updateApp()` after agent creation with `external_agent_config`
- [x] Set `resolution` and `zoom_level` (100 for 1080p, 200 for 4K) in the config update
- [x] Test: Create agent via onboarding, verify `external_agent_config` is saved correctly
- [x] Test: Start a session with the new agent, verify resolution is applied

## Testing Results

✅ **All tests passed**

1. **UI Test**: Resolution dropdown appears in onboarding Step 3 when creating new agent
   - Shows "1080p - Run more agents in parallel" (default)
   - Shows "4K (2x scaling) - Sharper display quality"

2. **Database Verification**: Created agent with 4K selected, verified in database:
   ```sql
   SELECT config->'helix'->'external_agent_config' FROM apps WHERE id='app_01kj3n79jcnmyfgtkqpd0t00s1';
   -- Result: {"resolution":"4k","zoom_level":200}
   ```

3. **Build**: `yarn build` passes without errors