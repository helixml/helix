# Design: Fix Virtual Trackpad Tap Position

## Root Cause (Two Bugs)

### Bug 1: First tap before any drag (DesktopStreamViewer.tsx)

`cursorPositionRef` is initialized to `{x:0, y:0}` (line 175). On first touch, `handleTouchStart` calls `setCursorPosition` (React state) to set cursor to stream center, but does NOT update `cursorPositionRef.current`. `sendCursorPositionToRemote()` reads the ref synchronously, so first tap sends click to `(0,0)`.

**Fix:** In the `!hasMouseMoved` initialization block, also update `cursorPositionRef.current` and the trackpad DOM element alongside `setCursorPosition`.

### Bug 2: Tap after move — throttle race condition (websocket-stream.ts) ← MAIN BUG

`sendMousePosition` is **throttled**. When called from `sendCursorPositionToRemote()` (before a tap), if the throttle window hasn't elapsed, the position is stored in `pendingMousePosition` and scheduled to send later.

`sendMouseButton` is **not throttled** — it sends immediately via `sendInputMessage`.

So the remote receives events in this order:
1. `sendMouseButton` (click) — arrives immediately
2. `sendMousePosition` (position) — arrives after throttle delay

The remote clicks at wherever the cursor already was, not where the virtual cursor shows.

This affects all taps (single, right-click, middle-click). For single-tap there's a `DOUBLE_TAP_THRESHOLD_MS` delay before the click, so the throttle may have flushed, but it's timing-dependent. For multi-finger taps (right-click, middle-click) there's no delay at all.

**Fix:** In `sendMouseButton`, flush any `pendingMousePosition` synchronously (cancelling the scheduled timeout) before sending the button event. This guarantees position arrives before click.

## Key Files Modified

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` — Bug 1 fix (first-tap ref init)
- `frontend/src/lib/helix-stream/stream/websocket-stream.ts` — Bug 2 fix (flush pending position before button)

## Codebase Patterns Found

- `cursorPositionRef` + `setCursorPosition` are kept in sync during `handleTouchMove` but the initialization path missed the ref update.
- `sendMousePosition` uses adaptive throttling to avoid flooding the WebSocket during drag.
- `sendMouseButton` has no throttling because clicks are infrequent.
- The flush pattern (cancel timeout + send immediate + reset lastMouseSendTime) matches what `scheduleMouseFlush` does internally.

## Implementation Notes

- "Tap after move" is the main user-reported bug — caused by the throttle race in websocket-stream.ts
- "First tap before move" is a secondary bug — caused by the ref/state desync in DesktopStreamViewer.tsx
- Both bugs were present simultaneously; both are fixed
