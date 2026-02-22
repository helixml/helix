# Requirements: Auto-Pan Display When Cursor Reaches Edge in Zoom Mode

## User Story

As a mobile user viewing a remote desktop in zoom mode with trackpad emulation, I want the display to automatically pan when my cursor reaches the edge of the visible area, so I can access parts of the remote desktop that are currently off-screen without having to manually pinch-to-pan.

## Problem

Currently in `DesktopStreamViewer.tsx`:
1. Pinch-to-zoom works - users can zoom into the remote desktop (1x to 5x)
2. Trackpad mode works - users drag finger to move a virtual cursor
3. **Missing:** When zoomed in, the cursor is clamped to stream bounds, but there's no auto-panning when the cursor reaches the edge of the visible viewport

Users must manually two-finger pinch-drag to pan, which is awkward while also trying to control the cursor.

## Acceptance Criteria

1. **Auto-pan triggers at viewport edges**: When cursor moves within ~20px of visible viewport edge while zoomed (zoomLevel > 1), the viewport pans in that direction
2. **Proportional pan speed**: Pan speed increases as cursor gets closer to edge (smooth acceleration)
3. **Works in trackpad mode**: Auto-pan activates during single-finger cursor movement in trackpad emulation mode
4. **Respects zoom bounds**: Panning stops at remote desktop boundaries (can't pan beyond content)
5. **Smooth animation**: Pan uses requestAnimationFrame for 60fps smoothness
6. **No pan at 1x zoom**: Feature only activates when zoomLevel > 1 (nothing to pan at 1:1)

## Out of Scope

- Direct touch mode (non-trackpad) - taps go directly to touch coordinates
- Mouse/desktop browser behavior - this is mobile/touch specific
- Changing existing pinch-to-zoom behavior