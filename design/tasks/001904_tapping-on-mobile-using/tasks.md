# Implementation Tasks

- [x] Add synthetic mouse event guard to `handleMouseDown` in `DesktopStreamViewer.tsx` — return early if `touchMode === "trackpad"` and `Date.now() - lastTouchEndTimeRef.current < 500`
- [x] Add same guard to `handleMouseUp`
- [x] Guard `handler.onTouchStart()` delegation with `if (touchMode !== "trackpad")` to prevent stale state in StreamInput
- [x] Type-check passes (`yarn tsc --noEmit`)
- [ ] Test on mobile: single tap sends one click, menus stay open, double-tap-drag works, two-finger right-click works, real mouse/trackpad still works
