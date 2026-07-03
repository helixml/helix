# Design: Reset Desktop Reconnect Retry Count When Waking a Sleeping Spec Task

## Overview

Reset the desktop-stream reconnect retry counters (and clear stale error state)
whenever the session is woken — detected by the session transitioning into the
`starting` state. Both wake triggers already funnel into that transition, so a
single reset point covers both cases.

## How the code works today (discovered)

- `ExternalAgentDesktopViewer.tsx` derives `sandboxState` from
  `session.config.external_agent_status` and computes `isStarting` /
  `isRunning` / `isPaused`. It keeps `DesktopStreamViewer` mounted once the
  desktop has ever run.
- **Wake by message:** `handleWillSend` → `optimisticallyMarkSessionStarting()`
  (`utils/optimisticSessionStarting.ts`) flips the cached
  `external_agent_status` to `starting`, so `isStarting` becomes true.
  (`SpecTaskDetailContent.tsx` and `ExternalAgentDesktopViewer.tsx`.)
- **Wake by button:** `handleStartSession` / `handleResume` call
  `v1SessionsResumeCreate(sessionId)`; the backend flips status to `starting`.
- **Retry counters** (`DesktopStreamViewer.tsx`):
  - `retryAttemptRef` (AlreadyStreaming retry, `scheduleAlreadyStreamingRetry`)
  - `manualReconnectAttemptsRef` (cap `MAX_MANUAL_RECONNECT_ATTEMPTS = 3`)
  - Both reset **only** on `connectionComplete` (lines ~919-921).
- **Transport counter** `reconnectAttempts` (cap 10) in `websocket-stream.ts`.
  It exposes a public `reconnect()` that sets `reconnectAttempts = 0` and starts
  a fresh connection.

## Key Decision

**Reset on the `starting` transition, inside `DesktopStreamViewer`.**

Both wake paths already converge on `isStarting === true`. Rather than adding
reset calls in every button handler and every send handler, we react to the
state transition in one place. This is the same signal the viewer already uses to
decide mounting/overlays, so it is reliable for both triggers.

Rationale for alternatives considered:
- *Reset in each handler* — more call sites, easy to miss the second
  `handleWillSend`/`handleResume` pair in `ExternalAgentDesktopViewer.tsx`.
- *Reset only the transport counter* — insufficient; the two
  `DesktopStreamViewer` refs would still be maxed and give up.

## Implementation

1. **Pass a wake signal into `DesktopStreamViewer`.**
   Add an optional prop (e.g. `isStarting?: boolean`, or a monotonic
   `wakeNonce?: number`) to `DesktopStreamViewerProps`
   (`DesktopStreamViewer.types.ts`). `ExternalAgentDesktopViewer.tsx` already
   knows `isStarting` and passes props to the viewer — forward it there. A nonce
   is preferred if repeated wakes must each force a reset even without a state
   change; otherwise a rising-edge `isStarting` effect is sufficient.

2. **Add a reset effect in `DesktopStreamViewer.tsx`.**
   On the rising edge of the wake signal (`false → true`, or nonce change):
   - `retryAttemptRef.current = 0`
   - `manualReconnectAttemptsRef.current = 0`
   - `setRetryAttemptDisplay(0)`
   - `setRetryCountdown(null)` and cancel any pending
     AlreadyStreaming/backoff timers
   - `setError(null)`; clear the "Connection failed" status
   - Call the transport reset + fresh connect: prefer
     `streamRef.current?.reconnect()` (which zeroes `reconnectAttempts`), or the
     component's own `reconnectRef.current(...)` if no stream exists yet.

   Reuse the existing reset block (lines ~919-921) — extract a small
   `resetRetryState()` helper so `connectionComplete` and the wake effect share
   one implementation.

3. **No backend changes.** The backend already wakes the session; this fix only
   restores the frontend's retry budget so the fresh connection can succeed.

## Files to Touch

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` — reset helper
  + wake effect.
- `frontend/src/components/external-agent/DesktopStreamViewer.types.ts` — new prop.
- `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` — pass
  the wake signal (`isStarting`/nonce) to `DesktopStreamViewer`.

## Testing / Verification

- Manually: pause a desktop until retries are exhausted (or simulate by forcing
  the counters), then (a) send a message and (b) click Start Desktop — verify it
  reconnects instead of erroring, with no page refresh.
- Confirm normal reconnect + give-up-after-max behaviour still works when no wake
  occurs.

## Gotchas

- There are **two** `handleWillSend`/wake code paths (`SpecTaskDetailContent.tsx`
  and `ExternalAgentDesktopViewer.tsx`). Resetting on the `starting` transition
  inside the viewer covers both without touching either handler.
- Cancel pending backoff timers when resetting, or a scheduled retry can fire
  after the reset and re-increment the counter.
- The AlreadyStreaming retry (`retryAttemptRef`) is uncapped, but its stale
  countdown/display should still be cleared on wake for a clean UX.
