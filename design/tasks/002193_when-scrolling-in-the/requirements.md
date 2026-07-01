# Requirements: Disable Chrome Swipe-Back Gesture in Desktop Stream Viewer

## Background

The desktop streaming viewer (`DesktopStreamViewer`) lets a user interact with a
remote desktop rendered into a `<canvas>`. Wheel/scroll events inside the viewer
are forwarded to the remote desktop so the user can scroll remote windows.

However, on Chrome (and Safari) a two-finger horizontal trackpad swipe — or
horizontal wheel movement — is interpreted by the browser as the native
"swipe back/forward" navigation gesture. Because the viewer forwards wheel
events without preventing the browser default, scrolling inside the desktop
viewer can accidentally navigate the whole Helix page *away* from the session
(the "back" gesture fires), losing the user's place.

## User Story

**As a** user interacting with a remote desktop in the streaming viewer,
**I want** horizontal scroll/swipe gestures to stay inside the viewer,
**so that** I don't accidentally trigger the browser's back/forward navigation
and get bounced off the session.

## Acceptance Criteria

1. **Swipe-back is suppressed inside the viewer.** When the pointer is over the
   desktop stream viewer, a two-finger horizontal trackpad swipe (or horizontal
   wheel scroll) does NOT trigger Chrome's back/forward page navigation.

2. **Scroll forwarding still works.** Vertical and horizontal wheel events are
   still forwarded to the remote desktop as before (remote windows can still be
   scrolled).

3. **Scoped to the streaming component only.** The change applies ONLY to the
   desktop stream viewer container. Normal Chrome swipe-to-navigate continues to
   work everywhere else in the Helix app (other pages, sidebars, chat, etc.).

4. **No regression to existing input handling.** Touch handling (trackpad mode,
   pinch-zoom, tap-to-click), keyboard input, and mouse input continue to behave
   as they do today.

## Out of Scope

- Changing scroll/gesture behaviour anywhere outside `DesktopStreamViewer`.
- Adding user-configurable settings to toggle this behaviour.
- Changes to the screenshot-fallback viewer's input model beyond what is needed
  to keep parity with the canvas viewer.
