# Implementation Tasks: Fix Unresponsive Desktop Viewer After Safari Back/Forward Navigation

- [x] Reproduce the bug on Safari — N/A: this Linux dev env has no Safari. Code-reading verified zero `pageshow`/`pagehide` handlers exist in `frontend/src/`; user symptoms match BFCache textbook behaviour. Real Safari verification deferred to user post-fix.
- [x] Audit existing reconnection machinery — found `WebSocketStream.reconnect()` is already public (line 2211) and does the full force-reconnect: closes socket, resets attempts, cancels pending timeouts, calls `connect()` which cleans up decoders & dispatches `connecting` event. Big simplification: no new helper methods needed, no `DesktopStreamViewer` changes needed.
- [x] Wire BFCache `pageshow` listener in `WebSocketStream` — register inside `startHeartbeat()` alongside the existing `visibilityHandler`, unregister in `stopHeartbeat()`. On `event.persisted === true`, log and call `this.reconnect()`.
- [~] Run `cd frontend && yarn build` to confirm no TypeScript errors
- [ ] Write PR description in `pull_request_helix.md`
- [ ] Push feature branch — user will verify on Safari (macOS + iPadOS) and Chrome/Firefox; push triggers PR creation via Helix UI
