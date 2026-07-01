# Implementation Tasks: Standardise Review Spec & View PR Button Colours to Match Create Task

- [x] In `SpecTaskActionButtons.tsx`, change the inline "Review Spec" `CompactActionButton` from `color="info"` to `color="secondary"` (~line 507)
- [x] In `SpecTaskActionButtons.tsx`, change the stacked "Review Spec" `Button` from `color="info"` to `color="secondary"` (~line 541)
- [x] In `SpecTaskActionButtons.tsx`, remove the `buttonColor` state-based ternary for the single PR case (~lines 822–823) and set the inline + stacked "View Pull Request" buttons to `color="secondary"` (~lines 831, 852)
- [x] Retint the "Review Spec" pulse-glow `rgba(41,182,246,…)` to the brand cyan `rgba(0,213,255,…)` so the glow matches the new button colour
- [x] Type-check the change (`tsc --noEmit` clean; `vite build` transformed all modules — only the read-only `dist` bind mount blocked final write, an environment quirk)
- [ ] Verify in inner Helix (light + dark mode) that Review Spec, View Pull Request, and Create Task buttons all share the same colour, and the PR-state badge still shows merged/closed correctly
