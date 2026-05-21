# Implementation Tasks: Fix Unresponsive Desktop Viewer After Safari Back/Forward Navigation

- [~] Reproduce the bug on Safari (macOS) with the current build before changing code — confirm the symptom matches (stuck cursor, no clicks, frozen video after swipe-back-then-forward)
- [ ] In `frontend/src/lib/helix-stream/stream/websocket-stream.ts`, add `pageshowHandler` and `pagehideHandler` fields alongside the existing `visibilityHandler`
- [ ] In `websocket-stream.ts`, register `window.addEventListener("pageshow", ...)` and `("pagehide", ...)` in the connection-setup path; remove them in `stopHeartbeat()` / `close()`
- [ ] Add `forceReconnect(reason: string)` method on `WebSocketStream` that closes the socket with a non-1000 code, resets reconnect attempt count, and triggers the existing reconnect path
- [ ] Add `markBFCacheSuspended()` method that flips `connected = false` so queued input events don't try to send while the page is in BFCache
- [ ] In `pageshowHandler`, check `event.persisted === true` and call `forceReconnect("bfcache-restore")`
- [ ] In `pagehideHandler`, check `event.persisted === true` and call `markBFCacheSuspended()`
- [ ] In `frontend/src/components/external-agent/DesktopStreamViewer.tsx`, add a `pageshow` listener that on `event.persisted` re-initializes the `VideoDecoder` (the one Safari closed on BFCache entry)
- [ ] Wire the BFCache-triggered reconnect into the existing `showReconnectingOverlay` state in `ExternalAgentDesktopViewer.tsx` so users see feedback
- [ ] Add a single-line console log on BFCache-triggered reconnect (e.g. `[WebSocketStream] BFCache restored, force-reconnecting`) for support diagnostics — no other new logging
- [ ] Manual test: Safari macOS, swipe-back-then-forward — confirm video resumes within ~3s, clicks register, cursor updates
- [ ] Manual test: Safari iPadOS edge-swipe gesture — same checks
- [ ] Manual test: Chrome — confirm no regression; verify in DevTools → Application → Back/Forward Cache that the page is still bfcache-eligible
- [ ] Manual test: Firefox — confirm no regression
- [ ] Manual test: Safari hard refresh (Cmd+R) — confirm normal connect path unchanged
- [ ] Manual test: In-app React Router navigation away and back — confirm component unmount path still cleans up (not BFCache, this is a real unmount)
- [ ] Run `cd frontend && yarn build` to confirm no TypeScript errors
- [ ] Open PR against `helixml/helix` with reproduction steps and before/after notes; link this spec task
