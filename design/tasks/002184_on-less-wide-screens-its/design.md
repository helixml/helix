# Design: Make Comment Resolve Button Discoverable on Narrow Screens

## Current State (from code review)

Two places render the Resolve control as a bare `IconButton` with a
`CheckCircleIcon` and `color: 'success.main'`, no tooltip, no label:

- `frontend/src/components/spec-tasks/InlineCommentBubble.tsx:122-132` — header
  `Box` (`justifyContent="space-between"`) with a `<Chip label="Comment">` on the
  left and the resolve `IconButton` on the right. The component already receives
  an `isNarrowViewport` prop and switches the bubble between absolute (wide) and
  relative/stacked (narrow) layouts.
- `frontend/src/components/spec-tasks/CommentLogSidebar.tsx:69-80` — same
  pattern, resolve `IconButton` only rendered when `!comment.resolved`.

Both are wired to a resolve handler (`onResolve` / `onResolveComment`) that calls
`useResolveComment` in `DesignReviewContent.tsx` (`handleResolveComment`). The
behaviour itself is fine — the problem is purely discoverability of the control.

`isNarrowViewport` is computed in `DesignReviewContent.tsx:111` via
`useMediaQuery(theme.breakpoints.down(1000))` and passed into
`InlineCommentBubble`. `CommentLogSidebar` does not currently receive it.

### Root cause of the horizontal scroll (review feedback)

The wide-layout bubble is absolutely positioned **outside** the document column:

```js
const wideStyles = { position: "absolute", left: "820px", width: "300px", ... };
```

`left: 820px` is relative to the markdown column, which is `maxWidth: 800px` and
horizontally centred (`mx: "auto"`) inside the flex document area. So the bubble's
right edge is at `columnLeftMargin + 820 + 300`. Because the column is centred,
`columnLeftMargin` grows as the area widens, which means the bubble needs the
viewport to be **well above 1120px** (closer to ~1440px once centring margins are
included) before it fits without overflow. Yet the stacked layout only kicks in
below **1000px** (`breakpoints.down(1000)`).

The result: across a wide medium band, the side-positioned bubble — and the
Resolve button at its top-right — sits past the right edge of the viewport and is
only reachable by scrolling horizontally. The label change improves
discoverability, but the horizontal-scroll symptom (AC-6) is fixed by making the
in-flow stacked layout engage earlier so the off-document panel is never shown at
a width that can't contain it.

## Approach

Two complementary changes:

**A. Stop the horizontal scroll (the primary symptom).** Raise the
`isNarrowViewport` threshold in `DesignReviewContent.tsx:111` so the stacked,
in-flow, full-width layout engages before the side-positioned bubble can overflow
the viewport. The side layout needs the centred 800px column plus a 300px panel
offset at `left: 820px` plus centring margins — so a threshold around
`breakpoints.down(1280)` (up from 1000) keeps the stacked layout active through
the whole band where the panel would otherwise be pushed off-screen. Pick the
value to comfortably clear `820 + 300` plus typical centring margin; ~1280px is a
safe, simple choice. This is a one-line change and removes the "had to scroll
sideways to find it" problem at its root.

**B. Make the control obvious (discoverability).** Replace the bare icon button
with a clearly labeled Resolve control. Keep it minimal — match the existing MUI
usage already imported in these files.

### Decision: labeled MUI `Button` + tooltip (chosen)

Use MUI `Button` with `startIcon={<CheckCircleIcon />}` and the text "Resolve",
`size="small"`, `color="success"`. This gives an always-visible text label that
is obvious on every screen size and reads naturally in the stacked narrow layout
where there is full width available.

- Wide viewport: a small "Resolve" button sits top-right of the header, same
  position as today's icon. Header is `space-between`, so the `Chip` stays left
  and the button stays right.
- Narrow viewport: the bubble is full width, so the labeled button has plenty of
  room and is immediately discoverable (satisfies AC-2).

Also wrap with a `Tooltip title="Resolve comment"` for the hover affordance
(satisfies AC-1/US-2 even if a future variant goes icon-only).

### Alternatives considered

- **Tooltip only, keep icon-only button.** Smallest change, but a tooltip
  requires hover/focus discovery and does nothing for touch users on narrow
  screens — doesn't really solve "I can't find it". Rejected as insufficient for
  AC-2.
- **Icon-only on wide, labeled on narrow (conditional via `isNarrowViewport`).**
  Possible, but adds branching for little benefit; a small labeled button is fine
  on wide screens too and keeps the two render paths identical. We will just use
  the labeled button everywhere for consistency and simplicity.

## Changes

0. `DesignReviewContent.tsx`
   - Raise the `isNarrowViewport` breakpoint from `down(1000)` to ~`down(1280)`
     so the stacked layout engages before the side bubble overflows (fixes the
     horizontal scroll, AC-6). Update the adjacent comment that explains the
     1000px figure to reflect the new reasoning (panel offset `820px` + width
     `300px` + centring margin).

1. `InlineCommentBubble.tsx`
   - Replace the resolve `IconButton`/`CheckCircleIcon` block (lines ~129-131)
     with a `Tooltip`-wrapped `Button` (`size="small"`, `color="success"`,
     `startIcon={<CheckCircleIcon fontSize="small" />}`, label "Resolve").
   - `Button` is already imported; `Tooltip` import to be added from
     `@mui/material`.

2. `CommentLogSidebar.tsx`
   - Apply the same labeled `Button` (only when `!comment.resolved`, preserving
     existing condition).
   - Add `Button` and `Tooltip` to the `@mui/material` import.

No prop or API changes are required: both components keep their existing
`onResolve` / `onResolveComment` callbacks. `isNarrowViewport` does not strictly
need to be threaded into `CommentLogSidebar` because the labeled button is used
unconditionally; we will not add new props.

## Testing

- Build the frontend (`cd frontend && yarn build`).
- Verify end-to-end in the inner Helix: open a spec task in review with at least
  one inline comment, confirm the "Resolve" label is visible on the bubble and in
  the comment log, at both a wide window and a narrow (<1000px) window, and that
  clicking it resolves the comment (snackbar "Comment resolved", bubble
  disappears / shows resolved state).

## Notes / Gotchas

- The narrow-viewport stacked layout already exists and is driven by
  `isNarrowViewport`; this task only changes the Resolve control, not the
  stacking logic.
- Keep `e`-handlers identical — no `stopPropagation` is currently used here and
  none is needed; the bubble has no row-click behaviour.
