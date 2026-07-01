# Implementation Tasks: Disable Chrome Swipe-Back Gesture in Desktop Stream Viewer

- [x] In `frontend/src/components/external-agent/DesktopStreamViewer.tsx`, add `event.preventDefault()` inside the wheel handler (currently ~line 2847) after forwarding the event to the remote desktop.
- [x] Register the wheel listener as non-passive: `container.addEventListener("wheel", wheelHandler, { passive: false })`.
- [x] Update the outdated comment (~lines 2839–2842) to explain that swipe-to-navigate is intentionally suppressed inside the viewer.
- [x] Build the frontend: `cd frontend && yarn build` (verified via temp outDir; prod `dist/` is root-owned bind mount).
- [x] Verify the running dev server serves the change (`event.preventDefault()` + `{ passive: false }`) — confirmed via `curl http://localhost:8080/src/.../DesktopStreamViewer.tsx`.
- [ ] Manual (macOS Chrome, needs a human): two-finger horizontal swipe over the desktop canvas no longer triggers browser back/forward, while wheel scroll still reaches the remote desktop. NOTE: not reproducible in this Linux/DevTools env — the swipe-back is an OS-level trackpad compositor gesture, not a synthetic wheel event.
- [ ] Regression check (macOS Chrome, needs a human): confirm Chrome swipe-back still works on other Helix pages (outside the viewer).
