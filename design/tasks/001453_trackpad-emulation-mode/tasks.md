# Implementation Tasks

## Bug 1: Click Location Incorrect

- [ ] In `handleTouchEnd`, update `sendCursorPositionToRemote()` to use `cursorPositionRef.current` instead of `cursorPosition` state
- [ ] Remove `cursorPosition` from the `handleTouchEnd` `useCallback` dependency array

## Bug 2: Two-Finger Scroll Improvement

- [ ] Increase `PINCH_VS_SCROLL_THRESHOLD` from 30px to 50px (more forgiving for scroll)
- [ ] Add debug state refs to track two-finger gesture info:
  - `twoFingerDebugRef` with: gestureType, distanceChange, centerMovement, lastScrollDelta
- [ ] Update `handleTouchMove` to populate debug state during two-finger gestures
- [ ] Add two-finger gesture debug info to the stats overlay panel (when `showStats` enabled)

## Verification

- [ ] Build frontend successfully (`cd frontend && yarn build`)
- [ ] Deploy and test on real touch device
- [ ] If scroll still broken, report debug panel values for further tuning