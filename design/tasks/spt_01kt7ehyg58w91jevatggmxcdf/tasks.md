# Implementation Tasks: Fix Ugly Bright Blue Org Switcher Hover Color in Dark Mode

- [~] In `frontend/src/components/orgs/UserOrgSelector.tsx` at line 1244, replace `lightTheme.highlightColor` with `'rgba(0, 229, 255, 0.08)'` in the non-selected joined-org hover branch.
- [ ] Verify Vite HMR picks up the change (no rebuild needed); reload `http://localhost:8080` in the inner Helix.
- [ ] Manually test in dark mode: hover a non-selected joined org → faint cyan tint; hover the selected org → unchanged `rgba(0, 229, 255, 0.15)`; hover a non-member row → no background.
- [ ] Manually test in light mode: hover is still visible and consistent.
- [ ] Spot-check `AddMcpSkillDialog`, `AddApiSkillDialog`, and `AccessManagement` — these still use `lightTheme.highlightColor` and must look unchanged.
- [ ] Run `cd frontend && yarn build` to confirm no TypeScript/build regressions.
- [ ] Commit as `fix(frontend): use translucent hover tint for org switcher in dark mode` and open a PR against `helixml/helix`.
