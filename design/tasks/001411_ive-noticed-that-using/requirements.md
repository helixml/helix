# Requirements: Mobile Pinch-to-Zoom Quality

## Problem Statement

When using pinch-to-zoom on mobile devices in the DesktopStreamViewer, the zoomed image appears grainier/blurrier than expected given the underlying video resolution. The CSS `transform: scale()` is using browser default interpolation which causes quality degradation.

## User Stories

1. **As a mobile user**, I want pinch-to-zoom to produce crisp, pixel-sharp images so I can read small text clearly when zoomed in.

2. **As a tablet user**, I want zoomed content to look as sharp as native resolution viewing so I can work effectively on the remote desktop.

## Acceptance Criteria

- [ ] When zooming in on video content, pixels should appear crisp/pixelated rather than blurry
- [ ] The fix should apply to both the canvas element (video mode) and img element (screenshot mode)
- [ ] No performance regression on mobile devices
- [ ] Works correctly on iOS Safari, Android Chrome, and desktop browsers
- [ ] Existing pinch-to-zoom gesture functionality remains unchanged

## Technical Context

Current implementation in `DesktopStreamViewer.tsx`:
- Canvas element uses `transform: scale(${zoomLevel})` for zoom
- Screenshot img overlay uses same transform approach
- Browser applies bilinear interpolation by default, causing blurry scaling
- Need to apply `image-rendering: pixelated` (or `crisp-edges`) CSS property