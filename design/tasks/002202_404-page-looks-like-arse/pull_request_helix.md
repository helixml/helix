# fix(frontend): make 404 page legible in light mode

## Summary
The 404 / Not Found page (`frontend/src/pages/NotFound.tsx`) was styled for dark
mode only — every color was hardcoded (white text at various opacities, cyan
accents). In light mode the "404" glyph, the "Page not found" heading, the body
text, and the outlined/text buttons were white-on-white (invisible) or failed
contrast. This makes the page theme-aware so it's legible in both modes.

## Changes
- Adopt the existing `useLightTheme()` hook to drive colors from
  `theme.palette.mode` (same pattern as `Create.tsx`).
- Branch each hardcoded color on `isLight`, keeping the exact original values in
  the dark branch (no dark-mode regression) and substituting legible light-mode
  values (`textColor`, `textColorFaded`, `border`, and `highlightColor` for
  accents — brand cyan is illegible on white).
- Light-mode background gradient uses a faint teal instead of cyan.
- The contained "Home" button (cyan bg + black text) and the glitch animation
  are unchanged — they work in both modes.

Single-file change: `frontend/src/pages/NotFound.tsx` (+20/-12).

## Testing
- `tsc --noEmit` passes.
- Verified end-to-end in the dev stack: rendered `/notfound` and toggled
  light/dark. Light mode is now fully legible; dark mode is unchanged.

## Screenshots
Light mode — before (the bug: heading/body/buttons invisible):
![Light before](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002202_404-page-looks-like-arse/screenshots/00-404-light-before.png)

Light mode — after (legible):
![Light after](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002202_404-page-looks-like-arse/screenshots/01-404-light-after.png)

Dark mode — after (unchanged):
![Dark after](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002202_404-page-looks-like-arse/screenshots/02-404-dark-after.png)
