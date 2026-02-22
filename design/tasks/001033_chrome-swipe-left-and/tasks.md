# Implementation Tasks

- [x] Investigate current state: `touchAction: 'none'` already exists on canvas (line 3985), global `overscroll-behavior: none` present in index.html
- [x] Remove `event.preventDefault()` from touch handlers (handleTouchStart, handleTouchMove, handleTouchEnd, handleTouchCancel)
- [x] Verify frontend tests pass and build succeeds
- [ ] Test Chrome desktop: verify two-finger swipe back/forward works on non-stream pages (project list, settings)
- [ ] Test Chrome desktop: verify stream viewer touch/trackpad controls still work (cursor movement, tap-to-click, pinch-to-zoom)
- [ ] Test Safari iPad: verify no UI sliding/bouncing when scrolling or interacting with the app
- [ ] Test Safari iPad: verify stream viewer touch controls work correctly
- [ ] Test mobile Chrome: verify same behavior as desktop Chrome

## Testing Notes

**Code changes complete and verified:**
- Removed `event.preventDefault()` from all touch handlers
- CSS `touchAction: 'none'` remains on canvas (correct approach)
- Frontend tests pass (`yarn test`)
- Production build succeeds (`yarn build`)
- Changes committed to feature branch: `b38908070`

**Manual testing required:**
The remaining tasks require actual browser testing on different devices/platforms. The user should test:
1. Chrome desktop with trackpad - verify swipe navigation works on non-stream pages
2. Chrome desktop - verify stream controls still work (touch/trackpad mode)
3. Safari iPad - verify no rubber-band scrolling or UI sliding
4. Safari iPad - verify stream touch controls work
5. Mobile Chrome - verify same behavior as desktop

**Fallback plan:**
If manual testing reveals that stream touch controls are broken, the last task provides a fallback: implement scoped `preventDefault()` that only triggers when touch starts inside canvas bounds.