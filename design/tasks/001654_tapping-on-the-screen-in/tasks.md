# Implementation Tasks

- [~] In `handleTouchStart` initialization block (around line 3032 of `DesktopStreamViewer.tsx`), extract the center coordinates into variables and update `cursorPositionRef.current` alongside `setCursorPosition` — add `cursorPositionRef.current = { x: centerX, y: centerY }` so the ref stays in sync with the initialized state
- [ ] Also update the `trackpadCursorRef` DOM element in that same block (same pattern as line 3233-3235 in `handleTouchMove`) so the visual cursor renders immediately at the correct position
- [ ] Test: open virtual trackpad mode on a fresh session, tap without dragging first, verify the click lands at the center of the remote screen (not top-left)
- [ ] Test: drag cursor to a position, then tap, verify click lands at that dragged-to position
- [ ] Test: two-finger tap (right-click) and three-finger tap (middle-click) still work correctly
