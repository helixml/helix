# Implementation Tasks

- [x] Add edge-pan constants near existing zoom constants (~L279): `EDGE_PAN_ZONE_PX = 20`, `EDGE_PAN_SPEED = 8`
- [x] Add `edgePanAnimationRef = useRef<number | null>(null)` for tracking animation frame
- [x] Create `calculateVisibleViewportBounds()` helper function that returns edge positions based on container size, zoomLevel, and panOffset
- [x] Create `startEdgePan(direction)` function that uses requestAnimationFrame to smoothly update panOffset
- [x] In `handleTouchMove` trackpad mode section (~L3264): after cursor position update, check if zoomLevel > 1 and cursor is within EDGE_PAN_ZONE_PX of visible viewport edge
- [x] Calculate pan velocity using quadratic easing based on distance from edge
- [x] Call `startEdgePan()` with appropriate direction(s) when cursor is in edge zone
- [x] Cancel edge-pan animation when cursor moves away from edge zone via `stopEdgePan()`
- [x] In `handleTouchEnd`: cancel any active edge-pan animation via `stopEdgePan()`
- [x] In `handleTouchCancel`: cancel any active edge-pan animation via `stopEdgePan()`
- [x] Ensure panOffset stays within valid bounds (reuses existing clamping logic from pinch-pan)
- [ ] Test on mobile device: verify auto-pan triggers when dragging cursor to edge while zoomed
- [ ] Test pan stops at desktop boundaries (can't pan beyond content)
- [ ] Test no auto-pan occurs at zoomLevel = 1

## Implementation Summary

The edge-pan feature has been fully implemented in `DesktopStreamViewer.tsx`:

1. **Constants added** (L279-282):
   - `EDGE_PAN_ZONE_PX = 20` - triggers panning within 20px of viewport edge
   - `EDGE_PAN_SPEED = 8` - max pixels per frame to pan
   - `edgePanAnimationRef` - tracks requestAnimationFrame handle

2. **Helper functions** (L3105-3183):
   - `calculateVisibleViewportBounds()` - calculates visible viewport region accounting for zoom and pan
   - `startEdgePan(direction, intensity)` - starts smooth 60fps animation with quadratic easing
   - `stopEdgePan()` - cancels animation frame

3. **Edge detection** (L3264-3321):
   - In `handleTouchMove`, after updating cursor position, checks if zoomed (zoomLevel > 1)
   - Calculates distance from cursor to each viewport edge
   - If within EDGE_PAN_ZONE_PX, calculates intensity (closer to edge = faster pan)
   - Calls `startEdgePan()` with direction vector and intensity
   - Calls `stopEdgePan()` when cursor leaves edge zone

4. **Cleanup**:
   - `handleTouchEnd` calls `stopEdgePan()` (L3567)
   - `handleTouchCancel` calls `stopEdgePan()` (L3621)