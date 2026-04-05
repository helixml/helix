# Keep agent editor tab components mounted to prevent settings loss

## Summary
Switching between tabs in the agent editor (e.g., Settings → Appearance) caused settings changes to be silently lost. This happened because tab components were conditionally rendered — switching tabs unmounted the old component, destroying its local state. A subsequent save from the new tab could overwrite the previous tab's changes with stale data.

## Changes
- Changed tab rendering in `frontend/src/pages/App.tsx` from conditional (`{tabValue === 'x' && <Component />}`) to always-mounted with CSS display toggling (`display: tabValue === 'x' ? 'block' : 'none'`)
- Applied to all four tab panels: appearance, settings, knowledge, apikeys

## Test plan
- Change resolution on Settings tab → switch to Appearance → change name → switch back: resolution preserved
- Change name on Appearance → switch to Settings → change resolution → switch back: name preserved
- TypeScript compilation passes with no errors
