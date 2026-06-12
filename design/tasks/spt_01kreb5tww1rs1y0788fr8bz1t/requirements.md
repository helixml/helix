# Requirements: Fix light-mode contrast bugs in queue header and usage sparkline tooltip

Two unrelated but visually similar light-mode contrast bugs, fixed together
because they're both one-line palette-token swaps in adjacent UI surfaces.

## Bug 1 — Message queue header text

In the prompt input's message-queue header, the text "Message queue (saved locally)"
(and the sibling strings "Editing - paused from here" and "Offline - saved locally,
will send when connected") renders as dark text on a dark background when the app
is in **light mode**, making it effectively unreadable.

The header `Box` has its background set to `primary.dark` / `info.dark` /
`warning.dark` (palette tokens that resolve to dark shades in both modes), but the
nested `<Typography>` and lucide icons inherit the default body text color — which
is dark in light mode.

## Bug 2 — Usage sparkline hover tooltip on project cards

When the user hovers over the per-card token-usage sparkline on the Projects page
(cards view), a tooltip popper appears showing the date, token count, and request
count for that day. The tooltip's `<Paper>` hard-codes
`backgroundColor: 'rgba(30, 30, 30, 0.95)'` (dark gray) but the text inside uses
the MUI palette tokens `text.primary` and `text.secondary`, which resolve to near-
black / dark gray in light mode. Result: dark text on dark gray background —
unreadable. The vertical hover guideline drawn on the sparkline is also hard-coded
`rgba(255,255,255,0.5)`, which is effectively invisible against the light card
background in light mode.

## User stories

- **As a user on a light-themed Helix instance**, I want the message-queue
  header text to be legible so I can see what state my queue is in without
  switching to dark mode.
- **As a user on a light-themed Helix instance**, I want the sparkline hover
  tooltip on project cards to be legible (and the hover guideline to be visible)
  so I can actually read the per-day usage numbers.

## Acceptance criteria

### Bug 1 (queue header)
- In **light mode**, the queue header label and its leading icon (`Cloud` /
  `CloudOff` / `CirclePause`) are clearly readable against the dark
  `primary.dark` / `warning.dark` / `info.dark` background.
- All three header states are covered:
  - online + not editing → `"Message queue (saved locally)"` on `primary.dark`
  - offline → `"Offline - saved locally, will send when connected"` on `warning.dark`
  - editing → `"Editing - paused from here"` on `info.dark`
- The count `<Chip>` next to the label remains legible in both modes.
- The existing **dark mode** appearance is unchanged.

### Bug 2 (sparkline tooltip)
- In **light mode**, hovering over a project card's sparkline shows a tooltip
  whose date, "Tokens:" / "Requests:" labels, and numeric values are all
  clearly readable.
- The vertical dashed hover guideline drawn over the sparkline is visible
  against the card's stat-strip background in both light and dark mode.
- The existing **dark mode** appearance is unchanged.

## Out of scope

- No restyling of the queue items themselves (the rows under the header).
- No restyling of the sparkline curve itself (the green polyline / gradient
  area) — the color is brand-correct and visible in both modes.
- No change to detection / offline / data-loading logic.
