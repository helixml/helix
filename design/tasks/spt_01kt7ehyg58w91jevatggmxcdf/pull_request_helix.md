# fix(frontend): use translucent hover tint for org switcher in dark mode

## Summary

When dark mode was added, the org-switcher popover (`UserOrgSelector`) inherited an unsuitable hover style. Hovering over a non-selected, joined organisation painted the whole row a solid bright Material cyan (`#4fc3f7`) — visually jarring against the dark panel and washing out the row's text.

Root cause: the hover branch used `lightTheme.highlightColor`, which resolves to the opaque `darkHighlight: '#4fc3f7'` from `themes.tsx`. That token is fine as a *fill* (it's used as a code-block background in skill dialogs etc.) but is wrong as a *hover overlay*.

This swaps that one branch for `rgba(0, 229, 255, 0.08)` — a translucent cyan that matches the existing selected resting state (`0.1`) and selected hover state (`0.15`) used in the same component. The shared `darkHighlight` token is **not** touched, so other call sites (`AddMcpSkillDialog`, `AddApiSkillDialog`, `AccessManagement`) are unaffected.

## Changes

- `frontend/src/components/orgs/UserOrgSelector.tsx` (1 line) — non-selected joined-org hover now uses `rgba(0, 229, 255, 0.08)` instead of `lightTheme.highlightColor`.

## Screenshots

Hovering a non-selected org row in dark mode:

**Before** (opaque bright cyan, text barely readable):
![Before](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kt7ehyg58w91jevatggmxcdf/screenshots/00-hover-nonselected-BEFORE.png)

**After** (subtle cyan tint, fits dark theme):
![After](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kt7ehyg58w91jevatggmxcdf/screenshots/01-hover-nonselected-AFTER.png)

## Test plan

- [x] `yarn tsc --noEmit` clean inside `helix-frontend-1`
- [x] Verified in inner Helix dark mode with two orgs: non-selected hover → subtle cyan tint; selected hover → unchanged `rgba(0, 229, 255, 0.15)`; non-member rows still get no hover background.
- [x] Verified light mode still produces a perceptible hover (cyan over white reads fine).
- [x] No change to `themes.tsx` tokens — components using `lightTheme.highlightColor` as a fill are unaffected.
