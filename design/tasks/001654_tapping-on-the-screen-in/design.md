# Design: Fix Virtual Trackpad Tap Position

## Root Cause (Three Bugs)

### Bug 1 (Main): Synthetic mouse events override trackpad cursor on tap

**File:** `DesktopStreamViewer.tsx`, `handleMouseMove`

When a user taps on a touchscreen, the browser fires synthetic mouse events after the touch events (`mousemove`, `mousedown`, `mouseup`, `click`). These carry the physical tap coordinates (`clientX/clientY`).

`handleMouseMove` had no guard for `touchMode === "trackpad"`, so on every tap:
1. `setCursorPosition({ x: tapX, y: tapY })` — the local cursor overlay **jumps** to the tap position
2. `handler.onMouseMove(event.nativeEvent, rect)` — in "follow" mouseMode, sends `sendMousePositionClientCoordinates(tapX, tapY)` — the **remote cursor jumps** to the tap position

This is the bug the user saw: "tapping would jump the virtual mouse cursor to the tap position."

**Fix:** Add `if (touchMode === "trackpad") return;` at the top of `handleMouseMove`. Trackpad cursor position is owned exclusively by the touch handlers.

### Bug 2: First tap before any drag clicks at (0,0)

**File:** `DesktopStreamViewer.tsx`, `handleTouchStart`

On first touch, only `setCursorPosition` (React state) was updated to stream center — `cursorPositionRef.current` stayed at `{x:0,y:0}`. Since `sendCursorPositionToRemote()` reads the ref synchronously, first tap before any drag sent click to top-left.

**Fix:** Also update `cursorPositionRef.current` and the trackpad DOM element in the `!hasMouseMoved` initialization block.

### Bug 3: Throttle race causes click to arrive before position

**File:** `websocket-stream.ts`, `sendMouseButton`

`sendMousePosition` is throttled; within the throttle window, the position goes into `pendingMousePosition` to be sent later. `sendMouseButton` sends immediately. So the remote can receive: click → then position. Remote clicks at old cursor location.

**Fix:** In `sendMouseButton`, flush any `pendingMousePosition` synchronously before sending the button event.

## Key Files Modified

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` — Bug 1 (main) + Bug 2
- `frontend/src/lib/helix-stream/stream/websocket-stream.ts` — Bug 3

## Codebase Patterns Found

- Touchscreens fire synthetic mouse events after touch events — React components must guard against this when custom touch handling is in place.
- The input handler's `touchMode` type (`"touch" | "mouseRelative" | "pointAndDrag"`) does not include `"trackpad"` — `"trackpad"` is handled entirely in `DesktopStreamViewer` before delegating to the input handler.
- The default `mouseMode` is `"follow"`, which sends absolute position on every `onMouseMove` call.
- `cursorPositionRef` is the synchronous source of truth for cursor position in tap handlers; `setCursorPosition` is the async React state for rendering.
