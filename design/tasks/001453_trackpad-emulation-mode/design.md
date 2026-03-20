# Design: Trackpad Emulation Mode Regression Fix

## Architecture Overview

The touch input system has two modes:
- **Direct mode**: Touch position = cursor position (touch-to-click)
- **Trackpad mode**: Finger movement moves a virtual cursor relatively; tap = click at cursor position

```
Touch Events → DesktopStreamViewer.tsx → StreamInput (via getInputHandler())
                     ↓
              Virtual cursor state:
              - cursorPosition (React state, for rendering)
              - cursorPositionRef (ref, for synchronous access)
```

## Root Cause Analysis

### Bug 1: Click Location Incorrect

**Location**: `handleTouchEnd` in `DesktopStreamViewer.tsx` (around line 3436)

**Problem**: The `sendCursorPositionToRemote()` helper uses `cursorPosition` from the closure:

```typescript
const sendCursorPositionToRemote = () => {
  // ...
  const streamRelativeX = cursorPosition.x - streamOffsetX;  // ← STALE!
  const streamRelativeY = cursorPosition.y - streamOffsetY;  // ← STALE!
  // ...
};
```

`cursorPosition` is captured when the `useCallback` is created. Due to React's batched updates, this value can be stale when the tap occurs.

**Fix**: Use `cursorPositionRef.current` instead, which is updated synchronously in `handleTouchMove`:

```typescript
const sendCursorPositionToRemote = () => {
  // ...
  const currentPos = cursorPositionRef.current;  // ← FRESH!
  const streamRelativeX = currentPos.x - streamOffsetX;
  const streamRelativeY = currentPos.y - streamOffsetY;
  // ...
};
```

### Bug 2: Two-Finger Scroll Not Working

**Location**: `handleTouchMove` in `DesktopStreamViewer.tsx` (around line 3343)

**Analysis**: The two-finger gesture detection logic exists and distinguishes pinch from scroll:

```typescript
if (distanceChange > PINCH_VS_SCROLL_THRESHOLD) {
  twoFingerGestureTypeRef.current = "pinch";
} else if (centerMovement > 10) {
  twoFingerGestureTypeRef.current = "scroll";
}
```

**Likely issue**: The threshold check order means pinch is always checked first. If a user's fingers naturally spread slightly while scrolling, it triggers pinch mode immediately.

**Fix**: Increase `PINCH_VS_SCROLL_THRESHOLD` from 30px to 50px, and add debug info to help diagnose further if needed.

## Implementation Plan

### Phase 1: Fix Click Location (High Confidence)

1. In `handleTouchEnd`, change `sendCursorPositionToRemote()` to use `cursorPositionRef.current`
2. Remove `cursorPosition` from the `useCallback` dependency array (it's no longer used)

### Phase 2: Improve Two-Finger Scroll + Add Debug Panel

1. Increase `PINCH_VS_SCROLL_THRESHOLD` from 30px to 50px (more forgiving for scroll detection)
2. Add debug state for two-finger gesture tracking:
   - Last gesture type ("undecided" | "pinch" | "scroll")
   - Last distance change value
   - Last center movement value
   - Last scroll delta sent
3. Display this in the existing debug overlay (when `showStats` is enabled)

## Debug Panel Addition

Add to the stats debug panel (visible when user clicks the stats icon):

```
Two-Finger Gesture:
  Type: scroll
  Dist Change: 12px
  Center Move: 45px  
  Last Scroll: dx=0 dy=15
```

This lets the user report back what values they're seeing during scroll attempts.

## Key Files

| File | Purpose |
|------|---------|
| `frontend/src/components/external-agent/DesktopStreamViewer.tsx` | Touch event handlers, virtual cursor state, debug panel |
| `frontend/src/lib/helix-stream/stream/websocket-stream.ts` | WebSocket transport for input events |

## Testing

Testing will be done after deployment on real touch device. Debug panel info can be reported back for further tuning if scroll still doesn't work.

## Risks

- **Low risk**: Using ref instead of state is a safe change
- **Low risk**: Increasing pinch threshold may slightly delay pinch-to-zoom detection, but 50px is still responsive