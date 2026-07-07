# fix(frontend): reset desktop reconnect retries on session wake

## Summary

When a spec task's desktop went to sleep and the user returned later, the desktop
stream viewer had already burned through its reconnect retry budget. Waking the
desktop — by sending a message or clicking **Start Desktop** — left the viewer stuck
on a stale `Connection lost - max reconnection attempts reached` error overlay instead
of reconnecting to the freshly-woken desktop. The only workaround was a full page
refresh.

The reconnect retry counters (`DesktopStreamViewer`'s `retryAttemptRef` /
`manualReconnectAttemptsRef`, and the transport's `reconnectAttempts`) only reset on a
*successful* connection, which a wake never reached because the counters were already
maxed out and the viewer stays mounted across the pause.

This wires the wake lifecycle into the viewer: when the desktop comes back from a
paused/absent state, the viewer resets its retry counters, clears the stale error, and
starts a fresh connection with a full retry budget.

## Changes

- `DesktopStreamViewer.tsx`
  - Added a `resetRetryState()` helper (zeroes the retry counters/display, clears the
    countdown, cancels pending reconnect/backoff timers). Reused by the existing
    `connectionComplete` handler.
  - Added a `wakeSignal`-change effect that resets retry state, clears `error`, and
    triggers a fresh reconnect (a fresh `WebSocketStream` resets the transport
    `reconnectAttempts` to 0, so no transport-layer change was needed).
- `DesktopStreamViewer.types.ts` — new optional `wakeSignal?: number` prop.
- `ExternalAgentDesktopViewer.tsx` — bumps a monotonic `wakeSignal` counter on the
  `absent → reachable` edge (guarded so a normal fresh page load isn't treated as a
  wake) and passes it to the viewer.

A **counter on the `absent → reachable` edge** is used rather than the transient
`starting` boolean: a fast resume goes `absent → running` between the 3s status polls,
so `starting` is often never sampled — an earlier attempt keyed on `isStarting` did not
fire. The edge approach is also trigger-agnostic (covers both message-send and Start
Desktop) since both leave the paused state. No backend changes.

## Verification

Tested end-to-end against a live spec-task desktop (Claude Code + real Zed) in the inner
Helix:
1. Booted a desktop, stream connected and displayed live Zed.
2. Stopped the desktop → viewer retried with backoff and, after 10 attempts, showed the
   `Connection lost - max reconnection attempts reached` overlay (**bug reproduced**).
3. Clicked **Start Desktop** → `Session waking - resetting reconnect retry state` logged,
   stream reconnected, error cleared, live video returned (**fix confirmed**).

Frontend `tsc -b` passes.

## Screenshots

![Desktop streaming](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002211_when-i-come-back-to-a/screenshots/01-desktop-running.png)
![Recovered after wake](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002211_when-i-come-back-to-a/screenshots/04-recovered-after-wake.png)
