# Requirements: Make Onboarding Page Light-Mode Friendly

## Background

The `/onboarding` flow (`frontend/src/pages/Onboarding.tsx`, ~2520 lines) is the
first screen a new user sees after signing up. It currently renders correctly
in dark mode but is broken in light mode: the page background lightens, but the
step cards and almost all interactive content inside them stay hardcoded dark.

A partial light-mode adaptation already exists via `getOnboardingPalette(isLight)`
(lines 83–107) which adapts the *outer* page background, top-level text and
input chrome. But the file comment at lines 84–87 explicitly documents that
"cards always stay dark — the inner step content has thousands of inline
hardcoded `rgba(255,255,255,…)` text colors that would each need touching."

A quick grep confirms ~114 hardcoded dark-mode color values
(`rgba(255,255,255,…)`, `#fff`, `"white"`, `#1a1a2e`) still in the file. Of
particular note:

- Module-level constants `inputSx`, `labelSx`, `helperSx`, `btnSx`, `CARD_BG`,
  `CARD_BORDER` (lines 112–145) are dark-only but still referenced from the
  render path alongside the light-aware palette.
- Five `MenuProps.PaperProps` blocks hardcode `bgcolor: "#1a1a2e"` for select
  dropdowns (lines 1205, 1493, 2062, 2157, 2209) — these float over the page
  and look broken in light mode.
- The "inactive step" indicator uses `rgba(255,255,255,0.15)` (line 1054) which
  becomes invisible on a light background.
- Per-step content (`renderStepContent`) is full of inline `color: "#fff"`,
  `color: "rgba(255,255,255,0.x)"`, and `borderColor: "rgba(255,255,255,…)"`
  values that don't respond to theme.

## User Stories

### Story 1: Light-mode user can read the onboarding flow

**As** a new Helix user with light theme selected,
**I want** the onboarding page to render in light mode the same as the rest of
the app,
**so that** I can read text, see step indicators, and complete onboarding
without strained eyes or invisible elements.

**Acceptance criteria:**

- On `/onboarding` with `theme.palette.mode === 'light'`:
  - Page background is light (already works).
  - Step cards have light backgrounds with visible borders and dark text
    (currently dark cards on light page).
  - All text within step cards is dark and meets WCAG AA contrast (≥ 4.5:1)
    against its background.
  - Inactive step circles are visible (dark grey, not invisible white-15%).
  - Select dropdown popovers (organization, agent, resolution, etc.) have light
    backgrounds with dark text — not the hardcoded `#1a1a2e` panel.
  - Text inputs have light background, dark text, and visible borders.
  - The accent green (`#00e891`) for active states and primary buttons is
    preserved in both themes.

### Story 2: Dark-mode user sees no regression

**As** a Helix user with dark theme selected,
**I want** the onboarding page to look identical to how it does today,
**so that** the fix doesn't break the existing UX I already approved.

**Acceptance criteria:**

- With `theme.palette.mode === 'dark'`, every pixel of `/onboarding` matches
  the current dark-mode rendering. Screenshot diff at each step (signin,
  organization, subscription, provider, project, task) is empty.

### Story 3: Future maintainers don't reintroduce hardcoded colors

**As** a developer touching `Onboarding.tsx` later,
**I want** the file to have *zero* hardcoded `#fff` / `rgba(255,255,255,…)` /
`#1a1a2e` / `"white"` values,
**so that** any new content I add automatically respects both themes.

**Acceptance criteria:**

- After this change, `grep -E "rgba\(255,255,255|#fff|#1a1a2e|'white'|\"white\""
  frontend/src/pages/Onboarding.tsx` returns no matches (other than possibly
  inside `getOnboardingPalette` itself where it produces the dark-mode value
  via a ternary on `isLight`).
- The misleading comment at lines 84–87 (explaining why cards stay dark) is
  removed or replaced with the actual new policy.
