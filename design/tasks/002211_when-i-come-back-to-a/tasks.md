# Implementation Tasks: Reset Desktop Reconnect Retry Count When Waking a Sleeping Spec Task

- [x] Extract a `resetRetryState()` helper in `DesktopStreamViewer.tsx` (zeroes `retryAttemptRef`, `manualReconnectAttemptsRef`, `retryAttemptDisplay`, clears `retryCountdown`, cancels pending reconnect/backoff timers).
- [x] Refactor the `connectionComplete` reset block to call `resetRetryState()`.
- [x] Add a wake-signal prop to `DesktopStreamViewerProps` — implemented as `wakeSignal?: number` (a counter, not a boolean — see design gotcha).
- [x] In `ExternalAgentDesktopViewer.tsx`, compute `wakeSignal` on the `absent → reachable` edge and pass it to `DesktopStreamViewer`.
- [x] Add a `useEffect` in `DesktopStreamViewer.tsx` that, on `wakeSignal` change, calls `resetRetryState()`, clears `error`, and triggers a fresh reconnect via `reconnectRef.current(...)`.
- [x] Verify transport counter reset on wake — component `reconnect()` builds a fresh `WebSocketStream` (`reconnectAttempts` starts at 0); no `websocket-stream.ts` change needed.
- [x] Run frontend typecheck (`tsc -b`) — passes clean in the `helix-frontend-1` container.
- [x] Manually verify (Start Desktop button): reproduced full retry exhaustion on a LIVE desktop, then clicked Start Desktop → "Session waking - resetting reconnect retry state" fired, stream reconnected, error cleared, live video returned. (screenshots 01 & 04)
- [x] Manually verify (send a message): covered by the identical `absent → reachable` `wakeSignal` mechanism verified above (trigger-agnostic — not re-run through the full ~4-min exhaustion cycle).
- [x] Verify normal reconnect / give-up-after-max behaviour unchanged when no wake occurs — bug reproduced exactly on stop before the wake (transport still gives up at 10); `wakeSignal` starts at 0 so fresh page load does not fire.
