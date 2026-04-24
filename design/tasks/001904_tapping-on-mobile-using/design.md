# Design

## Root Cause

In `DesktopStreamViewer.tsx`, when the user taps in trackpad mode, two independent code paths both send a click:

1. **Trackpad tap handler** (`handleTouchEnd`, ~line 3550): Detects the tap and sends `sendMouseButton(true/false, LEFT)` after a `DOUBLE_TAP_THRESHOLD_MS` delay.

2. **Synthetic mouse events**: Mobile browsers fire synthetic `mousedown`/`mouseup` events after a touch sequence (standard behavior when `preventDefault()` is not called on touch events). The `handleMouseDown` (~line 2947) and `handleMouseUp` (~line 2955) handlers forward these to `StreamInput.onMouseDown`/`onMouseUp`, which send a second click.

`handleMouseMove` (line 2972) already guards against this:
```typescript
if (touchMode === "trackpad" && Date.now() - lastTouchEndTimeRef.current < 500) return;
```
But `handleMouseDown` and `handleMouseUp` have no such guard.

## Fix

Add the same synthetic mouse event suppression to `handleMouseDown` and `handleMouseUp` that already exists in `handleMouseMove`. The pattern uses `lastTouchEndTimeRef` (already maintained by `handleTouchEnd` at line 3469) to detect and ignore mouse events arriving shortly after a touch interaction.

This approach is preferred over calling `preventDefault()` on touch events because that would break native behaviors (scrolling, etc.) and is a bigger change surface.

## Secondary Cleanup (Optional)

`handleTouchStart` (line 3110) unconditionally delegates to `handler.onTouchStart()` even in trackpad mode, which registers state in StreamInput (primaryTouch, touchTracker) that is never cleaned up (since `handleTouchEnd` returns early and never calls `handler.onTouchEnd`). This doesn't cause the double-click bug but leaves stale state. Could be guarded with an `if (touchMode !== "trackpad")` check around the delegation.

## Key Files

| File | Role |
|------|------|
| `frontend/src/components/external-agent/DesktopStreamViewer.tsx` | Touch/mouse event handlers, trackpad mode logic |
| `frontend/src/lib/helix-stream/stream/input.ts` | StreamInput — lower-level touch/mouse event processing |
