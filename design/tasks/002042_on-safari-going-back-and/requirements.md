# Requirements: Fix Unresponsive Desktop Viewer After Safari Back/Forward Navigation

## Background

On Safari, navigating back and forward — particularly via the trackpad swipe-left/swipe-right gesture — leaves the desktop viewer (the streaming remote Linux desktop component) in a broken state:

- Severe perceived lag, or a frozen image
- The viewer no longer accepts mouse clicks
- The cursor gets "stuck" displaying one shape (e.g. text-caret, hand) and never updates

Other browsers (Chrome, Firefox) appear less affected, but the user has reproduced this primarily on Safari.

## Root Cause (Hypothesis)

Safari aggressively uses its **Back/Forward Cache (BFCache)** to instantly restore previous pages. When a page enters BFCache, Safari:

- Forcibly closes all WebSockets (since 2018; see [Apple WebKit BFCache docs](https://webkit.org/blog/427/webkit-page-cache-i-the-basics/))
- Suspends `requestAnimationFrame` loops
- Closes WebCodecs `VideoDecoder` instances

The Helix desktop viewer (`DesktopStreamViewer.tsx` + `websocket-stream.ts`) currently has **no `pageshow` / `pagehide` event handling** and no `event.persisted` checks. After a swipe-back-then-forward:

1. The WebSocket is dead but `this.connected === true` in `WebSocketStream`
2. The `VideoDecoder` is closed but the canvas render loop still references it
3. Mouse events are serialized and queued onto the dead socket
4. The cursor state (held in refs `cursorImageRef` / `cursorCssNameRef`) is whatever it was at the moment of BFCache entry, with no new cursor messages arriving

The component does correctly handle `visibilitychange` for iOS JS suspension, but BFCache is a different lifecycle event.

## User Stories

### US-1: Recovery After Swipe Back/Forward
**As a** Safari user with an active desktop session
**I want** the viewer to recover automatically after I swipe back and then forward (or use the back/forward buttons)
**So that** I can continue working without manually refreshing the page

**Acceptance:**
- After back-then-forward on Safari, video resumes streaming within ~3 seconds
- Mouse clicks register and are sent to the remote desktop
- Cursor updates correctly as the pointer moves over different UI regions
- No console errors about closed WebSocket / closed decoder during recovery

### US-2: Clear Feedback During Recovery
**As a** user
**I want** to see a visual indicator (reconnecting overlay) during the brief reconnection window
**So that** I know the app is recovering rather than wondering if it's broken

**Acceptance:**
- A reconnecting overlay appears within 500ms of BFCache restoration if reconnection is in progress
- Overlay disappears once stream resumes

### US-3: No Regression on Non-BFCache Navigation
**As a** user on any browser
**I want** normal in-app route changes (clicking nav links) and page reloads to continue working as today
**So that** the BFCache fix doesn't introduce new bugs

**Acceptance:**
- Chrome / Firefox / Edge: no behavior change for normal navigation
- Safari hard refresh (Cmd+R): no behavior change
- React Router in-app navigation: no behavior change (component unmount path)

## Scope

**In scope:**
- Detecting BFCache restoration via `pageshow` with `event.persisted === true`
- Tearing down and re-establishing the stream connection (WebSocket + VideoDecoder + render loop)
- Visible reconnection state via the existing `showReconnectingOverlay` mechanism

**Out of scope:**
- Preserving in-flight input events across BFCache (best-effort only — closing the socket is correct, lost mouse moves are acceptable)
- Investigating non-BFCache lag separately (the user noted "it might not only be due to that" — those reports should become separate tasks if reproduced)
- Disabling BFCache via `Cache-Control: no-store` (anti-pattern — hurts non-viewer pages and is what other apps do badly)

## Reproduction Steps

1. Open Safari (macOS or iPadOS), log in to Helix
2. Start a spec task or sandbox with a streaming desktop
3. Wait for the viewer to display video and accept clicks
4. Navigate to another page within the app, OR swipe-left on the trackpad to go back
5. Swipe-right (or click forward) to return to the viewer page
6. Observe: black/frozen image, cursor stuck, clicks ignored
