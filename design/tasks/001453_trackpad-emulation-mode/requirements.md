# Requirements: Trackpad Emulation Mode Regression Fix

## Problem Statement

Trackpad emulation mode on touch devices has regressed with two issues:
1. **Click location bug**: Clicks execute at the tap location instead of the virtual cursor position
2. **Two-finger scroll broken**: Two-finger scroll gesture no longer works (may conflict with pinch-to-zoom)

## User Stories

### US1: Accurate Click Position
As a mobile user in trackpad mode, when I tap to click, the click should occur at the **virtual cursor position** (where the cursor icon is displayed), not where my finger physically touched the screen.

**Acceptance Criteria:**
- Single tap sends click at virtual cursor coordinates
- Two-finger tap (right-click) sends click at virtual cursor coordinates
- Three-finger tap (middle-click) sends click at virtual cursor coordinates
- Double-tap-drag starts drag at virtual cursor coordinates

### US2: Two-Finger Scroll
As a mobile user in trackpad mode, when I use two fingers to scroll (without pinching), the remote desktop should scroll accordingly.

**Acceptance Criteria:**
- Two-finger vertical swipe scrolls content vertically
- Two-finger horizontal swipe scrolls content horizontally
- Scroll gesture is distinguished from pinch-to-zoom based on finger distance change
- Natural scrolling direction (swipe up = scroll up)

## Technical Context

The trackpad mode creates a virtual cursor that moves relative to finger movement (like a laptop trackpad). The cursor position is stored in both:
- `cursorPosition` (React state) - for rendering
- `cursorPositionRef` (ref) - for synchronous access

The bug is in `handleTouchEnd` where `sendCursorPositionToRemote()` uses `cursorPosition` from the closure, which can be stale due to React's batched state updates. The ref (`cursorPositionRef.current`) should be used instead.

## Out of Scope

- Changes to direct touch mode (non-trackpad)
- Pinch-to-zoom functionality (already working)
- Edge-pan when zoomed (already working)