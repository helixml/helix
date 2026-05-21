# Design: Make Onboarding Page Light-Mode Friendly

## Approach

Extend the existing `getOnboardingPalette(isLight)` function so it returns
**every** color value the page needs (cards, menus, dividers, all per-step
inline colors). Then mechanically replace the ~114 hardcoded color literals in
`Onboarding.tsx` with references to fields on that palette object.

We are *not* refactoring the page into smaller components, not introducing a
new theme system, and not changing the visual design. The fix is a search-and-
replace within one file plus a richer palette object.

## Why not just add a `useTheme()` ternary everywhere?

That's effectively what we're doing, but funneling all the ternaries through
one `getOnboardingPalette` helper has two benefits:

1. The render path stays readable: `sx={{ color: palette.TEXT_PRIMARY }}` is
   easier to scan than `sx={{ color: isLight ? "#1a1a2e" : "#fff" }}`.
2. The palette becomes the single audit point for "do we cover all colors?" —
   future contributors copy a field name, not a ternary.

## Palette additions

Extend `getOnboardingPalette()` (currently at lines 83–107) with the missing
fields. Below, **dark values** match what's currently hardcoded; **light
values** need to be picked to mirror the rest of the app's light theme.

| Field | Used for | Dark | Light |
|---|---|---|---|
| `CARD_BG` *(replace existing)* | Step card background | `#0f0f1e` | `#ffffff` |
| `CARD_BG_ACTIVE` *(replace existing)* | Active step card background | `#101024` | `#fafafa` |
| `MENU_BG` *(new)* | Select dropdown popover | `#1a1a2e` | `#ffffff` |
| `MENU_TEXT` *(new)* | Select dropdown text | `#fff` | `#1a1a2e` |
| `STEP_INACTIVE` *(new)* | Inactive step circle outline | `rgba(255,255,255,0.15)` | `rgba(0,0,0,0.2)` |
| `BORDER_SUBTLE` *(new)* | Hover/divider | `rgba(255,255,255,0.06)` | `rgba(0,0,0,0.06)` |
| `BORDER_HOVER` *(new)* | Card hover border | `rgba(255,255,255,0.15)` | `rgba(0,0,0,0.15)` |
| `OVERLAY_FAINT` *(new)* | Subtle background fills | `rgba(255,255,255,0.02)` | `rgba(0,0,0,0.02)` |
| `OVERLAY_DIM` *(new)* | Slightly stronger fills | `rgba(255,255,255,0.03)` | `rgba(0,0,0,0.03)` |
| `RADIO_UNCHECKED` *(new)* | Unchecked radio button | `rgba(255,255,255,0.3)` | `rgba(0,0,0,0.4)` |

The accent green (`#00e891`) and `btnSx`'s `color: "#000"` (black text on
green button) stay the same — green-on-black is the brand mark and reads fine
in both themes.

## Code structure changes

1. **Expand `getOnboardingPalette()`** with the fields above. Delete the
   "cards always stay dark" comment.
2. **Delete the dark-only module constants** at lines 112–124 (`BG`, `CARD_BG`,
   `CARD_BG_ACTIVE`, `CARD_BORDER`, `inputSx`, `labelSx`, `helperSx`). These
   are only referenced from inside `Onboarding()` where `palette` is in scope —
   replace each reference with `palette.X`.
3. **Convert `btnSx`** from a module constant to a `useMemo` inside the
   component if it needs to vary by theme. (Currently it doesn't — green
   button with black text works in both themes — so leave as-is.)
4. **Replace inline hardcoded colors** throughout `renderStepContent` and its
   step-specific render functions. Mechanical replacement:
   - `color: "#fff"` → `color: palette.TEXT_PRIMARY`
   - `color: "rgba(255,255,255,0.6)"` → `color: palette.TEXT_SECONDARY`
   - `color: "rgba(255,255,255,0.4)"` → `color: palette.TEXT_FADED`
   - `color: "rgba(255,255,255,0.25)"` / `0.3` → `color: palette.TEXT_DIM`
   - `color: "rgba(255,255,255,0.15)"` (inactive step) → `palette.STEP_INACTIVE`
   - `borderColor: "rgba(255,255,255,0.1)"` → `palette.CARD_BORDER`
   - `borderColor: "rgba(255,255,255,0.06)"` → `palette.BORDER_SUBTLE`
   - `borderColor: "rgba(255,255,255,0.15)"` → `palette.BORDER_HOVER`
   - `bgcolor: "#1a1a2e"` (5 menus) → `bgcolor: palette.MENU_BG`
   - `bgcolor: "rgba(255,255,255,0.02)"` → `palette.OVERLAY_FAINT`
   - `bgcolor: "rgba(255,255,255,0.03)"` → `palette.OVERLAY_DIM`
   - `color: "rgba(255,255,255,0.3)"` on radios → `palette.RADIO_UNCHECKED`

## Edge cases

- **`CodingAgentForm` is a separate component** (`components/agent/CodingAgentForm.tsx`)
  that receives `labelSx`, `captionSx`, `selectSx`, `menuPaperSx`,
  `textFieldInputSx`, `textFieldLabelSx`, `textFieldHelperSx`,
  `claudeCredentialsBoxSx`, `claudeRadioSx` props from `Onboarding.tsx`. We
  pass palette-derived sx objects through these props — no change to
  `CodingAgentForm` itself.
- **`BrowseProvidersDialog`, `AddProviderDialog`, `ClaudeSubscriptionConnect`**
  are rendered from `Onboarding.tsx` but are full components with their own
  theme handling. Out of scope here — if any of them is also light-mode-broken,
  flag it as a follow-up.
- **`Bot` and `Server` icons from `lucide-react`** take a `color` prop, not
  `sx`. The current `<Bot size={16} color="rgba(255,255,255,0.4)" />` (line 2078)
  needs `palette.TEXT_FADED`. Same pattern for any other lucide icons.
- **Filter shadow on completed step icon** (line 1045) uses
  `filter: drop-shadow(0 0 6px ${ACCENT}60)`. This is the accent green glow —
  fine in both themes, no change.

## Files touched

- `frontend/src/pages/Onboarding.tsx` — entire file gets the
  search-and-replace + palette extension.

That's it. No new files, no other components.

## Testing strategy

The Helix-in-Helix inner stack at `http://localhost:8080` runs the full UI.
Toggle theme via the user menu (top-right after sign-in) or via
`localStorage.setItem('theme', 'light')` + reload.

1. **Register a fresh user** (`test@helix.ml` / `helixtest`) so onboarding
   triggers.
2. **Take screenshots of all 6 steps** in both light and dark mode using
   `chrome-devtools` MCP. Save under
   `helix-specs/design/tasks/002039_onboarding-is-not-light/screenshots/`.
3. **Eyeball comparison:** dark screenshots should match the
   pre-change baseline; light screenshots should look like the rest of the
   light-mode app (compare with `/login` rendered in light, which uses the same
   color philosophy).
4. **Contrast check:** open each light-mode step in Chrome DevTools, use the
   Accessibility panel's contrast checker on text elements. Target AA (4.5:1
   normal, 3:1 large).
5. **Final grep check:**
   `grep -cE "rgba\(255,255,255|#fff|#1a1a2e|['\"]white['\"]" frontend/src/pages/Onboarding.tsx`
   should be 0 (or only inside `getOnboardingPalette` ternaries).
6. **`yarn build`** in `frontend/` must succeed.

## Risk

Low. One file, mostly mechanical changes. The biggest risk is missing some
colors and shipping a partially light-mode page; the final grep check catches
that.
