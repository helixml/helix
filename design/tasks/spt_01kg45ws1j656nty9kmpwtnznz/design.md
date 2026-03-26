# Design: Comment Box Overlap Fix

## Root Cause

Both `InlineCommentBubble` and `InlineCommentForm` use this wide-viewport style:

```tsx
// InlineCommentBubble.tsx:96-103 and InlineCommentForm.tsx:56-63
const wideStyles = {
  position: "absolute" as const,
  left: "670px",   // ← BUG: 670 + 300 (width) = 970px, but doc is 800px wide
  top: `${yPos}px`,
  width: "300px",
};
```

These components are rendered inside the inner `Box` in `DesignReviewContent.tsx` which has:
```tsx
sx={{ maxWidth: "800px", mx: "auto", position: "relative" }}
```

Since comments are `position: absolute` within this 800px box, `left: "670px"` places the comment panel starting 130px inside the right edge of the document — always overlapping.

## Fix

Change `left: "670px"` to `left: "820px"` in both components. This places the comment 20px to the right of the 800px document edge, with no overlap.

```tsx
const wideStyles = {
  position: "absolute" as const,
  left: "820px",   // 800px doc + 20px gap
  top: `${yPos}px`,
  width: "300px",
};
```

The parent `documentRef` Box has `overflow: "auto"`, so comments at 820px will render correctly on wide viewports (where there is horizontal room) and scroll horizontally if needed on intermediate widths (though this is already handled by the `isNarrowViewport` check which switches to a non-overlapping layout at ≤ 1000px).

## Files to Change

- `frontend/src/components/spec-tasks/InlineCommentBubble.tsx` — `wideStyles.left`
- `frontend/src/components/spec-tasks/InlineCommentForm.tsx` — `wideStyles.left`

## Pattern Note

This codebase uses a hardcoded `isNarrowViewport` breakpoint at 1000px (`theme.breakpoints.down(1000)`) with the comment `1000px = document (800px) + panel (300px) + minimal padding`. The hardcoded `left` pixel values must be consistent with this layout contract. The correct starting position for the side panel is `800px + gap`, not `670px`.
