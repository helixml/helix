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

### Code Changes

**DesktopStreamViewer.tsx - Canvas style (around line 4875):**

Current:
```tsx
width: canvasDisplaySize ? `${canvasDisplaySize.width}px` : "100%",
height: canvasDisplaySize ? `${canvasDisplaySize.height}px` : "100%",
transform: `translate(-50%, -50%) scale(${zoomLevel}) translate(${panOffset.x / zoomLevel}px, ${panOffset.y / zoomLevel}px)`,
```

Proposed:
```tsx
width: canvasDisplaySize ? `${canvasDisplaySize.width * zoomLevel}px` : "100%",
height: canvasDisplaySize ? `${canvasDisplaySize.height * zoomLevel}px` : "100%",
transform: `translate(calc(-50% + ${panOffset.x}px), calc(-50% + ${panOffset.y}px))`,
```

Apply same change to screenshot img element.

---

## Issue 2: Trackpad Edge-Pan Not Working

### Root Cause

Coordinate system mismatch in `handleTouchMove`:

1. **Cursor position** (`newX`/`newY`): Container screen coordinates (e.g., 0-390px on mobile)
2. **Viewport bounds** from `calculateVisibleViewportBounds()`: Content coordinates divided by zoom

When comparing cursor position against viewport bounds for edge detection, the coordinate systems don't match:

```tsx
// calculateVisibleViewportBounds() returns content coords
const visibleWidth = containerRect.width / zoomLevel;  // e.g., 195px at 2x zoom
const left = centerX - visibleWidth / 2 - panOffset.x / zoomLevel;

// But newX is in screen coords (0-390px range)
const distFromLeft = newX - viewportBounds.left;  // WRONG: mixing coord systems
```

At 2x zoom on a 390px container:
- `viewportBounds.left` might be ~97px (content coords)
- `newX` cursor at left edge is ~0px (screen coords)
- `distFromLeft` = 0 - 97 = -97px → incorrectly thinks cursor is way outside

### Solution: Use Screen Coordinates for Edge Detection

The edge-pan logic should work in screen coordinates since that's what the cursor uses. The visible viewport in screen coordinates is simply the container bounds (0 to containerWidth, 0 to containerHeight).

**Option A: Simplify to container bounds (Recommended)**

When zoomed, the visible viewport in screen coordinates IS the container. Edge detection should check distance from container edges, not from calculated viewport bounds:

```tsx
// In handleTouchMove, replace viewport bounds calculation with:
if (zoomLevel > 1 && containerRef.current) {
  const containerRect = containerRef.current.getBoundingClientRect();
  
  // Distance from container edges (screen coordinates)
  const distFromLeft = newX;
  const distFromRight = containerRect.width - newX;
  const distFromTop = newY;
  const distFromBottom = containerRect.height - newY;
  
  // Rest of edge detection logic unchanged...
}
```

**Option B: Convert cursor to content coordinates**

Alternatively, convert cursor position to content coordinates before comparison:
```tsx
const cursorContentX = (newX - containerRect.width/2) / zoomLevel + containerRect.width/2;
```

Option A is simpler and more intuitive.

### Code Changes

**DesktopStreamViewer.tsx - handleTouchMove (around line 3265):**

Current:
```tsx
if (zoomLevel > 1) {
  const viewportBounds = calculateVisibleViewportBounds();
  if (viewportBounds) {
    const distFromLeft = newX - viewportBounds.left;
    const distFromRight = viewportBounds.right - newX;
    // ...
  }
}
```

Proposed:
```tsx
if (zoomLevel > 1 && containerRef.current) {
  const containerRect = containerRef.current.getBoundingClientRect();
  
  // Use container edges directly (cursor is in container-relative coords)
  const distFromLeft = newX;
  const distFromRight = containerRect.width - newX;
  const distFromTop = newY;
  const distFromBottom = containerRect.height - newY;
  
  let panDirection = { x: 0, y: 0 };
  let maxIntensity = 0;
  
  if (distFromLeft < EDGE_PAN_ZONE_PX) {
    panDirection.x = 1; // Pan right (reveal content on left)
    maxIntensity = Math.max(maxIntensity, 1 - distFromLeft / EDGE_PAN_ZONE_PX);
  } else if (distFromRight < EDGE_PAN_ZONE_PX) {
    panDirection.x = -1; // Pan left (reveal content on right)
    maxIntensity = Math.max(maxIntensity, 1 - distFromRight / EDGE_PAN_ZONE_PX);
  }
  // ... similar for top/bottom
}
```

### Additional Fix: Remove calculateVisibleViewportBounds dependency

The `calculateVisibleViewportBounds()` function can be removed or simplified since edge-pan no longer needs it. Keep it only if used elsewhere.

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