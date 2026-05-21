# Design: Fix Unresponsive Desktop Viewer After Safari Back/Forward Navigation

## Current State (Investigation Findings)

These notes are from exploring the codebase — useful context for whoever implements:

### Components Involved
- **`frontend/src/components/external-agent/DesktopStreamViewer.tsx`** (~2700 lines): The canvas + WebCodecs video decoder + input handling. Owns the `streamRef` to a `WebSocketStream` instance. Has a single mount/unmount `useEffect` (line ~1904) that calls `disconnect()` on cleanup.
- **`frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx`** (~790 lines): Wrapper that decides whether to show stream vs screenshot mode, keeps `DesktopStreamViewer` mounted across paused/starting transitions to avoid fullscreen-exit jank. Has `showReconnectingOverlay` state already.
- **`frontend/src/lib/helix-stream/stream/websocket-stream.ts`** (~2800 lines): The WebSocket lifecycle. URL pattern `/api/v1/external-agents/{sessionId}/ws/stream`. Has heartbeat (5s interval, 10s stale timeout), exponential-backoff reconnect (1s → 8s, 10 attempts max), and `visibilitychange` handling to skip stale-detection while hidden.

### What's Already Handled
- `visibilitychange` event in `WebSocketStream.startHeartbeat()` (line 2122) — used to reset `lastMessageTime` and skip stale checks when `document.hidden`. This is for iOS JS suspension, NOT BFCache.
- Heartbeat-based reconnection if the WebSocket goes silent.
- Cleanup on React unmount (closes socket, decoder, timers, revokes cursor blob URLs).

### What's Missing (The Bug)
- **Zero references to `pageshow`, `pagehide`, `event.persisted`, or `bfcache`** anywhere in `frontend/src/` (verified via grep).
- Safari closes the WebSocket when entering BFCache, but the component's reconnect logic only fires if the WS `onclose` callback runs — and BFCache restoration may not always deliver pending `onclose` events reliably before the user interacts.
- Even if `onclose` does fire, the existing heartbeat-based stale detection has a delay (up to ~15s), during which clicks are silently lost.

## Approach

### Option A: Listen for `pageshow` and force reconnect (CHOSEN)
- Add a `pageshow` listener that checks `event.persisted`
- If true, treat it as a forced reconnect: tear down WebSocket + decoder, then re-establish
- Mirror in `pagehide` with `event.persisted` for symmetry / cleanup

**Pros:** Targeted, small surface area, well-supported pattern (recommended by web.dev and MDN BFCache guides), works on all modern browsers
**Cons:** Brief flash of reconnecting overlay on every back-forward — acceptable

### Option B: Disable BFCache entirely via `Cache-Control: no-store` on the viewer page
**Rejected:** Heavy-handed, hurts Lighthouse scores, breaks the snappy back/forward UX everywhere else, and is what other apps do badly.

### Option C: Add an "unload" listener
**Rejected:** Listening for `unload` / `beforeunload` *disqualifies the page from BFCache* in Safari and Chrome. We don't want that — we want the BFCache benefit on other pages and just want the viewer to recover.

## Implementation Plan

### Change 1: Add BFCache awareness to `WebSocketStream`
In `frontend/src/lib/helix-stream/stream/websocket-stream.ts`:

1. Add two new private fields: `pageshowHandler`, `pagehideHandler` (alongside the existing `visibilityHandler`).
2. In a new `startBFCacheHandling()` method called from `connect()` / `startHeartbeat()`:
   ```
   pageshowHandler = (event) => {
     if (event.persisted) {
       // BFCache restoration: WebSocket and VideoDecoder were forcibly closed
       this.forceReconnect("bfcache-restore")
     }
   }
   pagehideHandler = (event) => {
     if (event.persisted) {
       // Page entering BFCache - mark connection as known-dead so we don't
       // try to send input events that will fail silently
       this.markBFCacheSuspended()
     }
   }
   window.addEventListener("pageshow", pageshowHandler)
   window.addEventListener("pagehide", pagehideHandler)
   ```
3. Add a `forceReconnect(reason: string)` method that:
   - Sets `this.connected = false`
   - Closes the existing socket (with a non-1000 code, e.g. 4001, so reconnect-on-close logic doesn't suppress it)
   - Resets reconnect attempt counter to 0
   - Calls the existing reconnect path
4. Add a `markBFCacheSuspended()` method that flips `this.connected = false` and stops the input throttle from queueing further messages, but does not yet attempt to reconnect (we're in BFCache, no JS runs).
5. In `stopHeartbeat()` / `close()`, remove both new listeners alongside the existing visibility listener cleanup.

### Change 2: Surface reconnection in the UI
In `DesktopStreamViewer.tsx`:

1. Subscribe to a new `onReconnecting` callback from `WebSocketStream` (it already emits `onStatusChange`-ish events — extend or reuse).
2. When the reconnect is triggered by `bfcache-restore`, show the existing reconnecting overlay (`showReconnectingOverlay` is already wired up in `ExternalAgentDesktopViewer`).
3. Re-create the `VideoDecoder` (it was closed by Safari on BFCache entry — calling `decode()` on a closed decoder throws `InvalidStateError`).

### Change 3: Defensive — reset decoder on `pageshow`
Even if the WebSocket reconnect logic works, the `VideoDecoder` instance may need fresh recreation. In `DesktopStreamViewer.tsx`:

1. Add a `pageshow` listener at the component level that, when `event.persisted === true`, calls the existing decoder-reset path (search for where `VideoDecoder` is constructed — there should be an init function that can be re-invoked).

## Key Decisions

| Decision | Rationale |
|---|---|
| Use `pageshow` with `event.persisted` check | Standard, well-documented BFCache detection. Works on Safari, Chrome, Firefox. |
| Don't use `beforeunload` / `unload` | Those listeners disqualify the page from BFCache. |
| Force-reconnect on restore rather than try to revive | The WebSocket is genuinely dead and the VideoDecoder may be in a corrupt state — reconnecting is more reliable than patching. |
| Reuse existing `showReconnectingOverlay` | Already implemented, already styled. |
| Don't preserve in-flight input events | BFCache restoration is rare enough that losing a few queued mouse moves is acceptable. The user will move the mouse again. |

## Testing

### Manual Test Plan (REQUIRED)
1. **Safari macOS** — start a desktop streaming session, swipe-back, swipe-forward → verify reconnect within 3s, mouse works, cursor updates
2. **Safari iPadOS** — same with edge-swipe gesture
3. **Chrome** — same flow, confirm no regression (Chrome also uses BFCache as of v96)
4. **Firefox** — same flow
5. **Hard refresh on Safari** — verify normal connect path still works
6. **In-app navigation via nav links** — confirm React Router unmount cleanup still runs (not the `pagehide`-with-persisted path)

### Console Verification
- Look for new log line `[WebSocketStream] BFCache restored, force-reconnecting` on swipe-forward
- No `InvalidStateError` from VideoDecoder
- No `WebSocket is already in CLOSING or CLOSED state` errors after the reconnect

### Edge Cases
- What if the session has already been deleted server-side during the BFCache hold? → Existing reconnect logic should surface that as a session-not-found error and route to the appropriate UI
- What if a user back-forwards within milliseconds of starting a session? → Reconnect logic already handles in-flight handshake; this should just retry

## References
- web.dev: [Back/forward cache](https://web.dev/articles/bfcache) — patterns and gotchas
- MDN: [Page lifecycle API](https://developer.mozilla.org/en-US/docs/Web/API/Page_Visibility_API) — `pageshow`, `pagehide`, `event.persisted`
- WebKit blog: [Page Cache](https://webkit.org/blog/427/webkit-page-cache-i-the-basics/) — Safari's BFCache implementation
- Chrome devtools has an "Application → Back/Forward Cache" panel to verify a page is bfcache-eligible after the fix

## Risks / Open Questions
- **Unknown:** Whether the `VideoDecoder` constructor parameters (codec config) are captured cleanly enough to reconstruct without re-negotiating with the server. Implementer should verify by reading where the initial config arrives in the stream.
- **Unknown:** Whether other components on the spec-task detail page (e.g. chat panel) also hold WebSockets that need similar treatment. This task is scoped to the desktop viewer; flag any others as follow-up tickets.
- The user mentioned lag "might not only be due to [back/forward]". If after this fix lag is still reported in steady-state Safari use, open a separate ticket — don't expand scope here.
