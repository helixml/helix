# Implementation Tasks: Fix Ugly Bright Blue Org Switcher Hover Color in Dark Mode

- [ ] In `frontend/src/components/orgs/UserOrgSelector.tsx`, change the non-current-org hover branch (around line 1244) from `lightTheme.highlightColor` to a mode-aware translucent value (`rgba(0, 229, 255, 0.08)` in dark, `rgba(14, 116, 144, 0.08)` in light).
- [ ] Leave the current-org hover (`rgba(0, 229, 255, 0.15)`) and the `org.member === false` `transparent` branch untouched.
- [ ] Do not modify `darkHighlight` or `darkIconHover` tokens in `frontend/src/themes.tsx` — they are reused as foreground accents elsewhere.
- [ ] Manually verify in dark mode: hover each org row, confirm the highlight is a subtle wash and the current-org row remains visually distinct.
- [ ] Manually verify in light mode: confirm no visual regression.
