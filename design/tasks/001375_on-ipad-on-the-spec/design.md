# Design: iPad Text Selection & Comment Panel Fixes

## Problem Statement

Two issues on iPad for the spec review page:
1. **Text selection doesn't trigger comment panel**: Touch-based text highlighting doesn't open the comment form because the component only listens for `mouseup` events
2. **Comment panel obscures content**: The comment panel/bubble uses a hardcoded `left: '670px'` position, which can overlap the document on narrower screens (tablets, smaller viewports)

## Architecture

### Current Implementation

`DesignReviewContent.tsx`:
- Text selection handler: `handleTextSelection()` called on `onMouseUp`
- Selection triggers `setShowCommentForm(true)` with position from selection rect

`InlineCommentForm.tsx` and `InlineCommentBubble.tsx`:
- Hardcoded positioning: `left: '670px'`, `width: '300px'`
- Document content area: `maxWidth: '800px'`
- At 670px + 300px = 970px minimum needed, plus margins

### Solution

#### Fix 1: Add Touch Event Support

Add `onTouchEnd` handler alongside `onMouseUp` on the document container:

```tsx
<Box
  onMouseUp={handleTextSelection}
  onTouchEnd={handleTextSelection}
  sx={{ ... }}
>
```

The existing `handleTextSelection` function uses `window.getSelection()` which works for both mouse and touch selections—no changes needed to the handler itself.

#### Fix 2: Responsive Comment Panel Positioning

**Option A (Selected): Calculate position based on document container**

Pass a `contentRef` down to `InlineCommentForm` and `InlineCommentBubble`. Position the comment panel:
- Desktop (>1000px viewport): Position to the right of the document (`left: 670px` or calculated from content width + gap)
- Tablet/narrow (≤1000px): Position as a bottom sheet or overlay centered on screen

**Option B: Use CSS `calc()` with clamp**

```tsx
left: 'clamp(0px, calc(100% - 320px), 670px)'
```

This keeps the panel at 670px when space permits, otherwise pushes it left.

**Chosen: Option A** — Bottom sheet pattern is more touch-friendly on tablets and ensures text is never obscured.

## Key Decisions

1. **Reuse existing selection handler** — `window.getSelection()` works for touch; no need for separate touch logic
2. **Bottom sheet on narrow viewports** — Better UX than squishing panels side-by-side
3. **Breakpoint: 1000px** — Document (800px) + panel (300px) + minimal padding
4. **Use `useMediaQuery`** — Consistent with existing patterns in codebase (see `SpecTaskKanbanBoard.tsx`, `SpecTasksPage.tsx`)

## Files to Modify

| File | Change |
|------|--------|
| `DesignReviewContent.tsx` | Add `onTouchEnd={handleTextSelection}`, add responsive breakpoint detection, pass positioning props |
| `InlineCommentForm.tsx` | Accept `isNarrowViewport` prop, render as bottom sheet when narrow |
| `InlineCommentBubble.tsx` | Accept `isNarrowViewport` prop, adjust positioning |

## Edge Cases

- **Selection across multiple paragraphs on touch**: Works—`window.getSelection()` handles this
- **Virtual keyboard on iPad**: May need to account for keyboard height (existing pattern in `DesktopStreamViewer.tsx`)
- **Portrait vs landscape**: Both handled by viewport width breakpoint
- **Panel below visible area**: After showing the comment form, auto-scroll to ensure it's visible. Use `scrollIntoView({ behavior: 'smooth', block: 'nearest' })` on the panel element after render.

## Implementation Notes

### Touch Selection Fix
- Added `onTouchEnd={() => handleTextSelection(true)}` alongside the existing `onMouseUp`
- Modified `handleTextSelection(isTouch: boolean)` to accept a flag indicating touch vs mouse
- For touch events, added a 50ms `setTimeout` delay before processing the selection—iOS doesn't finalize text selection immediately on touchend
- The core selection logic using `window.getSelection()` works identically for both mouse and touch

### Responsive Positioning
- Used `useMediaQuery(theme.breakpoints.down(1000))` to detect narrow viewports
- Breakpoint of 1000px chosen because: document (800px max) + panel (300px) + padding
- `InlineCommentForm`: On narrow viewports, renders as a fixed bottom sheet (`position: fixed`, `bottom: 20px`, centered with `left: 50%` + `transform: translateX(-50%)`)
- `InlineCommentBubble`: On narrow viewports, renders inline with `position: relative` and full width, stacking vertically below the document
- Auto-scroll implemented via `useEffect` that calls `scrollIntoView({ behavior: 'smooth', block: 'nearest' })` when the form appears

### Files Modified
1. `frontend/src/components/spec-tasks/DesignReviewContent.tsx` - Touch handler, useMediaQuery, passing isNarrowViewport prop
2. `frontend/src/components/spec-tasks/InlineCommentForm.tsx` - Responsive positioning, auto-scroll
3. `frontend/src/components/spec-tasks/InlineCommentBubble.tsx` - Responsive positioning