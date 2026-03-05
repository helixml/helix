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

Potential issues:
1. The gesture starts as "undecided" but may be classified as pinch too quickly
2. `PINCH_VS_SCROLL_THRESHOLD` (30px) may be too sensitive
3. The scroll handler calls `handler.sendMouseWheel?.()` but this may not be reaching the backend correctly

**Investigation needed**: Add logging to verify:
- Is `twoFingerGestureTypeRef.current` being set to "scroll"?
- Is `sendMouseWheel` being called with correct values?

## Implementation Plan

### Phase 1: Fix Click Location (High Confidence)

1. In `handleTouchEnd`, change `sendCursorPositionToRemote()` to use `cursorPositionRef.current`
2. Remove `cursorPosition` from the `useCallback` dependency array (it's no longer used)

### Phase 2: Investigate Two-Finger Scroll (Needs Testing)

1. Add console logging to track gesture classification
2. Verify `sendMouseWheel` is being called
3. If issue is gesture detection threshold, tune `PINCH_VS_SCROLL_THRESHOLD`
4. If issue is backend, trace through WebSocket → desktop-bridge → Wayland

## Key Files

| File | Purpose |
|------|---------|
| `frontend/src/components/external-agent/DesktopStreamViewer.tsx` | Touch event handlers, virtual cursor state |
| `frontend/src/lib/helix-stream/stream/websocket-stream.ts` | WebSocket transport for input events |
| `frontend/src/lib/helix-stream/stream/input.ts` | StreamInput class with send methods |

## Testing

1. Open session on touch device (iPad/Android tablet)
2. Enable trackpad mode (toggle in toolbar)
3. Move finger to position cursor
4. Single tap - verify click occurs at cursor, not finger position
5. Two-finger tap - verify right-click at cursor position
6. Two-finger scroll (fingers parallel, same distance) - verify remote scrolls
7. Pinch to zoom - verify local zoom still works (not broken by scroll fix)

## Risks

- **Low risk**: Using ref instead of state is a safe change
- **Medium risk**: Adjusting scroll/pinch threshold may affect some users' pinch-to-zoom experience