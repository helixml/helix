# Implementation Tasks: Reset Desktop Reconnect Retry Count When Waking a Sleeping Spec Task

- [x] Extract a `resetRetryState()` helper in `DesktopStreamViewer.tsx` that zeroes `retryAttemptRef`, `manualReconnectAttemptsRef`, and `retryAttemptDisplay`, clears `retryCountdown`, and cancels pending reconnect/backoff timers.
- [x] Refactor the existing `connectionComplete` reset block to call the new `resetRetryState()` helper.
- [x] Add a wake-signal prop (`isSessionStarting?: boolean`) to `DesktopStreamViewerProps` in `DesktopStreamViewer.types.ts`.
- [x] In `ExternalAgentDesktopViewer.tsx`, pass the existing `isStarting` value into the `DesktopStreamViewer` element as `isSessionStarting`.
- [x] Add a `useEffect` in `DesktopStreamViewer.tsx` that, on the rising edge of the wake signal, calls `resetRetryState()`, clears `error`, and triggers a fresh reconnect via `reconnectRef.current(...)`.
- [x] Verify the transport-level counter is reset on wake — the component `reconnect()` calls `connect()` which constructs a fresh `WebSocketStream` (`reconnectAttempts` starts at 0), so no `websocket-stream.ts` change is needed.
- [x] Run frontend typecheck (`tsc -b`) — passes clean in the `helix-frontend-1` container.
- [ ] Manually verify: after retries are exhausted on a slept task, sending a message reconnects the desktop without a page refresh.
- [ ] Manually verify: after retries are exhausted on a slept task, clicking Start Desktop reconnects the desktop without a page refresh.
- [ ] Verify normal reconnect and the existing give-up-after-max-attempts behaviour are unchanged when no wake trigger occurs.
