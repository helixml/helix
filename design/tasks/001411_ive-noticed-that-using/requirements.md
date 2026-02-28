# Requirements: Mobile Pinch-to-Zoom Quality & Trackpad Edge-Pan

## Problem Statement

Two related issues with mobile/touch interaction in DesktopStreamViewer:

### Issue 1: Pinch-to-Zoom Quality

When using pinch-to-zoom on mobile devices, the zoomed image appears grainier than expected. The root cause is that the canvas is rendered at the native video resolution (e.g., 1920x1080) but displayed at the mobile viewport size (e.g., ~390px wide on iPhone). When pinch-to-zoom scales up this already-downscaled CSS representation, users are zooming into an image that was already shrunk, not accessing the full native resolution.

### Issue 2: Trackpad Mode Edge-Pan Not Working

In emulated trackpad mode, when the cursor reaches the edge of the visible viewport while zoomed in, the screen should auto-pan to follow the cursor. This edge-pan feature is not working correctly due to a coordinate system mismatch:

- `calculateVisibleViewportBounds()` returns bounds in "content coordinates" (divided by zoomLevel)
- Cursor position (`newX`/`newY`) is in container screen coordinates
- These coordinate systems don't align when zoomed, causing edge detection to fail

## Root Cause Analysis

### Zoom Quality Issue

1. **Canvas internal resolution**: Set to native video resolution (1920x1080 or higher)
2. **Canvas CSS display size**: Calculated to fit mobile viewport while maintaining aspect ratio (e.g., 390x219)
3. **Pinch-to-zoom**: Applies CSS `transform: scale()` to the already-downscaled display

This means on a mobile device, a 1920px-wide video is first CSS-scaled down to ~390px, then when you zoom 2x, you're viewing a 780px representation of what should be 1920px.

### Edge-Pan Issue

The `calculateVisibleViewportBounds()` function calculates viewport bounds like this:
```
visibleWidth = containerRect.width / zoomLevel
left = centerX - visibleWidth/2 - panOffset.x/zoomLevel
```

But cursor position is calculated and clamped in screen coordinates without zoom adjustment:
```
newX = clamp(currentPos.x + scaledDx, streamOffsetX, streamOffsetX + rect.width)
```

When comparing `newX` (screen coords) against `viewportBounds.left` (content coords), the edge detection math is wrong.

## User Stories

1. **As a mobile user**, I want pinch-to-zoom to show me more detail from the native video resolution, not just a blown-up version of the scaled-down display.

2. **As a tablet user using trackpad mode**, I want the view to automatically pan when I move the cursor to the edge of the screen while zoomed in.

## Acceptance Criteria

### Zoom Quality
- [ ] When zoomed in, users should see detail from the native video resolution
- [ ] Text that is readable at native resolution should become readable when zoomed
- [ ] Works correctly on iOS Safari, Android Chrome, and desktop browsers

### Edge-Pan
- [ ] In trackpad mode while zoomed, moving cursor to edge triggers smooth auto-pan
- [ ] Edge-pan works in all four directions (left, right, top, bottom)
- [ ] Edge-pan stops when cursor moves away from edge or touch ends
- [ ] Pan direction is correct (cursor at right edge pans view right to reveal more content)

### General
- [ ] No significant performance regression on mobile devices
- [ ] Existing pinch-to-zoom gesture functionality remains unchanged