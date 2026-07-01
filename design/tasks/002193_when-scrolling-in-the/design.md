# Design: Disable Chrome Swipe-Back Gesture in Desktop Stream Viewer

## Root Cause

File: `frontend/src/components/external-agent/DesktopStreamViewer.tsx`

The wheel handler (currently ~lines 2839–2860) forwards wheel events to the
remote desktop but deliberately does **not** call `preventDefault()`:

```tsx
// Forward wheel events to remote desktop via WebSocketStream.
// No preventDefault needed — the container has overflow:hidden so there's nothing
// for the browser to scroll, and leaving the event unhandled lets Chrome's native
// swipe-to-navigate gesture (two-finger horizontal swipe for back/forward) work.
useEffect(() => {
  const container = containerRef.current;
  if (!container) return;

  const wheelHandler = (event: WheelEvent) => {
    const input = /* ...getInput()... */;
    input?.onMouseWheel(event);
  };

  container.addEventListener("wheel", wheelHandler);
  return () => container.removeEventListener("wheel", wheelHandler);
}, []);
```

The comment reflects the *previous* intent (let swipe-nav work). That intent is
exactly what the user now wants disabled — inside this viewer only.

Why `overscrollBehavior: "none"` on the container is not enough: `overscroll-behavior`
only governs scroll *chaining* when there is a scrollable area. The viewer container
has `overflow: hidden`, so there is nothing to scroll, and Chrome's two-finger
swipe-navigation is a gesture layered on top of the wheel stream — it is triggered by
the wheel/gesture events themselves, not by scroll overscroll. The reliable way to
suppress it is to `preventDefault()` the wheel events on this element.

## Approach

Call `event.preventDefault()` inside the existing wheel handler, and register the
listener as **non-passive** so `preventDefault()` is honoured.

```tsx
const wheelHandler = (event: WheelEvent) => {
  const input = /* ...getInput()... */;
  input?.onMouseWheel(event);
  event.preventDefault(); // suppress Chrome's swipe back/forward navigation
};

container.addEventListener("wheel", wheelHandler, { passive: false });
```

### Key decisions

- **Non-passive listener (`{ passive: false }`).** Browsers treat wheel
  listeners as passive by default in many contexts; a passive listener's
  `preventDefault()` is ignored (and warns in console). Registering explicitly
  non-passive makes the suppression effective. The removeEventListener call needs
  no options change (only `capture` must match, which we don't use).

- **Suppress unconditionally, not just on horizontal.** The container has
  `overflow: hidden`, so there is no local scroll to preserve, and we already
  forward the full wheel event to the remote desktop. Calling `preventDefault()`
  on every wheel event is simplest and safe, and also stops any browser-level
  overscroll bounce. (If a reason to keep default vertical behaviour surfaces
  during testing, this can be narrowed to `if (event.deltaX !== 0)`, but the
  default of the plan is unconditional.)

- **Scope is inherently correct.** The listener is attached to `containerRef`,
  which is local to `DesktopStreamViewer`. Nothing outside this component is
  affected, satisfying the "just the streaming component" requirement. No global
  CSS or document-level listener is added.

- **Update the misleading comment.** The existing comment states the opposite of
  the new behaviour; it must be rewritten so future readers understand that
  swipe-nav is intentionally suppressed here.

## Files Changed

| File | Change |
|------|--------|
| `frontend/src/components/external-agent/DesktopStreamViewer.tsx` | Add `event.preventDefault()` to the wheel handler, register listener with `{ passive: false }`, update the comment. |

## Testing

- **Build:** `cd frontend && yarn build`.
- **Manual (inner Helix at `localhost:8080`):** start a spec task to get a live
  desktop session, open the desktop stream viewer, and on a trackpad perform a
  two-finger horizontal swipe over the canvas. Confirm the page does NOT navigate
  back, and that scrolling still reaches remote windows.
- **Regression:** confirm swipe-back still works on other pages (e.g. navigate
  between two Helix pages and two-finger-swipe over normal page content).
