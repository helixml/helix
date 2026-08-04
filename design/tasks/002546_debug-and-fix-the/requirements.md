# Requirements: Fix Black Desktop Video Stream from WebGL Context Loss

## Background

On meta.helix.ml (2026-08-04 ~12:29 BST), Luke clicked **Restart** on a spec-task
desktop in **Safari/macOS**. The UI reported "connected" but the video area stayed
black for 7 minutes with no error message.

The live diagnostics in the brief prove the server side was healthy throughout:
guest desktop rendering normally (screenshot), NVENC pipeline at a solid 60 fps with
`pipeline_dropped=0`, `frame_size_bytes` varying 15.6k–17.1k (i.e. real changing
content, not a black frame), `ws_write_ms` p99 ≤ 1 ms (no backpressure), exactly one
subscribed client, and an API-side proxy socket open since 11:28:57Z that was never
closed. **The bytes reached Safari and Safari did not paint them.**

## Root cause (established by code reading — see design.md for detail)

The Safari-only WebGL2 render path (`webgl-video-renderer.ts`, taken because
`isAppleWebKit()` is true) has **no WebGL context-loss handling whatsoever**. When the
context is lost — routine on macOS Safari after a tab is backgrounded or the GPU
process recycles — `texImage2D` and `drawArrays` silently become no-ops. Decode keeps
succeeding, `receivedFirstKeyframe` stays true, `videoStarted` already fired so the
connecting overlay is hidden, and the stats panel stays green. The user gets an
indefinitely black rectangle and no signal at all.

Two additional defects in the same code make black screens reachable *deterministically*,
independent of a GPU-driver-initiated loss (Findings 2 and 3 in design.md).

Hypothesis A (retry budget) is a real, separate defect but is **not** the cause of this
incident: exhausting the budget sets `gaveUp` and renders an error string, and Luke saw
black, not an error.

## User Stories

### US-1 — The stream must survive WebGL context loss
**As a** user watching a desktop stream in Safari,
**I want** the renderer to recover automatically when the browser drops the GPU context,
**so that** backgrounding a tab or a GPU process recycle doesn't permanently kill my video.

Acceptance criteria:
- [ ] `WebGLVideoRenderer` registers `webglcontextlost` (calling `preventDefault()`, which is
      what makes the browser attempt a restore) and `webglcontextrestored` handlers.
- [ ] On `webglcontextrestored` the program, VAO, texture and uniform state are fully
      reallocated and rendering resumes without a reconnect and without a page refresh.
- [ ] While the context is lost, `draw()` is a no-op that does not throw and does not
      silently consume frames as if they were painted.
- [ ] Forcing loss with the `WEBGL_lose_context` extension mid-stream, then restoring,
      returns live video. Verified in a real browser against a real desktop session.

### US-2 — Re-attaching the canvas must never produce a dead renderer
**As a** user toggling between screenshot and video quality modes, or reconnecting,
**I want** the video to come back,
**so that** a mode switch or restart isn't a one-way trip to a black screen.

Acceptance criteria:
- [ ] Calling `setCanvas()` twice with the same `<canvas>` element yields a working
      renderer. (Today the second call force-loses the context in `dispose()` and then
      re-acquires that same dead context — see design.md Finding 2.)
- [ ] After `close()` followed by `reconnect()` on the same stream object, frames are
      painted rather than silently dropped at the `!videoRenderer && !canvasCtx` guard.
- [ ] The screenshot → video quality-mode switch (`DesktopStreamViewer.tsx:1956`) restores
      live video on WebKit.

### US-3 — A black screen must never be silent
**As a** user whose stream has failed to paint,
**I want** a clear message and a working Retry button,
**so that** I don't stare at a black rectangle for 7 minutes.

Acceptance criteria:
- [ ] A paint watchdog surfaces an error when the socket is open and frames are decoding
      but nothing has been painted for N seconds. `VIDEO_START_TIMEOUT_MS` does not cover
      this — it is cleared as soon as `videoStarted` fires, which happens in the decode
      path *before* any pixel reaches the canvas.
- [ ] Lost-and-unrecovered WebGL context, and `gaveUp`, both produce a visible message
      with a Retry control — never a bare black canvas.
- [ ] Retry from each of those states actually recovers a healthy stream (test the
      *next* operation, not just the state change).

### US-4 — The retry budget must not latch on a healthy desktop
**As a** user on a flaky network,
**I want** transient disconnects not to permanently disable my stream,
**so that** six reconnects in 20 minutes doesn't burn the budget to zero.

Acceptance criteria:
- [ ] Root-cause and fix the refund path so a connection that genuinely delivers video
      refunds its retry (the `!receivedFirstKeyframe` block is currently the only refund site,
      and three code paths reset `receivedFirstKeyframe = false`).
- [ ] `scheduleReconnect()`'s `reconnectWhenVisible` gate cannot defer a reconnect
      indefinitely if `visibilitychange` never fires.
- [ ] Confirm the **Restart** button routes through `WebSocketStream.reconnect()` (the only
      thing that clears `gaveUp`), and fix it if it does not.

### US-5 — No hacks, no dual code paths
Per CLAUDE.md:
- [ ] Fix at the source. No "if it looks broken, reconnect anyway" band-aids.
- [ ] The WebKit branch's `catch` currently falls back to `canvas.getContext("2d")`, which
      **returns null** on a canvas that already holds a WebGL context. That fallback is both
      a forbidden dual code path and non-functional — remove it and surface a real error.
- [ ] Delete any code the fix makes dead.

### US-6 — Verified end-to-end in the inner Helix
- [ ] All work happens against `http://localhost:8080`. **meta.helix.ml is not touched.**
- [ ] Reproduce the black screen, instrument it, and report actual observed values for:
      `videoDecoder` non-null/configured, `receivedFirstKeyframe`, `reconnectAttempts`/`gaveUp`,
      `gl.isContextLost()`, and whether `VideoFrame`s reach the renderer.
- [ ] Before/after shown in a real browser against a real desktop session — not a DOM
      harness, not a unit test.
- [ ] Safari cannot be run in the sandbox. Force the WebKit branch by stubbing
      `isAppleWebKit()` to true under Chromium and drive context loss explicitly with
      `WEBGL_lose_context`. State plainly in the write-up that real-Safari confirmation is
      outstanding and why.

### US-7 — Write-up and PR
- [ ] Findings written to `design/2026-08-04-black-video-stream-regression.md`.
- [ ] PR opened against `helixml/helix`, referenced by full URL.

## Open Questions

1. **Which defect fired on prod?** Findings 1, 2 and 3 all produce an identical
   black-screen-with-no-error signature. Finding 1 (driver-initiated context loss) best fits
   "Safari-only, intermittent, after a Restart". Findings 2 and 3 need a `setCanvas()` or
   `close()`→`reconnect()` on the same stream object, which depends on what the Restart button
   does — worth confirming during repro, but **all three should be fixed regardless**, since
   each independently causes the reported symptom.
2. **Restart button behaviour.** Does Restart remount `DesktopStreamViewer` (fresh canvas +
   fresh `WebSocketStream`) or reuse the existing objects? This determines whether Findings 2/3
   are reachable via Restart. I could not find a restart handler in `DesktopStreamViewer.tsx` —
   it appears to live in a parent component. Assumption: it reuses the stream object.
3. **Watchdog threshold.** Assumed **5 seconds** without a painted frame while the socket is
   open and frames are decoding. Long enough to survive an idle desktop at the 10 fps keepalive,
   short enough to be useful. Confirm this is acceptable.
4. **Recovery vs. notify on context loss.** Assumed the renderer recovers silently on
   `webglcontextrestored` and only surfaces a message if restore does not arrive within the
   watchdog window. Alternative: always tell the user the stream glitched. Silent recovery is
   the better UX and is assumed here.
5. **Retry-budget scope.** US-4 is a genuine defect but is *not* what caused this incident.
   Confirm it should be fixed in the same PR rather than split out.
6. **`isAppleWebKit()` stub mechanism.** Assumed a temporary local edit for testing, reverted
   before commit. If a permanent query-param override (e.g. `?forcewebgl=1`) is wanted for
   future debugging, say so — that is a deliberate second code path and needs sign-off given
   the no-dual-paths rule.
