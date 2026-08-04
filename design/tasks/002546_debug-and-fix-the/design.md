# Design: Fix Black Desktop Video Stream from WebGL Context Loss

## 1. Evidence and what it rules out

The brief's live diagnostics establish, as facts, that every server-side stage was
healthy while the screen was black: guest composed and rendering, NVENC at 60 fps,
`pipeline_dropped=0`, varying frame sizes (real content), no TCP backpressure, one
subscribed client, proxy socket open and never closed.

That eliminates the guest, the encoder, the shared video source, the WebSocket proxy,
and the network. It also eliminates "the client went away" — the subscriber count would
have dropped and the pipeline would have stopped after the 60 s grace period.

The failure is therefore **client-side, after decode**: bytes arrive, get decoded, and
never become pixels.

## 2. Root cause

### Finding 1 (primary) — the WebGL2 renderer has no context-loss handling

`frontend/src/lib/helix-stream/stream/webgl-video-renderer.ts` is 141 lines and contains
**no** `webglcontextlost` or `webglcontextrestored` listener, and no `isContextLost()`
check. This path is taken **only on Safari/WebKit** (`websocket-stream.ts:2053`,
`isAppleWebKit()`), which is exactly the engine in the report.

WebGL's documented behaviour on context loss is that calls become no-ops rather than
throwing. So after a loss:

| Observable | State while black |
|---|---|
| WebSocket | open, draining, no backpressure |
| `VideoDecoder` | configured, decoding successfully |
| `receivedFirstKeyframe` | `true` |
| `videoStarted` event | already dispatched → connecting overlay hidden |
| `reconnectAttempts` / `gaveUp` | untouched — nothing reconnects, nothing gave up |
| `gl.texImage2D` / `gl.drawArrays` | silent no-ops |
| Canvas | black, forever |

Every row matches an observation in the brief, including the crucial one: **black, not
an error message**. This resolves Hypothesis C — the overlay is correctly hidden because
`videoStarted` genuinely fired; the canvas is genuinely blank.

Context loss on macOS Safari is routine: tab backgrounding, GPU process recycle, or the
system switching GPUs. That explains "more than once in the past week" and the
Safari-exclusivity.

### Finding 2 — `setCanvas()` re-acquires the context it just destroyed

`websocket-stream.ts:2043`:

```ts
setCanvas(canvas) {
  this.videoRenderer?.dispose()          // dispose() calls WEBGL_lose_context.loseContext()
  this.videoRenderer = null
  this.canvasCtx = null
  if (isAppleWebKit()) {
    try { this.videoRenderer = new WebGLVideoRenderer(canvas) }   // getContext("webgl2") on the SAME canvas
    catch (e) { this.canvasCtx = canvas.getContext("2d", ...) }   // returns null — see below
  }
  ...
}
```

A canvas element has at most one context per type, and `getContext` returns the
*existing* one. `dispose()` (`webgl-video-renderer.ts:105`) explicitly calls
`loseContext()`. So a second `setCanvas()` on the same element hands the new renderer
the context it just force-lost. Construction then fails inside `buildProgram` (shader
compile/link status is falsy on a lost context) and throws.

The `catch` calls `canvas.getContext("2d")` — which returns **null**, because the canvas
is already bound to a WebGL context. Result: `videoRenderer === null && canvasCtx === null`,
so `websocket-stream.ts:1061` drops every frame:

```ts
if (!this.canvas || (!this.videoRenderer && !this.canvasCtx)) {
  frame.close(); this.framesDropped++; return
}
```

Black screen, no error. Reachable today via the screenshot → video quality-mode switch
(`DesktopStreamViewer.tsx:1956`), which calls `setCanvas()` on the same `canvasRef.current`.

### Finding 3 — `close()` clears the canvas; `reconnect()` never restores it

`close()` (`:2487-2490`) nulls `canvas`, `canvasCtx` and `videoRenderer`. `reconnect()`
(`:2444`) sets `closed = false` and calls `connect()`, but `connect()` only does
`cleanupDecoders()` + `resetStreamState()` — **neither re-establishes the canvas or the
renderer**. Any `close()` → `reconnect()` on the same stream object therefore hits the
`:1061` guard on every frame. Same silent-black signature, and this one is
engine-independent.

### Finding 4 (secondary, real, but not this incident) — retry budget

Since `1a3b4902e`, `onOpen()` no longer resets `reconnectAttempts`; the only refund site
is inside the `!receivedFirstKeyframe` block (`:1353-1359`), and three paths reset
`receivedFirstKeyframe = false` (decoder `error` callback, `decode()` catch,
`setVideoEnabled(true)`). Six disconnects in 20 minutes were observed. Worth fixing.

But budget exhaustion sets `gaveUp` and `DesktopStreamViewer.tsx:1042-1050` renders an
error string. **Luke saw black, not an error** — so this did not cause the incident.
Similarly, `reconnectWhenVisible` deferring forever leaves the socket closed, which the
server would have observed as an unsubscribe; the brief shows `clients=1` sustained.

### Finding 5 — no watchdog covers the post-`videoStarted` window

`VIDEO_START_TIMEOUT_MS` (15 s, `DesktopStreamViewer.tsx:642`) is cleared as soon as
`videoStarted` fires. `videoStarted` is dispatched from the **decode** path
(`websocket-stream.ts:1363`), before any pixel reaches the canvas. So once decoding
starts, the user has zero protection against a renderer that paints nothing. This is
why the failure was silent for 7 minutes.

## 3. Design decisions

**D1 — Handle context loss in the renderer, not around it.**
`WebGLVideoRenderer` owns the context, so it owns its lifecycle. Register
`webglcontextlost` with `preventDefault()` (required — without it the browser will not
attempt a restore) and `webglcontextrestored` to reallocate program, VAO, texture and
uniforms. Extract the existing constructor GL-setup into a private `initGL()` reused by
both. `draw()` guards on `gl.isContextLost()`.

*Rejected:* detecting loss in `websocket-stream.ts` and rebuilding the renderer. That
leaks GL lifecycle into the stream client and cannot work anyway — the canvas is already
bound to the dead context (Finding 2).

**D2 — Stop `dispose()` destroying a canvas that will be reused.**
Remove the `loseContext()` call from `dispose()`. It exists to free GPU memory but makes
the canvas permanently unusable, which is strictly worse. `setCanvas()` on the same
element should reuse the live renderer rather than tear down and rebuild.

**D3 — Delete the 2D fallback in the WebKit branch.**
It is a forbidden dual code path *and* it does not work (`getContext("2d")` returns null
after WebGL2 binding). If WebGL2 is genuinely unavailable on WebKit, surface a real
error through the existing info-event channel. This satisfies "delete anything the fix
makes dead".

**D4 — `connect()` must guarantee a live render target.**
Establishing the renderer belongs with the rest of connection setup, not only in the
React ref callback. Keep the canvas reference across `close()`/`reconnect()`, or have
`connect()` re-establish the renderer from the retained canvas. Findings 2 and 3 are the
same underlying mistake — renderer lifetime is not tied to stream lifetime.

**D5 — Paint watchdog as a first-class stream event.**
Track `lastFramePaintedAt` at the actual `draw()` call site (not at decode). If the
socket is open, frames are decoding, and nothing has painted for **5 s** (assumption —
see requirements Open Question 3), dispatch a new info event. `DesktopStreamViewer`
renders a real message plus Retry, reusing the existing `gaveUp` error surface at
`:1042-1050` so there is one error path, not two.

**D6 — Fix the retry-budget refund at its source.**
Refund on evidence that video is genuinely flowing, decoupled from
`receivedFirstKeyframe` (which three unrelated paths clear). Bound the
`reconnectWhenVisible` deferral so a missing `visibilitychange` cannot defer forever.

## 4. Files to change

| File | Change |
|---|---|
| `frontend/src/lib/helix-stream/stream/webgl-video-renderer.ts` | Context-loss/restore handling; `initGL()` extraction; `isContextLost()`; `dispose()` no longer calls `loseContext()` |
| `frontend/src/lib/helix-stream/stream/websocket-stream.ts` | `setCanvas()` reuse (`:2043`); renderer lifetime tied to connect (`:2487`, `:2444`); delete 2D fallback (`:2058`); paint watchdog at the `draw()` site (`:1114`); retry-budget refund (`:1353`); visibility-gate bound (`:570`) |
| `frontend/src/lib/helix-stream/stream/websocket-stream.types.ts` | New info-event variant(s) for render failure / paint stall |
| `frontend/src/components/external-agent/DesktopStreamViewer.tsx` | Render the new error state with a working Retry; reuse the `gaveUp` surface (`:1042`) |
| `design/2026-08-04-black-video-stream-regression.md` | Write-up |

No server-side changes are expected — the brief proves the server was healthy.

## 5. Verification plan

Testing happens **entirely in the inner Helix at `http://localhost:8080`**. meta.helix.ml
is not touched.

1. Register (`test@helix.ml` / `helixtest`), onboard, create a spec task so a real desktop
   session with a live stream exists. Per CLAUDE.md a spec task is required — a bare
   `agent_type=zed_external` chat session never gets a workspace and Zed never connects.
2. Instrument first. Log `videoDecoder?.state`, `receivedFirstKeyframe`,
   `reconnectAttempts`, `gaveUp`, `gl.isContextLost()`, and a painted-frame counter at
   the `draw()` call site. Report observed values, not inferences.
3. Force the WebKit branch by stubbing `isAppleWebKit()` to `true` so the WebGL2 renderer
   runs under Chromium (temporary local edit, reverted before commit).
4. Reproduce each finding independently:
   - **F1:** `gl.getExtension('WEBGL_lose_context').loseContext()` mid-stream → expect black
     today; expect auto-recovery after the fix.
   - **F2:** toggle quality mode video → screenshot → video → expect black today.
   - **F3:** drive `close()` then `reconnect()` on the same stream → expect black today.
5. Confirm before/after in a real browser with screenshots against a real desktop session.
6. Exercise the *next* operation after each recovery (interact with the desktop, confirm
   input still works), not just the state change.

**Stated limitation:** Safari is not available in the sandbox. Hypotheses A and C are
engine-independent and fully testable. For the WebKit-specific renderer, the code path is
exercised under Chromium via the `isAppleWebKit()` stub and explicit `WEBGL_lose_context`,
which verifies the *logic*; it does not verify WebKit's own context-loss timing. The
write-up must say so plainly rather than claiming Safari coverage.

## 6. Notes for future agents

- **`isAppleWebKit()` gates a genuinely Safari-only render path.** Any bug reported as
  "Safari only" in the desktop viewer should start at `webgl-video-renderer.ts`. Chrome
  never executes it, so it gets a fraction of the real-world testing the 2D path gets.
- **A canvas element binds one context per type, permanently.** `getContext` returns the
  existing context. Once you call `loseContext()` on a canvas, that canvas is done — you
  cannot get a fresh WebGL context *or* a 2D context from it. Rebuild the DOM node, or
  never lose the context.
- **WebGL context loss does not throw.** Calls silently no-op. Any renderer without an
  explicit `webglcontextlost` listener fails invisibly, which is the worst failure mode.
- **`videoStarted` means "decoded", not "displayed".** It is dispatched from the decode
  path. Do not use it as evidence that pixels reached the user — that gap is exactly what
  hid this bug for 7 minutes.
- **`websocket-stream.ts:1061` is a silent frame sink.** If both render targets are null
  it increments `framesDropped` and returns. When debugging a black screen, check
  `framesDropped` climbing against `framesDecoded` — that pair distinguishes "nothing
  decoded" from "decoded but nowhere to draw".
- **Order of investigation that worked here:** trust the server-side evidence in the
  brief, then read the render path bottom-up from the canvas rather than top-down from
  the socket. The socket was never the problem.

---

# ROOT CAUSE FOUND — corrects §2 above

**The planning hypothesis (WebGL context loss) was WRONG.** Reproduced live in the inner
Helix on 2026-08-04, the actual mechanism is an unsatisfiable-condition **deadlock in
`PlayoutScheduler`** (`frontend/src/lib/helix-stream/stream/playout-scheduler.ts`).

## Live proof

Captured from the running viewer while the canvas was frozen, socket healthy:

```json
{"targetFrames":152,"prevTargetFrames":152,"prerolling":true,
 "nominalIntervalMs":16.5,"queueLen":30,"MAX_QUEUE":30,
 "decayAccumMs":1688,"depthMs":2508,"canEverClearPreroll":false}
```

Alongside: `framesDropped` climbing by ~44/s, `framesDecoded` frozen at 2,
`decoderState:"configured"`, `receivedFirstKeyframe:true`, `reconnectAttempts:0`,
`gaveUp:false`, `glContextLost:false`, rAF firing at ~53 Hz.

## The mechanism

`tick()` only ever clears the preroll hold with:

```ts
if (this.prerolling && q.length > target) this.prerolling = false
```

and `present()` is gated behind `if (!this.prerolling && paceReady)`.

But `push()` hard-caps the queue at `MAX_QUEUE = 30`. So whenever
`targetFrames >= MAX_QUEUE`, `q.length > target` is **unsatisfiable** — `prerolling`
latches `true` forever, `present()` is never called again, and every arriving frame is
dropped at the queue cap. The canvas freezes on its last painted frame (or stays black if
none was ever painted) while the socket, decoder and encoder all stay perfectly healthy.

## Why `targetFrames` reaches 152

```ts
const maxFrames = Math.max(1, Math.floor(this.MAX_DELAY_MS / median))   // MAX_DELAY_MS = 120
raw = Math.max(1, Math.min(maxFrames, Math.round(jitter / median)))
```

`median` is the median **inter-arrival** interval, not the frame cadence. After any stall
(reconnect, tab background/foreground, decoder hiccup, a throttled rAF period) the socket
delivers a catch-up **burst** whose inter-arrival spacing collapses to well under 1 ms.
With `median ≈ 0.78 ms`, `maxFrames = floor(120/0.78) = 152` — so the "cap the buffer at
120 ms" invariant silently becomes a 152-frame buffer, which at the real 60 fps cadence is
**2.5 seconds**, 20× the intended cap. `raw >= targetFrames` rises instantly (peak-hold),
so a single burst latches it.

Recovery is `TARGET_DECAY_MS = 2000` → **one frame per 2 seconds**. From 152 that is over
4 minutes before `target` even falls under 30 and presentation can resume — and any new
burst re-peaks it to the top. That is exactly Luke's 7-minute black screen.

## Why it looked Safari-only and looked like a recent regression

It is **engine-independent** — I reproduced it in Chromium. Safari just hits the trigger
far more often: WebKit throttles background tabs harder, so the foreground catch-up burst
that poisons `median` is routine there. The scheduler landed in `bcf7a34f9`
(2026-06-17, "GL→CUDA fence + playout coalesce for 4K stale frames"), well before the two
Jul 29 commits in the brief's suspect list. It needs a burst to fire, which is why it
presents as intermittent and recent rather than as a clean regression at one commit.

## What this means for the original hypotheses

- **Hypothesis A (retry budget)** — not implicated. Observed `reconnectAttempts:0`,
  `gaveUp:false` during the freeze. Still a latent defect; fixing separately per plan.
- **Hypothesis B (WebGL context loss)** — not the cause. Observed `glContextLost:false`
  throughout. The missing `webglcontextlost` handling is nevertheless a real gap that
  produces an identical silent-black failure, so it is still worth fixing.
- **Hypothesis C (state machine/overlay)** — resolved: the overlay correctly hides because
  `videoStarted` genuinely fired. The canvas is genuinely frozen. Confirms the brief's
  instinct that establishing this early splits the diagnosis.

The "black screen must never be silent" requirement (US-3) is unchanged and now clearly
the most valuable part of the fix: every one of these mechanisms is invisible today.
