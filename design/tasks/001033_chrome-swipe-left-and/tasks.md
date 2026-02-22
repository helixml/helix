# Implementation Tasks

- [~] Investigate current state: `touchAction: 'none'` already exists on canvas (line 3985), global `overscroll-behavior: none` present in index.html
- [ ] Remove `event.preventDefault()` from touch handlers (handleTouchStart, handleTouchMove, handleTouchEnd, handleTouchCancel)
- [ ] Test Chrome desktop: verify two-finger swipe back/forward works on non-stream pages (project list, settings)
- [ ] Test Chrome desktop: verify stream viewer touch/trackpad controls still work (cursor movement, tap-to-click, pinch-to-zoom)
- [ ] Test Safari iPad: verify no UI sliding/bouncing when scrolling or interacting with the app
- [ ] Test Safari iPad: verify stream viewer touch controls work correctly
- [ ] Test mobile Chrome: verify same behavior as desktop Chrome
- [ ] If removing preventDefault breaks stream controls, implement scoped preventDefault (only when touch starts inside canvas bounds)