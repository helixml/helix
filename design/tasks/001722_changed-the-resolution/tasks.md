# Implementation Tasks

- [x] In `frontend/src/pages/App.tsx` (lines 214-273): Replace conditional tab rendering (`{tabValue === 'x' && <Component />}`) with always-mounted components wrapped in `<Box sx={{ display: tabValue === 'x' ? 'block' : 'none' }}>` — apply to all tab panels (appearance, settings, knowledge, apikeys)
- [~] Test: Change resolution on Settings tab → switch to Appearance → change name → switch back to Settings → verify resolution is preserved
- [ ] Test: Change name on Appearance → switch to Settings → change model → switch back to Appearance → verify name is preserved
- [x] Run `cd frontend && npx tsc --noEmit` to verify no type errors
