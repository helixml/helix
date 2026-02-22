# Design: Auto-Pan Display When Cursor Reaches Edge in Zoom Mode

## Overview

Add edge-detection logic to `handleTouchMove` in `DesktopStreamViewer.tsx` that triggers viewport panning when the trackpad cursor approaches the visible viewport boundary while zoomed.

## Architecture

### Key Insight

The cursor position (`cursorPositionRef`) is in **container coordinates**, while zoom/pan transforms are applied to the canvas. When zoomed in, only a portion of the stream is visible. The visible viewport bounds can be calculated from `zoomLevel` and `panOffset`.

### Implementation Location

All changes in `helix/frontend/src/components/external-agent/DesktopStreamViewer.tsx`:

1. **New constants** (near existing zoom constants ~L232):
   - `EDGE_PAN_ZONE_PX = 20` - Distance from edge to trigger panning
   - `EDGE_PAN_SPEED = 8` - Max pixels per frame to pan

2. **New ref** for animation frame:
   - `edgePanAnimationRef = useRef<number | null>(null)`

3. **Edge detection logic** in `handleTouchMove` (trackpad mode section ~L2678):
   - After updating cursor position, check if cursor is within edge zone
   - If zoomLevel > 1 and cursor near edge, start panning animation

4. **Cleanup** in `handleTouchEnd`:
   - Cancel any active edge-pan animation

## Algorithm

```
On each cursor move (trackpad mode, zoomed):
  1. Calculate visible viewport bounds from container size, zoomLevel, panOffset
  2. Check cursor proximity to each edge (top/bottom/left/right)
  3. If within EDGE_PAN_ZONE_PX of any edge:
     - Calculate pan velocity (faster closer to edge)
     - Update panOffset via requestAnimationFrame
     - Clamp panOffset to valid bounds (same as pinch-pan logic)
  4. If cursor moves away from edge, cancel animation
```

## Pan Speed Curve

```typescript
// Distance from edge (0 = at edge, EDGE_PAN_ZONE_PX = at threshold)
const distanceFromEdge = Math.max(0, EDGE_PAN_ZONE_PX - cursorDistToEdge);
// Normalized 0-1 (1 = at edge)
const factor = distanceFromEdge / EDGE_PAN_ZONE_PX;
// Apply easing for smooth acceleration
const panSpeed = EDGE_PAN_SPEED * factor * factor; // quadratic easing
```

## Visible Viewport Calculation

When zoomed, the visible area is `containerSize / zoomLevel`, offset by `panOffset`:

```typescript
const containerRect = containerRef.current.getBoundingClientRect();
const visibleWidth = containerRect.width / zoomLevel;
const visibleHeight = containerRect.height / zoomLevel;

// Visible viewport center is shifted by panOffset
const viewportCenterX = containerRect.width / 2 - panOffset.x;
const viewportCenterY = containerRect.height / 2 - panOffset.y;

// Edge positions in container coordinates
const leftEdge = viewportCenterX - visibleWidth / 2;
const rightEdge = viewportCenterX + visibleWidth / 2;
// ... etc
```

## Dependencies

- Reuses existing `zoomLevel`, `panOffset`, `setPanOffset` state
- Reuses existing pan bounds calculation from pinch-zoom handler
- No new dependencies required