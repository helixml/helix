# fix(frontend): make Tests tab theme-aware (fix dark-on-dark in light mode)

## Summary
The **Tests** tab in agent settings (`TestsEditor.tsx`) hardcoded dark hex
backgrounds (`#2a2d3e`, `#1e1e2f`, `#0d1117`) and `color: 'white'` on the copy
buttons. These ignored the active theme, so in **light mode** MUI rendered
near-black text on those still-dark panels — the cards, CLI instructions and
CI/CD examples were unreadable ("black on black").

This wires the component to the existing `useLightTheme()` hook so every panel,
card, code block and copy icon uses theme-appropriate colors. Dark mode is
unchanged.

## Changes
- `frontend/src/components/app/TestsEditor.tsx`:
  - Import and call `useLightTheme()`.
  - Outer panels (per-test card, CLI box) → `panelColor`.
  - Inner boxes (per-step card, command snippet, accordions) → `backgroundColor`.
  - Code blocks → `isLight ? '#f0f0f4' : '#0d1117'` (light neutral in light mode,
    GitHub-dark in dark mode).
  - The three copy `IconButton`s → shared theme-aware `sx` (`color: icon` +
    theme-aware hover) instead of `color: 'white'`.
- No functional or layout changes; colors only.

## Testing
Verified end-to-end in the dev stack: Tests tab is fully readable in light mode
and visually unchanged in dark mode.

## Screenshots
Light mode (fixed):
![Light mode test card](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002198_tests-page-is-black-on/screenshots/01-light-mode-test-card.png)
![Light mode CLI + code](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002198_tests-page-is-black-on/screenshots/02-light-mode-after.png)

Dark mode (no regression):
![Dark mode test card](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002198_tests-page-is-black-on/screenshots/03-dark-mode-test-card.png)
![Dark mode code](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002198_tests-page-is-black-on/screenshots/04-dark-mode-code.png)
