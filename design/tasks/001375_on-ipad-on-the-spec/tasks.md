# Implementation Tasks

## Issue 1: Touch Selection Not Opening Comment Panel

- [ ] Add `onTouchEnd` handler alongside `onMouseUp` in `DesignReviewContent.tsx` (line ~834)
- [ ] Ensure `handleTextSelection()` works correctly with touch events (test `window.getSelection()` on touch)
- [ ] Add small delay (~50ms) before checking selection to allow iOS to finalize text selection

## Issue 2: Comment Panel Obscures Text on iPad

- [ ] Add `useMediaQuery` hook to detect mobile/tablet viewport in `DesignReviewContent.tsx`
- [ ] Update `InlineCommentForm.tsx` positioning:
  - Desktop: Keep `left: '670px'` (right side of content)
  - Mobile/tablet: Position below selected text or as bottom sheet
- [ ] Update `InlineCommentBubble.tsx` positioning:
  - Desktop: Keep `left: '670px'`
  - Mobile/tablet: Full-width overlay or bottom sheet pattern
- [ ] Consider reducing comment panel width on small screens (300px → responsive width)

## Testing

- [ ] Test on iPad Safari: highlight text → verify comment panel opens
- [ ] Test on iPad Safari: verify comment panel doesn't obscure the highlighted text
- [ ] Test on desktop: verify existing behavior unchanged