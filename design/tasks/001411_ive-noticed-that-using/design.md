# Design: Mobile Pinch-to-Zoom Quality & Trackpad Edge-Pan Fix

## Overview

Two related issues with mobile/touch interaction in DesktopStreamViewer:
1. **Zoom quality**: Pinch-to-zoom produces grainy results because we're scaling an already-downscaled CSS representation
2. **Edge-pan broken**: Trackpad mode edge-pan doesn't work due to coordinate system mismatch

## Issue 1: Pinch-to-Zoom Quality

### Root Cause

The rendering pipeline on mobile:

1. **Canvas internal buffer**: 1920x1080 (native video resolution)
2. **Canvas CSS display**: ~390x219 (fit to mobile viewport)
3. **Pinch-to-zoom**: CSS `transform: scale(2)` → 780x438 CSS pixels

The problem: CSS transform scaling operates on the *rendered* pixels (390px wide), not the canvas's internal buffer (1920px wide). The browser has already downsampled to fit the viewport, so zooming just magnifies the downsampled result.

### Solution: Scale Canvas CSS Size Instead of Transform

Instead of using CSS transform to zoom, increase the canvas's CSS display size proportionally to zoom level, then pan within a clipped container.

**How it works:**
- At zoom 1x: Canvas CSS = 390x219 (fit to container)
- At zoom 2x: Canvas CSS = 780x438 (larger than container, clipped)
- Pan by adjusting position within the overflow-hidden container

**Why this works:** When `canvasDisplaySize.width * zoomLevel` exceeds the container width, the browser must render more pixels from the canvas's internal 1920px buffer to fill the larger CSS area.

---

## Issue 2: Trackpad Edge-Pan Not Working

### Root Cause

Coordinate system mismatch in `handleTouchMove`:

1. **Cursor position** (`newX`/`newY`): Container screen coordinates (e.g., 0-390px on mobile)
2. **Viewport bounds** from `calculateVisibleViewportBounds()`: Content coordinates divided by zoom

When comparing cursor position against viewport bounds for edge detection, the coordinate systems don't match.

### Solution: Use Screen Coordinates for Edge Detection

When zoomed, the visible viewport in screen coordinates IS the container. Edge detection should check distance from container edges directly:

```tsx
const distFromLeft = newX;
const distFromRight = containerRect.width - newX;
const distFromTop = newY;
const distFromBottom = containerRect.height - newY;
```

---

## Implementation Notes

### Files Modified
- `frontend/src/components/external-agent/DesktopStreamViewer.tsx`

### Key Changes Made

1. **Canvas element style (line ~4872)**
   - Changed `width` from `canvasDisplaySize.width` to `canvasDisplaySize.width * zoomLevel`
   - Changed `height` from `canvasDisplaySize.height` to `canvasDisplaySize.height * zoomLevel`
   - Changed `transform` from `scale(${zoomLevel})` to just centering + pan offset

2. **Container element (line ~4236)**
   - Added `overflow: "hidden"` to clip zoomed content

3. **Screenshot img element (line ~4910)**
   - Applied same CSS size-based zoom approach as canvas

4. **Pan offset bounds calculation (lines ~3151, ~3401)**
   - Updated `maxPanX`/`maxPanY` to use: `(scaledCanvasWidth - containerRect.width) / 2`
   - Added `canvasDisplaySize` to useCallback dependencies

5. **Edge-pan detection (line ~3273)**
   - Replaced `calculateVisibleViewportBounds()` with direct container edge detection
   - Uses `newX`/`newY` directly against container dimensions

6. **Cleanup**
   - Removed unused `calculateVisibleViewportBounds()` function
   - Removed it from `handleTouchMove` dependencies

### Gotchas Discovered
- The pan offset calculation needed updating because the relationship between zoom and pan bounds changed
- Old formula: `maxPanX = containerRect.width * (zoomLevel - 1) / 2`
- New formula: `maxPanX = (canvasDisplaySize.width * zoomLevel - containerRect.width) / 2`

---

## Testing

### Zoom Quality
1. Open stream on mobile device
2. Zoom to 2x - text should be noticeably sharper than current implementation
3. Zoom to max - should approach native resolution clarity

### Edge-Pan
1. Enable trackpad mode on tablet
2. Pinch to zoom in (2x or more)
3. Drag cursor to left edge - view should pan left to reveal more content
4. Test all four edges
5. Verify pan stops when finger lifts or cursor moves away from edge