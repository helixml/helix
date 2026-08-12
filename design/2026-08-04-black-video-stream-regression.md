# Black desktop video stream — root cause and fix

**Date:** 2026-08-04
**Reported by:** Luke, on meta.helix.ml, Safari/macOS, ~12:29 BST
**Symptom:** desktop reported connected, video area blank/black for 7 minutes.

## Summary

Two independent defects, either of which produces a permanently black or frozen
canvas with **no error message**, while the socket, decoder and server pipeline all
stay perfectly healthy.

1. **`PlayoutScheduler` deadlock** — the actual cause of the reported incident.
   Engine-independent. Reproduced live.
2. **No WebGL context-loss handling** — a second, Safari-only silent-black mode.
   Real, reproduced, and fixed, but not what happened on 2026-08-04.

Both were invisible to the user, which is why the incident burned 7 minutes before
anyone knew what was happening. A render watchdog now makes any such failure loud.

## What the prod evidence ruled out

Collected while the black screen was on screen: guest desktop compositing normally
(API screenshot), NVENC at a solid 60 fps with `pipeline_dropped=0`,
`frame_size_bytes` varying 15.6k–17.1k (real changing content, not a black frame),
`ws_write_ms` p99 ≤ 1 ms (no backpressure — the browser was draining the socket),
exactly one subscribed client sustained, API proxy socket open since 11:28:57Z and
never closed.

That eliminates the guest, encoder, shared video source, WS proxy and network. The
bytes reached Safari. The failure is client-side, after decode.

## Defect 1 — PlayoutScheduler deadlock (the incident)

`frontend/src/lib/helix-stream/stream/playout-scheduler.ts`

Captured from a live frozen viewer in the inner Helix:

```json
{"targetFrames":152,"prevTargetFrames":152,"prerolling":true,
 "nominalIntervalMs":16.5,"queueLen":30,"MAX_QUEUE":30,
 "decayAccumMs":1688,"depthMs":2508,"canEverClearPreroll":false}
```

with `framesDropped` climbing ~44/s, `framesDecoded` frozen, decoder `configured`,
`receivedFirstKeyframe:true`, `reconnectAttempts:0`, `gaveUp:false`,
`glContextLost:false`, and rAF firing normally at ~53 Hz.

**Mechanism.** Presentation is gated behind `if (!this.prerolling && paceReady)`, and
`prerolling` was only ever cleared by:

```ts
if (this.prerolling && q.length > target) this.prerolling = false
```

`push()` hard-caps the queue at `MAX_QUEUE = 30`. So once `targetFrames >= 30`,
`q.length > target` is **unsatisfiable** — preroll latches forever, `present()` is
never called again, and every arriving frame is dropped at the cap. The canvas holds
its last painted frame (or stays black if none was painted) indefinitely.

**Why the target reached 152.** The depth cap was computed as
`floor(MAX_DELAY_MS / median)` where `median` is the median **socket inter-arrival**
interval — not the frame cadence. After any stall (reconnect, tab background/
foreground, decoder hiccup) the catch-up burst collapses arrival spacing to well
under 1 ms. At `median ≈ 0.78 ms` the "cap the buffer at 120 ms" rule yields 152
frames, which at the true 60 fps cadence is **2.5 seconds** — 20× the intended cap.
`raw >= targetFrames` rises instantly (peak-hold), so one burst latches it.

Recovery was `TARGET_DECAY_MS = 2000`, i.e. one frame per 2 seconds: over 4 minutes
before the target even fell below 30, and any fresh burst re-peaked it. That matches
the observed 7-minute black screen.

**Fix.**
- Cadence is now measured from **PTS deltas**, which bursts cannot distort.
- The depth target is capped below `MAX_QUEUE`, so the preroll test is structurally
  always reachable.
- Preroll ends at `q.length >= target` (was `>`, which also cost a needless frame of
  latency).

Before: 0 frames presented in 6 s, `targetFrames=152`, depth 2508 ms, queue pinned at 30.
After: **56.4 fps presented**, `targetFrames=1`, depth 50 ms, queue 4.

**Provenance.** Introduced in `bcf7a34f9` (2026-06-17, "GL→CUDA fence + playout
coalesce for 4K stale frames"), not in the two Jul 29 commits that were the prime
suspects. It needs a burst to fire, which is why it presented as an intermittent
recent regression rather than a clean break at one commit.

## Defect 2 — no WebGL context-loss handling (Safari-only)

`frontend/src/lib/helix-stream/stream/webgl-video-renderer.ts` had no
`webglcontextlost` / `webglcontextrestored` handling at all. This path runs **only**
on Safari/WebKit (`isAppleWebKit()`).

WebGL calls silently no-op on a lost context, so `texImage2D`/`drawArrays` do nothing
while decode keeps succeeding and `videoStarted` has already hidden the overlay:
black forever, no error. macOS Safari loses contexts routinely (tab backgrounding,
GPU process recycle).

Two further defects in the same area, both reproduced:

- **`dispose()` called `WEBGL_lose_context.loseContext()`.** A canvas hands out one
  context per type for its lifetime, so this permanently bricked the element. A second
  `setCanvas()` on the same canvas got that dead context back, failed in
  `buildProgram`, and the `catch` fell through to `canvas.getContext("2d")` — which
  returns **null** on a WebGL-bound canvas. Both render targets ended up null and
  every frame was dropped in silence. Verified: renderer null, ctx null, 0 fps,
  console `WebGL shader compile failed: null`.
- **`close()` destroyed the render targets.** `close()` also runs for bitrate and
  quality-mode switches, and `reconnect()` never re-established them, so a resumed
  stream had nowhere to draw.

**Fix.** Context lost/restored handlers that reallocate GL state; `draw()` returns
`false` while lost so a non-paint is never counted as a paint; `dispose()` no longer
loses the context; `setCanvas()` reuses a live target; `close()` leaves render targets
alone (rendering is already gated by the `closed` flag); the broken 2D fallback is
deleted.

**Gotcha worth remembering:** `gl.getExtension("WEBGL_lose_context")` returns **null
once the context is lost**, so fetching it inside the loss handler is too late and
restore requests silently do nothing. The handle must be captured while the context is
alive. Also, `restoreContext()` must not be called synchronously inside the
`webglcontextlost` handler — defer a tick.

**When the browser will not restore**, the context is unrecoverable: no API can get a
fresh one from that canvas. The viewer now remounts the `<canvas>` element (via a
`key` bump), which is the only way to obtain a working context again. Verified
end-to-end: unrestorable loss → watchdog → canvas replaced → fresh context → 47.3
paint fps, no user action.

## Defect 3 — the failure was silent

`VIDEO_START_TIMEOUT_MS` is cleared as soon as `videoStarted` fires, and
`videoStarted` is dispatched from the **decode** path, before any pixel reaches the
canvas. Nothing watched the window after that.

Added a render watchdog: socket open, `_videoEnabled`, first keyframe seen, but
nothing painted for 5 s ⇒ `renderStalled` (with `contextLost` / `noRenderTarget` /
`notPresenting`), surfaced in the viewer as a real message plus Retry, reusing the
existing `gaveUp` error surface. `renderRecovered` clears it.

Paints are now tracked separately from decodes (`framesPainted`). `framesDecoded`
counts decode *attempts* and kept climbing at 60/s while nothing was on screen — it is
not a health signal, and using it as one is what made the bug look invisible during
investigation.

## Also fixed

- **Retry budget** is refunded on first **paint**, not first decode. A decoded frame
  that never reaches the canvas is not a working stream — this is what
  `1a3b4902e` ("reset retry budget only when video actually flows") was reaching for.
- **`reconnectWhenVisible`** could defer a reconnect forever if a `visibilitychange`
  was missed. A dedicated poll now re-checks `document.hidden` while a reconnect is
  deferred. **Gotcha:** the obvious place for this — the heartbeat interval — does not
  work, because `stopHeartbeat()` runs in `onClose`, so the heartbeat is dead exactly
  when a deferral is pending. The first attempt at this fix was verified to do nothing
  for that reason; the poll is the only timer alive in that state.

Neither was implicated in the incident (`reconnectAttempts:0`, `gaveUp:false`
throughout the freeze), and neither would have produced a black screen — budget
exhaustion renders an error string.

## Testing

All end-to-end in the inner Helix at `localhost:8080` against a real spec-task desktop
session with a live 60 fps stream. meta.helix.ml was not touched.

Verified in a real browser, measured in **actual paints**:

| Scenario | Before | After |
|---|---|---|
| Steady state (scheduler wedged) | 0 fps, queue pinned 30, depth 2508 ms | 56.4 fps, depth 50 ms |
| Second `setCanvas()`, same element | 0 fps, both targets null | 59.0 fps |
| Restorable context loss | black forever | auto-recovers, 59.0 fps |
| Unrestorable context loss | black forever, silent | watchdog → canvas remount → 47.3 fps |
| Chrome 2D path (stub removed) | — | 51.3 → 50.0 fps across re-attach |
| Input after recovery | — | inputs sent, playout collapses to interactive/0 ms |
| Desktop container restarted under viewer | silent black | real error + cause + Retry, then auto-recovers |
| 12 forced disconnect/reconnect cycles | — | 13 sockets, no `gaveUp` latch, video live |
| Visible again with no `visibilitychange` | deferred forever | reconnects via poll |

**NOT tested: real Safari.** There is no Safari in this Linux sandbox. The WebKit
branch was exercised under Chromium by stubbing `isAppleWebKit()` to `true` (reverted
before commit) and driving context loss with `WEBGL_lose_context`. That verifies the
logic and the recovery; it does not verify WebKit's own context-loss timing or
frequency. Defect 1 is engine-independent and was reproduced directly.

A synchronous `close()` + `reconnect()` driven from the console does not restore video,
because the server dedups the rapid reuse of the same `client_id`
(`Close received after close`). That is a console-only artifact — the viewer never does
this — but it means the raw `reconnect()` path is unverified for that specific race.
