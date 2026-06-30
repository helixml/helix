# Make comment Resolve button discoverable on narrow screens

## Summary

On less-wide screens the spec-review comment's Resolve action was hard to find,
and often required a horizontal scroll to reach it. The action was a bare,
unlabeled green check-circle icon button, and the side-positioned comment bubble
overflowed off the right edge of the document area in a wide band of viewport
widths.

This makes the Resolve action obvious and always reachable without horizontal
scrolling.

## Changes

- **`InlineCommentBubble.tsx` / `CommentLogSidebar.tsx`** — replaced the bare
  `IconButton` (check-circle) with a `Tooltip`-wrapped, labeled MUI `Button`
  ("Resolve", green, check-circle icon). Resolve behaviour is unchanged. Removed
  the now-unused `IconButton` import.
- **`DesignReviewContent.tsx`** — fixed the layout decision that caused the
  horizontal scroll. The floating side bubble is absolutely positioned in the
  centred document column's right gutter, which only fits on very wide screens.
  Replaced the window-based `useMediaQuery` check with a `ResizeObserver` that
  measures the actual document-area width and switches to the stacked, in-flow
  comment layout below ~1460px. This is correct for both surfaces that render
  this component (standalone review page and embedded workspace tab), and also
  stacks correctly when the comment-log sidebar is open. Removed unused
  `useMediaQuery` / `useTheme`.

## Testing

Verified live in a local Helix dev stack (seeded a design review + inline
comment):
- 1300px: comment stacked below the document, no horizontal scroll, labeled
  "Resolve" clearly visible.
- 1600px: side panel layout, no overflow, Resolve button fully within the
  viewport.
- Clicking Resolve resolves the comment (POST `.../resolve` → 200, bubble
  removed, unresolved count cleared).
- `tsc --noEmit` clean; production `vite build` succeeds.

## Screenshots

![Medium width — stacked, labeled Resolve, no horizontal scroll](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002184_on-less-wide-screens-its/screenshots/02-narrow-stacked.png)
![Wide — side panel, Resolve fully visible](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002184_on-less-wide-screens-its/screenshots/03-wide-side-panel.png)
![After resolving — comment cleared](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002184_on-less-wide-screens-its/screenshots/04-after-resolve.png)
