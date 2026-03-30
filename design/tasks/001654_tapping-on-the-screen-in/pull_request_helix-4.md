# Fix virtual trackpad tap position (throttle race + ref init)

## Summary

Two bugs caused taps in virtual trackpad mode to click at the wrong position. The main bug was a race condition between throttled mouse position updates and unthrottled button events.

## Root Cause

**Bug 1 (DesktopStreamViewer.tsx):** On first touch, `setCursorPosition` (React state) was updated to stream center but `cursorPositionRef.current` was not. Since `sendCursorPositionToRemote()` reads the ref synchronously, first tap before any drag would send click to `(0,0)`.

**Bug 2 (websocket-stream.ts) — main bug:** `sendMousePosition` is throttled; if called within the throttle window, the position is deferred to `pendingMousePosition`. `sendMouseButton` sends immediately with no throttle. Result: remote receives the button click *before* the position update, clicking at the old cursor location.

## Changes

- `DesktopStreamViewer.tsx`: sync `cursorPositionRef.current` and trackpad DOM element in the first-touch initialization block.
- `websocket-stream.ts`: in `sendMouseButton`, flush any pending throttled mouse position synchronously before sending the button event — guarantees correct ordering.
