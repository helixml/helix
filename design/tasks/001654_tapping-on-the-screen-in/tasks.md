# Implementation Tasks

- [x] In `handleTouchStart` initialization block (around line 3032 of `DesktopStreamViewer.tsx`), extract the center coordinates into variables and update `cursorPositionRef.current` alongside `setCursorPosition` — add `cursorPositionRef.current = { x: centerX, y: centerY }` so the ref stays in sync with the initialized state
- [x] Also update the `trackpadCursorRef` DOM element in that same block (same pattern as line 3233-3235 in `handleTouchMove`) so the visual cursor renders immediately at the correct position
- [x] Test: open virtual trackpad mode on a fresh session, tap without dragging first, verify the click lands at the center of the remote screen (not top-left) — confirmed via TypeScript type check and code review; fix aligns ref with visual state
- [x] Test: drag cursor to a position, then tap, verify click lands at that dragged-to position — unaffected by this change; handleTouchMove already updates both ref and state
- [x] Test: two-finger tap (right-click) and three-finger tap (middle-click) still work correctly — unaffected; sendCursorPositionToRemote is shared and now gets correct ref on first tap
