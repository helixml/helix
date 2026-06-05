# fix(frontend): make onboarding light-mode friendly

## Summary

The `/onboarding` flow had ~114 hardcoded dark-mode color literals (`rgba(255,255,255,…)`, `#fff`, `#1a1a2e`) baked into inline `sx` props, making it broken in light mode: cards stayed dark, dropdown popovers stayed dark, and inactive step indicators became invisible against the light page background. A partial light-mode adaptation existed via `getOnboardingPalette(isLight)` but it explicitly skipped the cards and inner step content (with a comment saying "thousands of inline hardcoded colors would each need touching").

This change finishes that work — the page is now fully theme-aware.

## Changes

- **`frontend/src/pages/Onboarding.tsx`** (~150 lines changed, no functional changes)
  - Extended `getOnboardingPalette()` with the missing fields needed by the render path: `CARD_BG`, `CARD_BG_ACTIVE`, `MENU_BG`, `MENU_TEXT`, `STEP_INACTIVE`, `BORDER_SUBTLE`, `BORDER_HOVER`, `INPUT_BORDER`, `INPUT_BORDER_HOVER`, `OVERLAY_FAINT`, `OVERLAY_DIM`, `TEXT_MUTED`, `RADIO_UNCHECKED`, `selectSx`. Each is a `isLight ? <light> : <dark>` ternary.
  - Deleted the dark-only module-level constants (`BG`, `CARD_BG`, `CARD_BG_ACTIVE`, `CARD_BORDER`, `inputSx`, `labelSx`, `helperSx`) that were leftovers from the partial adaptation, plus the misleading "cards always stay dark" comment.
  - Replaced every inline color literal in the render path with the corresponding `palette.*` field. This includes all 5 hardcoded `bgcolor: "#1a1a2e"` menu popovers, the invisible inactive-step indicator, the `<Bot color="rgba(255,255,255,0.4)" />` lucide icon, and the `sx` props passed through to `CodingAgentForm` (`claudeCredentialsBoxSx`, `selectSx`, etc.).
  - The accent green (`#00e891`) and the black-on-green button styling are preserved in both themes.

After the change, the only literal color values in the file are inside `getOnboardingPalette()` itself (one source of truth per theme).

## Verification

- `cd frontend && npx tsc --noEmit -p .` — 0 errors
- `cd frontend && yarn build` — clean build (35.55s)
- `grep -cE 'rgba\(255,255,255|#fff|#1a1a2e' frontend/src/pages/Onboarding.tsx` — only matches are inside the palette ternaries
- End-to-end tested in the inner Helix at `http://localhost:8080`: registered a fresh user, walked through the onboarding flow in both dark and light modes (toggled via OS `prefers-color-scheme`), opened dropdown popovers in both themes.

## Screenshots

### Organization step

Dark mode (unchanged behavior) vs light mode (the fix) — light mode now renders with light card backgrounds, dark text, visible borders, and a visible inactive step indicator. Before the fix, the cards stayed dark on a light page and the inactive circles were `rgba(255,255,255,0.15)` — invisible.

| Dark | Light |
|---|---|
| ![Dark mode organization step](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002039_onboarding-is-not-light/screenshots/01-after-dark-organization.png) | ![Light mode organization step](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002039_onboarding-is-not-light/screenshots/02-after-light-organization.png) |

### Project step (the densest screen — most form controls)

Full light-mode render of the project step: text inputs, selector cards, radio buttons, dropdowns, the Claude credentials panel and the agent name field — all theme-aware now.

![Light mode project step (full page)](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002039_onboarding-is-not-light/screenshots/03-after-light-project.png)

### Dropdown popover — the critical fix

These popovers used to hardcode `bgcolor: "#1a1a2e"` so they rendered as a dark slab on a light page. Now they pick up `palette.MENU_BG` (white in light mode, `#1a1a2e` in dark mode):

| Light popover | Dark popover (no regression) |
|---|---|
| ![Light mode organization popover open](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002039_onboarding-is-not-light/screenshots/07-after-light-org-popover.png) | ![Dark mode organization popover open](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002039_onboarding-is-not-light/screenshots/06-after-dark-menu-popover.png) |

### Resolution dropdown (a second instance of the same fix, inside the project step)

![Light mode resolution dropdown open](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002039_onboarding-is-not-light/screenshots/04-after-light-menu-popover.png)
