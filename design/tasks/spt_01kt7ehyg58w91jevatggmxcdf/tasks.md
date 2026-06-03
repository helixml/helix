# Implementation Tasks: Fix Ugly Bright Blue Org Switcher Hover Color in Dark Mode

- [x] In `frontend/src/components/orgs/UserOrgSelector.tsx` at line 1244, replace `lightTheme.highlightColor` with `'rgba(0, 229, 255, 0.08)'` in the non-selected joined-org hover branch.
- [x] Run TypeScript build (`yarn tsc --noEmit` inside `helix-frontend-1`) — clean, 0 errors.
- [~] Manually verify in the inner Helix: log in, open the sidebar org switcher in dark mode, hover non-selected/selected/non-member rows; capture before/after screenshots.
- [ ] Spot-check `AddMcpSkillDialog`, `AddApiSkillDialog`, and `AccessManagement` — these still use `lightTheme.highlightColor` and must look unchanged (deferred — call sites unchanged, no token change).
- [ ] Commit code change and push feature branch.
