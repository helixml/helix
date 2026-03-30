# Fix virtual trackpad tap jumping cursor to tap position

## Summary

In virtual trackpad mode, tapping would jump the virtual mouse cursor (both the local overlay and the remote mouse) to the physical tap coordinates. This was caused by synthetic mouse events that browsers fire after touch events, which `handleMouseMove` was processing unconditionally.

Two additional bugs were also fixed: first-tap-before-drag clicking at `(0,0)` due to a ref/state desync, and a throttle race that could cause click events to arrive at the remote before the position update.

## Root Cause

When a touchscreen tap occurs, the browser fires synthetic `mousemove` events after the touch events. `handleMouseMove` had no guard for trackpad mode, so it:
1. Called `setCursorPosition()` with the tap's physical coordinates → local cursor overlay jumps to tap position
2. Called `onMouseMove()` which (in "follow" mode) sent `sendMousePositionClientCoordinates(tapX, tapY)` → remote mouse jumps to tap position

## Changes

- `DesktopStreamViewer.tsx`: early return in `handleMouseMove` when `touchMode === "trackpad"` — cursor position in trackpad mode is owned by the touch handlers, not mouse events
- `DesktopStreamViewer.tsx`: sync `cursorPositionRef.current` and trackpad DOM element in the first-touch initialization block (fixes first-tap-before-drag)
- `websocket-stream.ts`: flush pending throttled mouse position before sending button events (fixes throttle ordering race)
