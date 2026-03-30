# Implementation Tasks

- [x] Fix main bug: add `if (touchMode === "trackpad") return` guard at top of `handleMouseMove` — prevents synthetic mouse events from touch taps overriding the virtual cursor position
- [x] In `handleTouchStart` initialization block, update `cursorPositionRef.current` alongside `setCursorPosition` — fixes first-tap-before-drag clicking at (0,0)
- [x] Also update `trackpadCursorRef` DOM element in that same block for immediate visual consistency
- [x] Fix throttle race: in `sendMouseButton`, flush any pending throttled mouse position before sending the button event
- [x] Test: tap after moving cursor — cursor no longer jumps to tap position; click lands at virtual cursor position
- [x] Test: first tap before any drag — click lands at stream center, not (0,0)
- [x] Test: two-finger and three-finger taps work correctly
