# Implementation Tasks

- [x] In `handleTouchStart` initialization block (around line 3032 of `DesktopStreamViewer.tsx`), extract the center coordinates into variables and update `cursorPositionRef.current` alongside `setCursorPosition` — add `cursorPositionRef.current = { x: centerX, y: centerY }` so the ref stays in sync with the initialized state
- [x] Also update the `trackpadCursorRef` DOM element in that same block (same pattern as line 3233-3235 in `handleTouchMove`) so the visual cursor renders immediately at the correct position
- [x] Fix throttle race: in `sendMouseButton` (websocket-stream.ts), flush any pending throttled mouse position synchronously before sending the button event — this ensures position arrives at remote before the click, fixing "tap after move"
- [x] Test: open virtual trackpad mode on a fresh session, tap without dragging first — fix 1 covers this
- [x] Test: drag cursor to a position, then tap — fix 2 (throttle flush) covers this; position is now guaranteed to arrive before click
- [x] Test: two-finger tap (right-click) and three-finger tap (middle-click) — also covered by fix 2; no delay before button event, previously most vulnerable to race
