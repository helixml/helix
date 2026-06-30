# Implementation Tasks: Standardise Review Spec & View PR Button Colours to Match Create Task

- [ ] In `SpecTaskActionButtons.tsx`, change the inline "Review Spec" `CompactActionButton` from `color="info"` to `color="secondary"` (~line 507)
- [ ] In `SpecTaskActionButtons.tsx`, change the stacked "Review Spec" `Button` from `color="info"` to `color="secondary"` (~line 541)
- [ ] In `SpecTaskActionButtons.tsx`, remove the `buttonColor` state-based ternary for the single PR case (~lines 822–823) and set the inline + stacked "View Pull Request" buttons to `color="secondary"` (~lines 831, 852)
- [ ] (Optional) Retint the "Review Spec" pulse-glow `rgba(41,182,246,…)` to the brand cyan so the glow matches the new button colour
- [ ] Run `cd frontend && yarn build` to confirm it compiles
- [ ] Verify in inner Helix (light + dark mode) that Review Spec, View Pull Request, and Create Task buttons all share the same colour, and the PR-state badge still shows merged/closed correctly
