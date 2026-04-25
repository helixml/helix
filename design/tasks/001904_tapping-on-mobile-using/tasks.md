# Implementation Tasks

- [x] Add synthetic mouse event guard to `handleMouseDown` in `DesktopStreamViewer.tsx` — return early if `touchMode === "trackpad"` and `Date.now() - lastTouchEndTimeRef.current < 500`
- [x] Add same guard to `handleMouseUp`
- [~] (Optional) Guard `handler.onTouchStart()` delegation at line 3110 with `if (touchMode !== "trackpad")` to prevent stale state in StreamInput
- [ ] Test on mobile: single tap sends one click, menus stay open, double-tap-drag works, two-finger right-click works, real mouse/trackpad still works
