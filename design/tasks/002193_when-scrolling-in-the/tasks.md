# Implementation Tasks: Disable Chrome Swipe-Back Gesture in Desktop Stream Viewer

- [ ] In `frontend/src/components/external-agent/DesktopStreamViewer.tsx`, add `event.preventDefault()` inside the wheel handler (currently ~line 2847) after forwarding the event to the remote desktop.
- [ ] Register the wheel listener as non-passive: `container.addEventListener("wheel", wheelHandler, { passive: false })`.
- [ ] Update the outdated comment (~lines 2839–2842) to explain that swipe-to-navigate is intentionally suppressed inside the viewer.
- [ ] Build the frontend: `cd frontend && yarn build`.
- [ ] Manually verify in the inner Helix: two-finger horizontal swipe over the desktop canvas no longer triggers browser back/forward, while wheel scroll still reaches the remote desktop.
- [ ] Regression check: confirm Chrome swipe-back still works on other Helix pages (outside the viewer).
