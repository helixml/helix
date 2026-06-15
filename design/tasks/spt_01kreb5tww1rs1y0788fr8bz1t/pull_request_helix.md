# fix(frontend): repair light-mode contrast in queue header and sparkline tooltip

## Summary

Two unrelated but visually similar light-mode contrast bugs fixed together
because both are one-line palette-token swaps in adjacent UI surfaces.

- **Queue header in `RobustPromptInput`** sets its background to
  `primary.dark` / `info.dark` / `warning.dark` (intentionally dark in both
  palette modes) but the inner `<Typography>` and lucide icons inherited MUI's
  default body text color — near-black in light mode → dark text on a dark
  strip. Fixed by setting `color` on the same `Box` to the matching
  `.contrastText`, mirroring the established pattern used by the history hint
  ~80 lines below.

- **Hover tooltip on `UsageSparkline`** hard-codes a dark
  `backgroundColor: 'rgba(30, 30, 30, 0.95)'` for the popper `<Paper>`, while
  the typography inside uses `text.primary` / `text.secondary`. In light mode
  that paints the user's "black on dark gray" tooltip. The dashed vertical
  hover guideline was also hard-coded white at 50% opacity, invisible against
  the light card background. Fixed by routing both through `useLightTheme()`
  to switch the surface chrome and guideline color between modes — matching
  what the parent `ProjectsListView` already does for its own stat strip.

## Changes

- `frontend/src/components/common/RobustPromptInput.tsx`: add
  `color: editingId ? 'info.contrastText' : isOnline ? 'primary.contrastText' : 'warning.contrastText'`
  alongside the existing `bgcolor` conditional on the queue-header `Box`.
- `frontend/src/components/usage/UsageSparkline.tsx`: import `useLightTheme`,
  call it once in the body, then use `lightTheme.isLight` to switch the SVG
  hover-line stroke and the tooltip `<Paper>` background / border / shadow.

Why this is safe in dark mode: `.dark` and `.contrastText` are properties of
the palette *family*, not of `palette.mode`. They resolve to the same value
regardless of mode, so dark-mode contrast was already correct and stays
correct.

## Screenshots

Light mode — sparkline tooltip BEFORE (still rendering with the old hard-coded
dark surface, via DevTools):

![sparkline before](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kreb5tww1rs1y0788fr8bz1t/screenshots/03-sparkline-tooltip-light-before.png)

Light mode — sparkline tooltip AFTER:

![sparkline after](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kreb5tww1rs1y0788fr8bz1t/screenshots/02-sparkline-tooltip-light-after.png)

Dark mode — sparkline tooltip unchanged:

![sparkline dark](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kreb5tww1rs1y0788fr8bz1t/screenshots/04-sparkline-tooltip-dark.png)

Queue header — all four states compared (one BEFORE, three AFTER) using MUI's
default palette values:

![queue header states](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kreb5tww1rs1y0788fr8bz1t/screenshots/05-queue-header-states.png)

## Test plan

- [x] `yarn tsc` clean
- [x] Light-mode sparkline tooltip is legible end-to-end (real project + seeded
      usage data in the inner Helix)
- [x] Dark-mode sparkline tooltip unchanged
- [ ] Queue header end-to-end (requires a working agent runtime — inner Helix
      model registry was 401 in this environment; verified synthetically via
      DevTools mockup using the real MUI default palette `.dark` values)
