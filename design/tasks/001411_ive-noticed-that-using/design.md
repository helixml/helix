# Design: Mobile Pinch-to-Zoom Quality Fix

## Overview

Fix the grainy/blurry appearance when using pinch-to-zoom on mobile by applying appropriate CSS `image-rendering` properties to prevent browser interpolation during CSS transform scaling.

## Root Cause

The `DesktopStreamViewer.tsx` component uses CSS `transform: scale()` for pinch-to-zoom. By default, browsers apply bilinear interpolation when scaling, which causes:
- Blurry edges on text and UI elements
- Loss of sharp pixel boundaries
- "Grainy" appearance that doesn't match the actual video resolution

## Solution

Add `image-rendering: pixelated` CSS property to both rendering elements:

1. **Canvas element** (video mode) - line ~4882
2. **Screenshot img element** (screenshot mode) - line ~4921

The `pixelated` value tells the browser to use nearest-neighbor scaling, preserving sharp pixel edges when zooming in.

## Code Changes

### DesktopStreamViewer.tsx

**Canvas element style (around line 4882):**
```tsx
transform: `translate(-50%, -50%) scale(${zoomLevel}) translate(${panOffset.x / zoomLevel}px, ${panOffset.y / zoomLevel}px)`,
transformOrigin: "center center",
imageRendering: zoomLevel > 1 ? "pixelated" : "auto",  // ADD THIS
```

**Screenshot img element style (around line 4921):**
```tsx
transform: zoomLevel > 1
  ? `scale(${zoomLevel}) translate(${panOffset.x / zoomLevel}px, ${panOffset.y / zoomLevel}px)`
  : undefined,
transformOrigin: "center center",
imageRendering: zoomLevel > 1 ? "pixelated" : "auto",  // ADD THIS
```

## Design Decisions

### Why `pixelated` over `crisp-edges`?

- `pixelated`: Nearest-neighbor scaling, intentionally blocky - ideal for pixel-precise content
- `crisp-edges`: Browser-dependent, may still blur
- For remote desktop viewing where users zoom to read text, sharp pixels are preferred

### Why conditional on `zoomLevel > 1`?

At 1x zoom (no zoom), we want normal rendering. Only apply pixelated rendering when actively zoomed in, avoiding any potential visual artifacts at native resolution.

## Browser Support

- `image-rendering: pixelated` is supported in all modern browsers
- Safari uses `-webkit-image-rendering: pixelated` but standard property works
- No polyfill needed

## Testing

1. Open desktop stream on mobile device (iOS Safari, Android Chrome)
2. Pinch to zoom in on text
3. Verify text edges are sharp/pixelated, not blurry
4. Verify no visual issues at 1x zoom (no zoom active)