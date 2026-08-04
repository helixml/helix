# Fix black/frozen desktop video stream and make render failures visible

## Summary

Luke hit a black video stream on Safari with a provably healthy server pipeline
(60 fps NVENC, `pipeline_dropped=0`, no backpressure, one subscribed client, proxy
socket never closed) and no error message for 7 minutes.

The reported incident was **not** caused by the two Jul 29 streaming commits, and not
by WebGL context loss. It was a **deadlock in `PlayoutScheduler`**, reproduced live in
the inner Helix. A second, independent silent-black mode (no WebGL context-loss
handling) was also found and fixed, along with the reason either one was invisible.

Full write-up with observed values: `design/2026-08-04-black-video-stream-regression.md`.

## Root cause

Presentation was gated on a preroll flag cleared only by `q.length > target`, while
`push()` caps the queue at `MAX_QUEUE = 30`. Once `targetFrames >= 30` that test is
**unsatisfiable**, so nothing was ever presented again and every frame was dropped at
the cap — canvas frozen, socket and decoder perfectly healthy.

`targetFrames` reached 152 because the depth cap was `floor(MAX_DELAY_MS / median)`
where `median` is median **socket inter-arrival** spacing, not frame cadence. Any
stall produces a catch-up burst that collapses arrival spacing below 1 ms, turning a
"120 ms max buffer" rule into a 152-frame (2.5 s) buffer. Peak-hold latched it
instantly; decay was 1 frame per 2 s, i.e. >4 minutes to recover.

Captured live while frozen:

```json
{"targetFrames":152,"prerolling":true,"queueLen":30,"MAX_QUEUE":30,
 "depthMs":2508,"canEverClearPreroll":false}
```

Introduced in `bcf7a34f9` (2026-06-17), not the Jul 29 commits. Engine-independent —
reproduced in Chromium.

## Changes

**`playout-scheduler.ts`** — cadence now measured from PTS deltas (immune to bursts);
depth target capped below `MAX_QUEUE` so the preroll test is structurally reachable;
preroll ends at `>= target` (was `>`, also costing a needless frame of latency).

**`webgl-video-renderer.ts`** — handle `webglcontextlost`/`webglcontextrestored` and
reallocate GL state on restore; `draw()` returns `false` while lost so a non-paint is
never counted as a paint; capture the `WEBGL_lose_context` handle while the context is
alive (`getExtension` returns null once lost, which silently defeated restore);
`dispose()` no longer calls `loseContext()` — it permanently bricked the canvas.

**`websocket-stream.ts`** — `setCanvas()` reuses a live render target instead of
tearing it down and re-acquiring a dead context; `close()` no longer destroys render
targets (it also runs for bitrate/mode switches — renderer lifetime belongs to the
canvas, not the socket); deleted the broken 2D fallback on the WebKit branch
(`getContext("2d")` returns null on a WebGL-bound canvas, so it silently dropped every
frame); added a render watchdog; retry budget refunded on first **paint**, not first
decode; a deferred-while-hidden reconnect now polls `document.hidden`.

**`DesktopStreamViewer.tsx`** — surface `renderStalled` as a real message with Retry,
reusing the existing `gaveUp` error surface; remount the `<canvas>` when its context is
unrecoverable, which is the only way to obtain a working context again.

## Testing

End-to-end in the inner Helix against a real spec-task desktop with a live 60 fps
stream, measured in **actual paints** (`framesDecoded` counts decode attempts and kept
climbing at 60/s while nothing was on screen — it is not a health signal).

| Scenario | Before | After |
|---|---|---|
| Steady state (scheduler wedged) | 0 fps, queue pinned 30, depth 2508 ms | 56.4 fps, depth 50 ms |
| Second `setCanvas()`, same element | 0 fps, both targets null | 59.0 fps |
| Restorable context loss | black forever | auto-recovers, 59.0 fps |
| Unrestorable context loss | black forever, silent | watchdog → canvas remount → 47.3 fps |
| Desktop container restarted under viewer | silent black | real error + cause + Retry, then auto-recovers |
| 12 forced disconnect/reconnect cycles | — | no `gaveUp` latch, video live |
| Visible again with no `visibilitychange` | deferred forever | reconnects via poll |
| Chrome 2D path (stub removed) | — | 51.3 → 50.0 fps across re-attach |

**NOT tested: real Safari** — none available in this Linux sandbox. The WebKit branch
was exercised under Chromium by stubbing `isAppleWebKit()` to `true` (reverted before
commit) and driving loss with `WEBGL_lose_context`. That verifies the logic and
recovery, not WebKit's own context-loss timing. The primary defect is
engine-independent and was reproduced directly.

A synchronous console `close()` + `reconnect()` still yields 0 fps because the server
dedups the reused `client_id` (`Close received after close`). That is not a viewer code
path; the real Retry path is verified above.

## Screenshots

![Silent black before — error surfaced after fix](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002546_debug-and-fix-the/screenshots/04-render-stalled-error-visible.png)
![Recovered after context loss](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002546_debug-and-fix-the/screenshots/05-recovered-after-context-loss.png)
![Real error during desktop restart](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002546_debug-and-fix-the/screenshots/06-after-desktop-container-restart.png)
![Auto-recovered after restart](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002546_debug-and-fix-the/screenshots/07-recovered-after-container-restart.png)
