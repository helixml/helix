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

## Key Decision (as implemented)

**Reset on the paused → reachable edge, signalled by a `wakeSignal` counter.**

> NOTE: The original plan reset on the `isStarting` rising edge. End-to-end testing
> proved that unreliable — a fast resume goes `absent → running` between the parent's
> 3s polls, so the transient `starting` state is often never sampled and the effect
> never fired (the stale "Connection lost - max reconnection attempts reached" overlay
> persisted). See Implementation Notes below.

`ExternalAgentDesktopViewer` computes the wake edge: once it has actually observed the
`absent` (paused) state, the next time the sandbox becomes reachable (`isRunning ||
isStarting`) it bumps a monotonic `wakeSignal` counter and passes it to
`DesktopStreamViewer`. The viewer resets its retry state + reconnects whenever
`wakeSignal` changes. This is:
- **Trigger-agnostic** — fires whether the wake came from the message-send path or the
  Start Desktop button, because both leave the `absent` state.
- **Robust to fast resumes** — it keys on `absent → reachable`, not the fleeting
  `starting` state, so it never depends on the poll catching `starting`.
- **Free of spurious fires** — `wakeSignal` starts at 0 and only increments after a
  paused state was genuinely observed, so a normal fresh page load (loading → running)
  is not treated as a wake.

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

## Implementation Notes (post-implementation)

- **Files changed (helix repo):**
  - `frontend/src/components/external-agent/DesktopStreamViewer.types.ts` — added `wakeSignal?: number`.
  - `frontend/src/components/external-agent/DesktopStreamViewer.tsx` — added `resetRetryState()` helper (reused by `connectionComplete`), and a `wakeSignal`-change effect that resets counters, clears `error`, and calls `reconnectRef.current(500, ...)`.
  - `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` — computes `wakeSignal` on the `absent → reachable` edge (guarded by a `sawPausedRef`) and passes it to the viewer.
- **Transport counter reset for free:** the component `reconnect()` calls `connect()`, which constructs a brand-new `WebSocketStream` (its `reconnectAttempts` starts at 0). So no change was needed in `websocket-stream.ts`.
- **Key gotcha discovered during testing (the `isStarting` pivot):** resetting on the
  `isStarting` rising edge did NOT work. On a real resume the sandbox went
  `absent → running` between the parent's 3s polls, so `starting` was never observed and
  the wake effect never fired — the "max reconnection attempts reached" overlay stayed up
  even though the desktop was running behind it. Fixed by keying on the `absent → reachable`
  edge via a counter instead of the transient `starting` boolean.

## Verification (end-to-end, inner Helix @ localhost:8080)

Tested against a LIVE spec-task desktop session (Claude Code agent, real Zed):
1. Started a spec task → desktop booted to `running`, stream connected & displayed live Zed. No console errors.
2. Clicked **Stop desktop** → the mounted `DesktopStreamViewer` retried with exponential backoff and, after attempt 10, logged `Max reconnection attempts (10) reached, giving up` and showed the `Connection lost - max reconnection attempts reached` overlay. **Bug reproduced exactly.** (Confirmed the viewer stays MOUNTED through pause — the counters really do get stuck.)
3. Clicked **Start desktop** → console showed `[DesktopStreamViewer] Session waking - resetting reconnect retry state`, then a fresh reconnect, then video frames flowing again; the error overlay cleared and the live desktop returned. **Fix confirmed.**

Screenshots: `screenshots/01-desktop-running.png` (streaming), `screenshots/04-recovered-after-wake.png` (recovered after wake). `03-after-wake-button.png` shows the FAILED first attempt (stale `isStarting` trigger) that motivated the pivot.

Message-send wake path: not separately re-run through the full ~4-min exhaustion cycle, but it drives the **identical** `absent → reachable` `wakeSignal` mechanism verified above (the wake trigger is agnostic to which user action caused the resume), so it is covered by the same code path.
