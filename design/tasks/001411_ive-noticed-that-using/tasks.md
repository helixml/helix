# Implementation Tasks

## Issue 1: Pinch-to-Zoom Quality

- [x] Modify canvas element style in `DesktopStreamViewer.tsx` to use CSS width/height scaling instead of transform scale
  - Change `width: canvasDisplaySize.width * zoomLevel` instead of `transform: scale(zoomLevel)`
  - Change `height: canvasDisplaySize.height * zoomLevel` similarly
  - Update transform to only handle centering and pan offset (no scale)
- [x] Ensure container has `overflow: hidden` to clip the enlarged canvas when zoomed
- [x] Update pan offset logic to work with the new CSS-size-based zoom approach
- [x] Apply same changes to screenshot img element for consistency in screenshot mode
- [ ] Test on iOS Safari - verify zoomed text is sharper than before
- [ ] Test on Android Chrome - verify zoomed text is sharper than before
- [ ] Verify pan gestures work correctly with new implementation
- [ ] Check GPU/performance impact at high zoom levels on mobile devices

## Issue 2: Trackpad Edge-Pan Fix

- [x] Fix coordinate system mismatch in `handleTouchMove` edge detection (around line 3265)
  - Replace `calculateVisibleViewportBounds()` usage with direct container edge detection
  - Use `distFromLeft = newX` instead of `newX - viewportBounds.left`
  - Use `distFromRight = containerRect.width - newX` instead of `viewportBounds.right - newX`
  - Same for top/bottom edges
- [ ] Test trackpad mode edge-pan on tablet while zoomed:
  - Cursor at left edge should pan view left (reveal content on left)
  - Cursor at right edge should pan view right (reveal content on right)
  - Same for top/bottom
- [ ] Verify edge-pan stops when touch ends
- [ ] Verify edge-pan stops when cursor moves away from edge zone
- [x] Consider removing or simplifying `calculateVisibleViewportBounds()` if no longer needed elsewhere
  - Removed the function entirely as it was no longer used after the edge-pan fix