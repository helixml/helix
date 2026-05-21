# fix(frontend): recover desktop viewer after Safari BFCache restore

## Summary

On Safari, navigating back-and-forth (especially via the trackpad swipe gesture) left the streaming desktop viewer frozen: stuck cursor, ignored mouse clicks, perceived lag. The same effect is reproducible on Chrome and Firefox to a lesser degree.

Root cause: Safari (and Chrome/Firefox) put the page into the back/forward cache on navigation. The browser silently kills the WebSocket on BFCache entry. The viewer's `WebSocketStream` had no `pageshow` listener, so on restore it believed it was still connected — but the socket was dead, the `VideoDecoder` was closed, and input events were being queued onto a dead pipe.

## Changes

- `frontend/src/lib/helix-stream/stream/websocket-stream.ts`: register a `window` `pageshow` listener inside `startHeartbeat()` (alongside the existing `visibilitychange` listener) that calls the already-public `this.reconnect()` when `event.persisted === true`. Unregister it in `stopHeartbeat()`. ~15 LOC total.

The existing `reconnect()` method already does everything needed for a clean restart: closes the dead socket, cancels any pending reconnect timeout, resets the attempt counter, and calls `connect()` which itself runs `cleanupDecoders()` and `resetStreamState()`. The `VideoDecoder` is recreated lazily by `onMessage` on the next keyframe, so no extra plumbing is required.

## Why minimal

- No `pagehide` handler: pages in BFCache are frozen — no JS runs, so there's nothing to "suspend".
- No changes in `DesktopStreamViewer.tsx` or `ExternalAgentDesktopViewer.tsx`: `WebSocketStream` already dispatches the `connecting`/`connected` info events that the wrapper components subscribe to for their reconnecting UI.
- No `beforeunload`/`unload` listeners (those would *disqualify* the page from BFCache — the opposite of what we want for the rest of the app).

## Testing

- `npx tsc --noEmit`: clean (exit 0)
- `yarn build`: all 21,115 modules transformed (the dev container's bind-mounted `dist/` prevents the actual write step but compilation succeeds)
- **Manual verification by user** required on:
  - Safari macOS — back-then-forward via trackpad swipe and arrow buttons
  - Safari iPadOS — edge-swipe gesture
  - Chrome / Firefox — confirm no regression
  - Safari hard refresh (Cmd+R) — confirm normal connect path unchanged
  - In-app React Router navigation away and back — confirm component unmount path still cleans up

## Specs

See [helix-specs design docs](https://github.com/helixml/helix/tree/helix-specs/design/tasks/002042_on-safari-going-back-and) for full investigation notes, decisions, and rejected alternatives.
