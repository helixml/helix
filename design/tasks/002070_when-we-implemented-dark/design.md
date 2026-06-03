# Design: Fix Ugly Bright Blue Org Switcher Hover Color in Dark Mode

## Root Cause

The org switcher popover (`frontend/src/components/orgs/UserOrgSelector.tsx`) uses `lightTheme.highlightColor` as the hover background for each org row:

```tsx
// UserOrgSelector.tsx ~line 1224-1246
sx={{
  '&:hover': {
    backgroundColor:
      org.member === false
        ? 'transparent'
        : currentOrgSlug === org.name
          ? 'rgba(0, 229, 255, 0.15)'
          : lightTheme.highlightColor,   // <-- the problem
  },
}}
```

`lightTheme.highlightColor` comes from `useLightTheme()` (`frontend/src/hooks/useLightTheme.tsx:12`), which in dark mode resolves to the `darkHighlight` token defined in `frontend/src/themes.tsx:103`:

```ts
darkHighlight: '#4fc3f7',   // bright cyan — used as a full opaque hover bg
```

The mistake is using a **fully opaque, saturated brand cyan as a hover background**. The rest of dark mode uses translucent washes (e.g. `rgba(0,229,255,0.13)` for menu hover in `contexts/theme.tsx:71`, and `rgba(0,229,255,0.15)` for the current-org hover on the line right above). Hitting a non-current org row drops a fully opaque `#4fc3f7` rectangle that swamps the dark panel.

## Decision

**Stop using `lightTheme.highlightColor` as a hover background.** Replace it with a translucent wash that matches the surrounding dark-mode hover idiom.

Specifically, change the non-current-org branch on `UserOrgSelector.tsx:1244` from:

```tsx
: lightTheme.highlightColor,
```

to a mode-aware translucent value, e.g.:

```tsx
: lightTheme.isLight ? 'rgba(14, 116, 144, 0.08)' : 'rgba(0, 229, 255, 0.08)',
```

Rationale:
- `rgba(0, 229, 255, 0.08)` keeps the brand cyan family but at ~8% opacity, in line with `menuHoverBg` (`0.13`) and the current-org hover (`0.15`). A non-current row should be *less* prominent than the current row, so a lower opacity (0.08) is right.
- Light-mode value uses the existing teal (`#0e7490` = `rgb(14, 116, 144)`) at 0.08, which gives a soft hover that doesn't darken the panel too much.
- This is a localised fix — we don't retune the `darkHighlight` token because that token may be used elsewhere as a foreground/accent colour (e.g. icon hover) where full opacity is correct. Touching the token risks regressions outside the org switcher.

## Alternatives Considered

1. **Retune `darkHighlight` token to a translucent value.** Rejected — `darkIconHover` and `darkHighlight` are also used for foreground accents (icon hover), where translucency would look washed-out. Mixing roles into one token is what caused the bug; deepening that conflation isn't the fix.
2. **Split `darkHighlight` into `darkHighlightFg` and `darkHighlightBg`.** Cleaner long-term, but out of scope for "fix the ugly hover". File a follow-up if other components have the same issue.
3. **Drop the hover background entirely on non-current rows.** Removes affordance — user wouldn't know hover is active until they click. Rejected.

## Files Touched

- `frontend/src/components/orgs/UserOrgSelector.tsx` — single `sx` expression on the org list item hover (around line 1244).

That's it. One file, one expression.

## Verification

Manual only (CSS / hover state — no unit test surface):
1. Run the frontend (`cd frontend && yarn dev` or follow project conventions).
2. Switch OS to dark mode (or force dark mode in the app).
3. Open the org switcher popover, hover each row, confirm the highlight is a subtle wash, not neon.
4. Switch OS to light mode, repeat — confirm no regression.
5. Confirm the current-org row still looks distinct from the others on hover (it uses its own `rgba(0, 229, 255, 0.15)` and shouldn't change).

## Notes for the Implementer

- Theme is Material UI with mode detected from OS preference (`window.matchMedia('(prefers-color-scheme: light)')`). No localStorage — every reload re-detects.
- `useLightTheme()` exposes `isLight` / `isDark` booleans — use those for mode branching inside `sx`.
- `darkHighlight` and `darkIconHover` are both `#4fc3f7` in `themes.tsx`. Don't touch the token unless you've audited every callsite.
- Hardcoded `#00E5FF` borders/glows elsewhere in `UserOrgSelector.tsx` (lines 532, 583, 623, 1272, 1303) are **out of scope** — user complaint is specifically about the hover colour.
