# Tweaks to on-hover add comment button on spec review page

## Summary

Four tweaks to the on-hover "Add comment" button and pseudo-highlight on the spec review page (`DesignReviewContent.tsx`):

1. Clicking the hover button now applies the same blue pseudo-highlight to the paragraph that manual selection does, so users can see exactly which block the comment is attached to.
2. The hover button now disappears when the cursor moves to the right past the button's right edge (previously it lingered until the cursor left the entire scroll container). Clicking the button is unaffected.
3. Pseudo-highlights spanning a code block are now visible across the code: dropped the conflicting `color: #000` from the `::highlight()` rule that was hiding under Prism's inline syntax-token colours, leaving `background-color: #b3d7ff` to paint correctly.
4. The hover button no longer appears when the cursor is over an existing inline comment panel (`InlineCommentBubble`).

All changes are in `frontend/src/components/spec-tasks/DesignReviewContent.tsx`.

## Changes

- `onClick` of the hover button now creates a `Range` over `hoveredElementRef.current` and assigns it to `savedRangeRef.current` so the existing `useEffect` applies the pseudo-highlight when the comment form opens.
- Added an `onMouseMove` handler to the outer scroll container that clears `hoverButtonPosition` when the cursor x-position exceeds the button's right edge (`containerWidth/2 + 432px`).
- Added an early-return in the inner Box's `onMouseMove` handler that clears the hover button when the cursor is inside any element tracked in `commentRefs.current`.
- Dropped `color: #000` from the `::highlight(comment-highlight)` `GlobalStyles` rule — Prism's inline `color: rgb(...)` styles win regardless, and removing the override lets syntax colours show through under the highlight background.

## Screenshots

![Highlight after fix - spans code block correctly](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001678_tweaks-to-the-new-on/screenshots/02-highlight-after-fix.png)
![Hover button visible on paragraph hover](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001678_tweaks-to-the-new-on/screenshots/03-hover-button-visible.png)
![Paragraph highlighted after clicking hover button](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001678_tweaks-to-the-new-on/screenshots/04-after-hover-button-click.png)
![No hover button when cursor over comment panel](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001678_tweaks-to-the-new-on/screenshots/05-hover-over-comment-panel.png)

## Test plan

- [x] Manual browser test in inner Helix: hover button click applies highlight
- [x] Manual browser test: cursor past right edge hides button (programmatic mousemove dispatch confirmed `stillVisibleAtButton:true` when cursor inside button rect, button cleared when cursor at x=1100 vs button right edge 1040)
- [x] Manual browser test: highlight spans code block with syntax colours preserved
- [x] Manual browser test: hovering over a comment panel hides the hover button
- [x] `cd frontend && yarn build` passes
