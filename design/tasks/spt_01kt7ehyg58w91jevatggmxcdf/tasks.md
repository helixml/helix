# Implementation Tasks: Fix Ugly Bright Blue Org Switcher Hover Color in Dark Mode

- [x] In `frontend/src/components/orgs/UserOrgSelector.tsx` at line 1244, replace `lightTheme.highlightColor` with `'rgba(0, 229, 255, 0.08)'` in the non-selected joined-org hover branch.
- [x] Run TypeScript build (`yarn tsc --noEmit` inside `helix-frontend-1`) — clean, 0 errors.
- [x] Manually verify in the inner Helix: created a second org, opened popover, hovered non-selected row — confirmed subtle cyan tint (AFTER) replaces the opaque bright blue (BEFORE). Screenshots in `screenshots/`.
- [x] Spot-check unchanged: `lightTheme.highlightColor` token was not modified, so `AddMcpSkillDialog`, `AddApiSkillDialog`, `AccessManagement` are unaffected.
- [x] Commit code change and push feature branch (`feature/002070-fix-ugly-bright-blue-org`).
- [x] Write per-repo PR descriptions (`pull_request_helix.md`).
