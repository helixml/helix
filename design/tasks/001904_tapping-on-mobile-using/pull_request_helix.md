# Fix double-click in mobile virtual trackpad mode

## Summary

Tapping in trackpad mode on mobile sent two clicks instead of one, making menus and toggles unusable (a tap would activate then immediately deactivate them).

## Root cause

Mobile browsers fire synthetic `mousedown`/`mouseup` events after touch sequences (default behavior unless `preventDefault()` is called on the touch events). In trackpad mode this caused two clicks per tap:

1. The trackpad tap handler in `handleTouchEnd` correctly sends one click at the virtual cursor position.
2. The browser then fires synthetic `mousedown`/`mouseup`, which `handleMouseDown`/`handleMouseUp` forwarded to `StreamInput`, sending a second click.

`handleMouseMove` already had a guard against these synthetic events using `lastTouchEndTimeRef`, but the down/up handlers were missing the same guard.

## Changes

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx`:
  - Add the existing `lastTouchEndTimeRef` synthetic-event guard to `handleMouseDown` and `handleMouseUp` (mirrors `handleMouseMove`).
  - Skip the `handler.onTouchStart()` delegation in trackpad mode so `StreamInput` doesn't accumulate stale `primaryTouch` / `touchTracker` state (we never call its `onTouchEnd` in trackpad mode).
