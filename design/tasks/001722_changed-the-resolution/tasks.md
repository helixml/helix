# Implementation Tasks

- [~] In `frontend/src/pages/App.tsx` (lines 214-242): Replace conditional tab rendering (`{tabValue === 'x' && <Component />}`) with always-mounted components wrapped in `<Box sx={{ display: tabValue === 'x' ? 'block' : 'none' }}>` — apply to all tab panels (appearance, settings, knowledge, integrations, access)
- [ ] Test: Change resolution on Settings tab → switch to Appearance → change name → switch back to Settings → verify resolution is preserved
- [ ] Test: Change name on Appearance → switch to Settings → change model → switch back to Appearance → verify name is preserved
- [ ] Run `cd frontend && yarn build` to verify no build errors
