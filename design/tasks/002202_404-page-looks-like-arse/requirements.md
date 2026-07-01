# Requirements: Fix 404 Page Legibility in Light Mode

## Background

The 404 / "Not Found" page (`frontend/src/pages/NotFound.tsx`) was styled for
dark mode only. Every color is hardcoded (white text at various opacities, a
faint cyan radial gradient, cyan accents). In light mode these colors sit on a
light background, so the "404" glyph, the "Page not found" heading, the body
text, and the outlined/text buttons are effectively invisible or fail WCAG AA
contrast. The page "looks like arse in light mode."

The app already supports light/dark mode via MUI (`theme.palette.mode`) and a
convenience hook `useLightTheme()` that returns mode-aware colors. The 404 page
simply doesn't use it.

## User Stories

### US-1: Legible 404 page in light mode
As a user browsing Helix in light mode, when I hit a missing page, I want the
404 page's text and controls to be clearly readable, so I can understand what
happened and navigate away.

**Acceptance criteria:**
- The "404" glyph is visible (not white-on-white) in light mode.
- The "Page not found" heading and body text meet readable contrast in light mode.
- The three buttons (Home / Organizations / Go back) and their hover states are
  legible in light mode, including borders and text.
- The background decoration/gradient does not look broken (no jarring artifact)
  in light mode.

### US-2: No regression in dark mode
As a user in dark mode, I want the 404 page to look exactly as it does today
(same colors, animations, and layout), so nothing regresses.

**Acceptance criteria:**
- In dark mode the page renders visually identical to the current version
  (404 glyph, heading, body, buttons, glitch/float/pulse animations unchanged).

## Out of Scope
- Redesigning the 404 page layout, copy, or animations.
- Changing the theme system or adding new theme tokens.
- Touching any other page.
