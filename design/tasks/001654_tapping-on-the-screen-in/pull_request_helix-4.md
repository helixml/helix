# Fix virtual trackpad tap position on first touch

## Summary

In virtual trackpad mode, tapping the screen before doing any drag would send the click to the top-left of the remote screen (`0,0`) instead of where the virtual cursor was displayed. This was caused by a ref/state desync on first touch initialization.

## Root Cause

`cursorPositionRef` (used synchronously in `sendCursorPositionToRemote`) was initialized to `{x:0, y:0}` and only gets updated during drag (`handleTouchMove`). On first touch, only the React state (`setCursorPosition`) was updated to the stream center — the ref was not. So a first tap would read the stale `{x:0, y:0}` ref and click the wrong position.

## Changes

- `DesktopStreamViewer.tsx`: In the first-touch initialization block inside `handleTouchStart`, extract center coords into variables and update `cursorPositionRef.current` and `trackpadCursorRef` DOM element alongside `setCursorPosition` — matching the existing pattern in `handleTouchMove`.
