# Implementation Tasks

- [ ] Add `touchAction: 'none'` to the canvas element style in `DesktopStreamViewer.tsx` (around line 3962)
- [ ] Test Chrome desktop: verify two-finger swipe back/forward works on non-stream pages (project list, settings)
- [ ] Test Chrome desktop: verify stream viewer touch/trackpad controls still work (cursor movement, tap-to-click, pinch-to-zoom)
- [ ] Test Safari iPad: verify no UI sliding/bouncing when scrolling or interacting with the app
- [ ] Test Safari iPad: verify stream viewer touch controls work correctly
- [ ] If `touch-action: none` alone is insufficient, consider removing `event.preventDefault()` from touch handlers as secondary step
- [ ] Test mobile Chrome: verify same behavior as desktop Chrome