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

## Implementation Notes (post-implementation)

What actually shipped, and what changed from the plan:

1. **Resolve control → labeled button (as planned).** `InlineCommentBubble.tsx`
   and `CommentLogSidebar.tsx` now render a `Tooltip`-wrapped MUI `Button`
   (`size="small"`, `color="success"`, `startIcon={<CheckCircleIcon/>}`, label
   "Resolve", `textTransform: none`, `flexShrink: 0`) instead of a bare
   `IconButton`. The now-unused `IconButton` import was removed from both files.

2. **Narrow/wide decision: container measurement, NOT a window breakpoint
   (changed from the plan).** The plan said "raise the `useMediaQuery` breakpoint
   to ~1280". During implementation, live testing showed that was still wrong:
   - The wide bubble is `position:absolute; left:820px; width:300px` relative to
     the **centred** 800px document column. Its right edge is
     `(docAreaWidth - 800)/2 + 1120`, so it only stops overflowing once the
     document area is ~1440px wide (empirically confirmed: overflow at doc-area
     ~1336px, no overflow at 1440/1536). At a 1400px *window* it still overflowed.
   - More importantly, `DesignReviewContent` renders in **two** places —
     `SpecTaskReviewPage` (standalone) and the workspace `TabsView` (embedded) —
     which have different chrome widths at the same window size. A window-based
     media query cannot be correct for both.
   - Fix: a `ResizeObserver` on `documentRef` tracks the real document-area
     `clientWidth`; `isNarrowViewport = width < 1460` (≈1440 overflow boundary +
     small gutter). Defaults to stacked until measured to avoid an off-screen
     flash. This also correctly stacks when the comment-log sidebar (400px) is
     open, which the window query missed. Removed `useMediaQuery` and `useTheme`.

3. **Gotcha for future agents — testing the review UI in inner Helix.** The
   planning agent / LLM is NOT needed to see a design review. Seed it directly:
   - Register + onboard (creates user + org). The pre-existing `default` project
     has empty `organization_id` AND empty `user_id`, so spec-task auth
     (`authorizeUserToProjectByID` → `authorizeUserToProject`) returns **403**
     and the review page hangs on a spinner. Set the project's `user_id` to your
     user id (old-style user-owned project passes auth).
   - Insert a `spec_tasks` row (only `id` is NOT NULL; set `project_id`,
     `user_id`, `organization_id`, `status='spec_review'`), then a
     `spec_task_design_reviews` row (`status='in_review'`, fill
     `requirements_spec` etc. with `E'...\n...'` for real newlines), then a
     `spec_task_design_review_comments` row with a `quoted_text` that is a
     substring of `requirements_spec` so the bubble anchors.
   - Route: `/orgs/{orgId}/projects/{projectId}/tasks/{taskId}/review/{reviewId}`.

## Verification (done)

Verified live in inner Helix at `localhost:8080` (dev mode, Vite HMR). Screenshots
in `screenshots/`: `02-narrow-stacked.png` (1300px — stacked, no scroll, labeled
Resolve), `03-wide-side-panel.png` (1600px — side panel, no overflow),
`04-after-resolve.png` (after clicking Resolve — POST `.../resolve` 200, bubble
removed, unresolved count cleared). `tsc --noEmit` clean; full `vite build`
succeeds (in-repo `dist` is a root-owned prod bind-mount, so built to a temp dir).
