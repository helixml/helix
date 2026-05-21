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

## Screenshots

End-to-end screenshots couldn't be captured because the inner Helix stack (API, postgres, frontend container) was still building during this work — the dev frontend was reachable at port 8081 but the API needed for registration/auth wasn't. Visual verification still to-do once the stack is up; the change is mechanical and the build passes.
