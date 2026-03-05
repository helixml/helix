# Implementation Tasks

## Bug 1: Click Location Incorrect

- [ ] In `handleTouchEnd`, update `sendCursorPositionToRemote()` to use `cursorPositionRef.current` instead of `cursorPosition` state
- [ ] Remove `cursorPosition` from the `handleTouchEnd` `useCallback` dependency array
- [ ] Test single tap click position on touch device in trackpad mode
- [ ] Test two-finger tap (right-click) position
- [ ] Test three-finger tap (middle-click) position
- [ ] Test double-tap-drag starting position

## Bug 2: Two-Finger Scroll Investigation

- [ ] Add console logging in `handleTouchMove` to trace gesture classification (`twoFingerGestureTypeRef.current`)
- [ ] Add logging to verify `sendMouseWheel` is called with correct delta values
- [ ] Test two-finger scroll gesture and observe logs
- [ ] If scroll events not firing: check gesture detection threshold (`PINCH_VS_SCROLL_THRESHOLD`)
- [ ] If scroll events firing but not working: trace WebSocket → backend scroll handling
- [ ] Tune `PINCH_VS_SCROLL_THRESHOLD` if needed (currently 30px may be too sensitive)
- [ ] Verify pinch-to-zoom still works after any scroll fixes

## Verification

- [ ] Test on iPad Safari
- [ ] Test on Android Chrome
- [ ] Test both portrait and landscape orientations
- [ ] Verify direct touch mode still works (regression test)