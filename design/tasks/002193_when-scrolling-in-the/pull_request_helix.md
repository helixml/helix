# fix(frontend): suppress Chrome swipe-back gesture in desktop stream viewer

## Summary

When scrolling inside the desktop streaming viewer, Chrome/Safari's native
two-finger horizontal "swipe back/forward" navigation gesture could fire,
accidentally navigating the whole page away from the session. This forwards
wheel events to the remote desktop as before but now calls `preventDefault()`
so the browser's swipe-nav gesture is suppressed — scoped to the viewer only.

## Changes

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx`:
  - Call `event.preventDefault()` in the container wheel handler after forwarding
    the event to the remote desktop.
  - Register the wheel listener as non-passive (`{ passive: false }`) so
    `preventDefault()` is honoured.
  - Update the (now-inverted) comment to explain swipe-nav is intentionally
    suppressed inside the viewer.

The listener is attached to the viewer's own container ref, so swipe-to-navigate
continues to work everywhere else in the app.

## Testing

- `vite build` compiles cleanly (all modules transformed).
- Confirmed the inner-Helix dev server serves the change (`preventDefault()` +
  `{ passive: false }`).
- NOTE: the actual macOS trackpad swipe-back gesture is an OS/compositor gesture
  and is not reproducible via DevTools synthetic events on Linux — final
  user-facing confirmation should be done in macOS Chrome.
