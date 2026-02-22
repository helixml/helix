# Implementation Tasks

- [ ] Add edge-pan constants near existing zoom constants (~L232): `EDGE_PAN_ZONE_PX = 20`, `EDGE_PAN_SPEED = 8`
- [ ] Add `edgePanAnimationRef = useRef<number | null>(null)` for tracking animation frame
- [ ] Create `calculateVisibleViewportBounds()` helper function that returns edge positions based on container size, zoomLevel, and panOffset
- [ ] Create `startEdgePan(direction)` function that uses requestAnimationFrame to smoothly update panOffset
- [ ] In `handleTouchMove` trackpad mode section (~L2678): after cursor position update, check if zoomLevel > 1 and cursor is within EDGE_PAN_ZONE_PX of visible viewport edge
- [ ] Calculate pan velocity using quadratic easing based on distance from edge
- [ ] Call `startEdgePan()` with appropriate direction(s) when cursor is in edge zone
- [ ] Cancel edge-pan animation when cursor moves away from edge zone
- [ ] In `handleTouchEnd`: cancel any active edge-pan animation via `cancelAnimationFrame`
- [ ] Ensure panOffset stays within valid bounds (reuse existing clamping logic from pinch-pan)
- [ ] Test on mobile device: verify auto-pan triggers when dragging cursor to edge while zoomed
- [ ] Test pan stops at desktop boundaries (can't pan beyond content)
- [ ] Test no auto-pan occurs at zoomLevel = 1