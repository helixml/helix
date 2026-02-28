# Requirements: Mobile Pinch-to-Zoom Quality

## Problem Statement

When using pinch-to-zoom on mobile devices in the DesktopStreamViewer, the zoomed image appears grainier than expected. The root cause is that the canvas is rendered at the native video resolution (e.g., 1920x1080) but displayed at the mobile viewport size (e.g., ~390px wide on iPhone). When pinch-to-zoom scales up this already-downscaled CSS representation, users are zooming into an image that was already shrunk, not accessing the full native resolution.

## Root Cause Analysis

1. **Canvas internal resolution**: Set to native video resolution (1920x1080 or higher)
2. **Canvas CSS display size**: Calculated to fit mobile viewport while maintaining aspect ratio (e.g., 390x219)
3. **Pinch-to-zoom**: Applies CSS `transform: scale()` to the already-downscaled display

This means on a mobile device, a 1920px-wide video is first CSS-scaled down to ~390px, then when you zoom 2x, you're viewing a 780px representation of what should be 1920px. The browser doesn't have access to the original pixels - it's just scaling up the downscaled rendering.

## User Stories

1. **As a mobile user**, I want pinch-to-zoom to show me more detail from the native video resolution, not just a blown-up version of the scaled-down display.

2. **As a tablet user**, I want zoomed content to be as sharp as what I'd see on a desktop monitor at the same resolution.

## Acceptance Criteria

- [ ] When zoomed in, users should see detail from the native video resolution
- [ ] Text that is readable at native resolution should become readable when zoomed
- [ ] Works correctly on iOS Safari, Android Chrome, and desktop browsers
- [ ] No significant performance regression on mobile devices
- [ ] Existing pinch-to-zoom gesture functionality remains unchanged

## Technical Context

Current flow in `DesktopStreamViewer.tsx`:
- `websocket-stream.ts` renders frames to canvas at `frame.displayWidth x frame.displayHeight`
- Canvas CSS size is calculated via `canvasDisplaySize` to fit container
- Pinch-to-zoom applies `transform: scale(${zoomLevel})` to the canvas element
- The CSS transform operates on the rendered pixels, not the source data