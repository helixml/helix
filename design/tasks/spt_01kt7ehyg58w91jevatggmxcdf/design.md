# Design: Fix Ugly Bright Blue Org Switcher Hover Color in Dark Mode

## Root Cause

`frontend/src/components/orgs/UserOrgSelector.tsx:1244` uses `lightTheme.highlightColor` as the hover background for non-selected joined-org rows.

`useLightTheme()` resolves `highlightColor` to:

- light mode: `themeConfig.lightHighlight` → `#0e7490` (dark cyan/teal — works on white)
- dark mode: `themeConfig.darkHighlight` → `#4fc3f7` (bright Material cyan — clashes on `#121214`)

`darkHighlight` of `#4fc3f7` is a generic fill colour reused elsewhere (skill-dialog code blocks, access-management hover rows). The bug is not the token itself — it's that the org-switcher uses an opaque fill for a *hover row*, where the rest of the org-switcher already uses translucent cyan (`rgba(0, 229, 255, 0.1)` for selected resting, `rgba(0, 229, 255, 0.15)` for selected hover). The non-selected hover should follow the same pattern.

## Decision

Make a **local, targeted fix** in `UserOrgSelector.tsx` only. Do not change the shared `darkHighlight` token.

Replace line 1244:

```tsx
: lightTheme.highlightColor,
```

with:

```tsx
: 'rgba(0, 229, 255, 0.08)',
```

`0.08` opacity sits one step below the `0.1` used for the selected resting state, so a non-selected hovered row reads as a lighter "candidate" of the same hue. It works on both the dark panel (`#1e1e24`) and the light panel (`#f4f4f4`) because the underlying cyan `#00E5FF` is the existing org-switcher accent (also used for the selected ring at line 1272).

## Why not change `darkHighlight` globally?

`darkHighlight` is referenced by 4 other components (`AddApiSkillDialog`, `AddMcpSkillDialog`, `AccessManagement`, plus the hook itself). Several use it as a *background fill* (code-block backgrounds, status pills), where a solid colour is correct. Swapping it for a translucent value would regress those surfaces. A targeted per-component override is the lower-risk change.

## Files Changed

- `frontend/src/components/orgs/UserOrgSelector.tsx` (1 line)

## Verification

1. Hot reload via Vite (port 8081) — no rebuild needed.
2. In the inner Helix at `http://localhost:8080`, register/log in, open the sidebar org switcher.
3. In dark mode (OS dark preference), hover over a non-selected org row → hover should be a faint cyan tint, not solid bright blue.
4. Hover the currently-selected org row → unchanged (`rgba(0, 229, 255, 0.15)`).
5. Hover a non-member row → no hover background.
6. Toggle to light mode (OS light preference) and confirm hover is still perceptible and not regressed.
7. Sanity-check `Settings → Access`, `Add MCP Skill`, `Add API Skill` dialogs are visually unchanged (we did not touch `darkHighlight`).

## Notes for Future Agents

- `useLightTheme()` (`frontend/src/hooks/useLightTheme.tsx`) is the canonical light/dark switch hook in this codebase. `highlightColor` from it is intended as a **fill** colour, not a hover indicator. When adding hover states to dark-mode surfaces, prefer translucent overlays (`rgba(255,255,255,0.04-0.08)` or theme-accent `rgba(0, 229, 255, 0.08)`) over opaque tokens.
- Dark/light tokens live in `frontend/src/themes.tsx` under the `helix` theme. Each token comes in `light*` / `dark*` pairs and is resolved by `useLightTheme()`. Avoid hard-coding hex colours in component `sx` props when an existing token fits — but also avoid forcing an opaque token into a translucent role.
