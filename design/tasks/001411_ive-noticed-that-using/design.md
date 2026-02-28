# Design: Mobile Pinch-to-Zoom Quality Fix

## Overview

The grainy appearance when pinch-to-zooming on mobile is caused by zooming into an already-downscaled CSS representation of the video, rather than accessing the native resolution pixels.

## Root Cause

The rendering pipeline on mobile:

1. **Canvas internal buffer**: 1920x1080 (native video resolution)
2. **Canvas CSS display**: ~390x219 (fit to mobile viewport)
3. **Pinch-to-zoom**: CSS `transform: scale(2)` → 780x438 CSS pixels

The problem: CSS transform scaling operates on the *rendered* pixels (390px wide), not the canvas's internal buffer (1920px wide). The browser has already downsampled to fit the viewport, so zooming just magnifies the downsampled result.

## Solution Options

### Option A: Adjust Canvas CSS Size Based on Zoom (Recommended)

Instead of using CSS transform to zoom, increase the canvas's CSS display size proportionally to zoom level, then pan within a clipped container.

**How it works:**
- At zoom 1x: Canvas CSS = 390x219 (fit to container)
- At zoom 2x: Canvas CSS = 780x438 (larger than container, clipped)
- Pan by adjusting position within the overflow-hidden container

**Pros:**
- Browser renders more pixels from the native resolution buffer
- No change to video streaming or decoding
- Relatively simple CSS change

**Cons:**
- Larger CSS size means more GPU work for compositing
- Need to rework pan/offset logic

### Option B: Use CSS `image-rendering: pixelated`

Apply `image-rendering: pixelated` to get crisp pixel scaling.

**Pros:** One-line fix

**Cons:** Doesn't solve the actual problem—still zooming into downscaled pixels, just with nearest-neighbor interpolation instead of bilinear. Text will look blocky, not sharp.

### Option C: Viewport-Aware Resolution Switching

Request lower resolution video when viewport is small, higher when zoomed.

**Pros:** Optimal bandwidth usage

**Cons:** Complex, requires server-side changes, introduces latency on zoom

## Recommended Approach: Option A

Modify the zoom implementation to scale the canvas CSS dimensions rather than using CSS transform.

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

The container should have `overflow: hidden` to clip the enlarged canvas.

### Why This Works

When `canvasDisplaySize.width * zoomLevel` exceeds the container width, the browser must render more pixels from the canvas's internal 1920px buffer to fill the larger CSS area. This gives access to the native resolution detail.

Example at 2x zoom:
- Canvas CSS width: 780px
- Canvas internal: 1920px
- Browser samples ~2.5 internal pixels per CSS pixel (still downsampling, but less than 1x zoom's ~5 pixels per CSS pixel)

At ~5x zoom on a 390px viewport, the CSS width would be 1950px, nearly 1:1 with the native resolution.

## Testing

1. Open stream on mobile device
2. Zoom to 2x - text should be noticeably sharper than current implementation
3. Zoom to max - should approach native resolution clarity
4. Verify pan gestures still work correctly
5. Check performance (GPU usage) at high zoom levels