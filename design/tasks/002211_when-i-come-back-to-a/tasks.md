# Implementation Tasks: Reset Desktop Reconnect Retry Count When Waking a Sleeping Spec Task

- [~] Extract a `resetRetryState()` helper in `DesktopStreamViewer.tsx` that zeroes `retryAttemptRef`, `manualReconnectAttemptsRef`, and `retryAttemptDisplay`, clears `retryCountdown`, and cancels pending reconnect/backoff timers.
- [~] Refactor the existing `connectionComplete` reset block (~lines 919-921) to call the new `resetRetryState()` helper.
- [~] Add a wake-signal prop (`isSessionStarting?: boolean`) to `DesktopStreamViewerProps` in `DesktopStreamViewer.types.ts`.
- [~] In `ExternalAgentDesktopViewer.tsx`, pass the existing `isStarting` value into the `DesktopStreamViewer` element as `isSessionStarting`.
- [~] Add a `useEffect` in `DesktopStreamViewer.tsx` that, on the rising edge of the wake signal, calls `resetRetryState()`, clears `error`, and triggers a fresh reconnect via `reconnectRef.current(...)` (a fresh stream resets the transport `reconnectAttempts` to 0).
- [ ] Verify the transport-level `reconnect()` in `websocket-stream.ts` is invoked so `reconnectAttempts` (cap 10) is also reset on wake.
- [ ] Manually verify: after retries are exhausted on a slept task, sending a message reconnects the desktop without a page refresh.
- [ ] Manually verify: after retries are exhausted on a slept task, clicking Start Desktop reconnects the desktop without a page refresh.
- [ ] Verify normal reconnect and the existing give-up-after-max-attempts behaviour are unchanged when no wake trigger occurs.
- [ ] Run frontend lint/typecheck (`tsc`) for the touched files.
