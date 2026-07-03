# Implementation Tasks: Reset Desktop Reconnect Retry Count When Waking a Sleeping Spec Task

- [ ] Extract a `resetRetryState()` helper in `DesktopStreamViewer.tsx` that zeroes `retryAttemptRef`, `manualReconnectAttemptsRef`, and `retryAttemptDisplay`, clears `retryCountdown`, cancels pending reconnect/backoff timers, and clears `error` state.
- [ ] Refactor the existing `connectionComplete` reset block (~lines 919-921) to call the new `resetRetryState()` helper.
- [ ] Add a wake-signal prop (`isStarting?: boolean` or a monotonic `wakeNonce?: number`) to `DesktopStreamViewerProps` in `DesktopStreamViewer.types.ts`.
- [ ] In `ExternalAgentDesktopViewer.tsx`, pass the wake signal (its existing `isStarting`, or a nonce that increments on each wake) into the `DesktopStreamViewer` element.
- [ ] Add a `useEffect` in `DesktopStreamViewer.tsx` that, on the rising edge of the wake signal, calls `resetRetryState()` and then triggers a fresh reconnect (`streamRef.current?.reconnect()` to zero the transport `reconnectAttempts`, or `reconnectRef.current(...)` if no stream exists yet).
- [ ] Verify the transport-level `reconnect()` in `websocket-stream.ts` is invoked so `reconnectAttempts` (cap 10) is also reset on wake.
- [ ] Manually verify: after retries are exhausted on a slept task, sending a message reconnects the desktop without a page refresh.
- [ ] Manually verify: after retries are exhausted on a slept task, clicking Start Desktop reconnects the desktop without a page refresh.
- [ ] Verify normal reconnect and the existing give-up-after-max-attempts behaviour are unchanged when no wake trigger occurs.
- [ ] Run frontend lint/typecheck (`tsc`) for the touched files.
