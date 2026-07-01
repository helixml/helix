# Design: Fix 404 Page Legibility in Light Mode

## Problem

`frontend/src/pages/NotFound.tsx` is a functional component with no theme
awareness. It hardcodes dark-mode colors that are illegible on a light
background:

| Line | Element | Hardcoded color | Light-mode problem |
|------|---------|-----------------|--------------------|
| 95  | background gradient | `rgba(0,229,255,0.04)` | barely visible |
| 111 | "404" glyph | `rgba(255,255,255,0.08)` | white on white |
| 133 | heading | `rgba(255,255,255,0.85)` | white on white |
| 142 | body text | `rgba(255,255,255,0.45)` | white on white |
| 185 | outlined button border/text | `rgba(255,255,255,0.2)` / `0.7` | white on white |
| 190 | outlined button hover | `#00E5FF` | cyan fails AA on white |
| 203 | text button | `rgba(255,255,255,0.4)` | white on white |
| 207 | text button hover | `rgba(255,255,255,0.7)` | white on white |

(The contained "Home" button at line 167 uses `bgcolor:#00E5FF` + `color:#000`
and is fine in both modes — leave it as-is.)

## Approach

Make the component theme-aware using the existing `useLightTheme()` hook
(`frontend/src/hooks/useLightTheme.tsx`), which is the established pattern in
this codebase (see `Create.tsx`). It returns `isLight` plus mode-aware
`textColor`, `textColorFaded`, `border`, `highlightColor`, etc., derived from
the theme config's `light*`/`dark*` values.

The theme config already defines light-safe values, e.g. `lightSecondary:
'#0e7490'` (dark teal) with a comment noting the brand cyan `#00d5ff` is
illegible on white. Use the hook's mode-aware colors rather than inventing new
ones.

### Key decisions

1. **Use `useLightTheme()`, not raw `useTheme()` conditionals scattered inline.**
   Call the hook once, then derive colors. Keeps the component consistent with
   the rest of the frontend and avoids re-deriving `palette.mode` everywhere.

2. **Preserve exact dark-mode appearance.** For each hardcoded value, branch on
   `isLight`: keep the current dark value in the dark branch, substitute a
   legible value in the light branch. This guarantees zero dark-mode regression.

3. **Map each element to a mode-aware color:**
   - "404" glyph: dark → `rgba(255,255,255,0.08)`; light → a faint dark tint
     (e.g. `rgba(0,0,0,0.08)`) so it stays a subtle watermark, still visible.
   - heading: use `textColor` (dark `#e0e0e0` / light `#000000`) — or the
     existing 0.85-opacity white in dark, a near-black in light.
   - body text: use `textColorFaded` (dark `#a0a0b0` / light `#4a4a4a`).
   - outlined button border: use `border` from the hook; text: `textColorFaded`
     / `textColor`.
   - accent/hover color: dark → `#00E5FF`; light → `highlightColor` /
     `lightSecondary` `#0e7490` (AA-safe on white). Apply to the outlined
     button hover border+text.
   - text button: `textColorFaded` with a slightly stronger color on hover;
     hover background tint branched by mode (`rgba(0,0,0,0.05)` in light).
   - background radial gradient: keep cyan but it's decorative and low-opacity;
     optionally swap to a dark-tinted gradient in light mode so it isn't a
     lone cyan smudge. Low priority — must not look broken.

4. **Animations untouched.** The `glitch` keyframe uses cyan/pink text-shadow;
   it's a stylistic effect on the near-transparent glyph and reads fine in both
   modes — leave the keyframes as-is.

## Files touched
- `frontend/src/pages/NotFound.tsx` — only file changed.

## Testing
- Build the frontend (`cd frontend && yarn build`).
- In the inner Helix (`http://localhost:8080`), navigate to a non-existent
  route to render the 404 page. Toggle light vs dark mode and confirm all text
  and buttons are legible in light mode and unchanged in dark mode. Capture
  before/after screenshots in both modes.

## Implementation Notes

- **Change is a single file:** `frontend/src/pages/NotFound.tsx` (+20/-12). No
  other files touched. `git status` on the feature branch shows only this file.
- **Pattern used:** call `useLightTheme()` once, destructure `isLight`,
  `textColor`, `textColorFaded`, `border`, `highlightColor`. Added a derived
  `accentColor = isLight ? highlightColor : '#00E5FF'` for hover states, since
  brand cyan is illegible on white. Each hardcoded color now branches on
  `isLight`, keeping the exact original value in the dark branch → zero
  dark-mode regression.
- **Background gradient:** light branch uses `rgba(14,116,144,0.05)` (faint
  teal) instead of the cyan so it isn't a lone cyan smudge on white.
- **Home button left untouched** — its cyan bg + black text works in both modes.
- **Glitch animation untouched** — the cyan/pink text-shadow reads fine on both
  backgrounds (it's what makes the near-transparent "404" glyph visible).

### How the 404 page is reached for testing
The route is `/notfound`; any unmatched path (e.g. `/this-does-not-exist`)
redirects there. It is **auth-gated AND onboarding-gated** — you must register
AND create an org first, otherwise the app forces you back to `/onboarding` and
the 404 page never renders. Steps: register (`test@helix.ml` / `helixtest`),
create org `testorg`, then hit a bogus URL.

### Verification result
- TypeScript compiles clean: `npx tsc --noEmit` in the `helix-frontend-1`
  container returned exit 0. (The local `frontend` dir has no `node_modules`, so
  `yarn build`/`vite` can't run on the host — use the container.)
- Visual E2E in inner Helix confirmed:
  - **Light before** (`screenshots/00-404-light-before.png`): heading, body,
    Organizations and Go-back buttons are invisible (white-on-white) — the bug.
  - **Light after** (`screenshots/01-404-light-after.png`): all legible.
  - **Dark after** (`screenshots/02-404-dark-after.png`): identical to original.

### Environment gotcha (not part of this change)
On session start the API container failed to build (`missing go.sum entry for
prometheus/client_golang`) — a pre-existing issue in the checked-out `main`,
unrelated to this frontend-only change. Resolved for testing by running
`go mod download` / `go mod tidy` inside the `api` container (module cache was
stale); no committed go.mod/go.sum changes resulted. The `/app` dir in both the
`api` and `frontend` containers is bind-mounted to the host repo, so
`touch`-ing files to trigger Air/Vite affects host files — be careful not to
create stray empty files.
