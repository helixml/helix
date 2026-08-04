# Implementation Tasks: Fix Black Desktop Video Stream from WebGL Context Loss

## Testing constraint — read first

**There is no Safari in this sandbox.** The Linux desktop has Chrome/Chromium only,
driven via the `mcp__chrome-devtools__*` MCP tools. Do not treat this as a blocker and
do not stall waiting for a Safari result.

The bug is reachable without Safari. `isAppleWebKit()` is a one-line vendor-string check,
so stubbing it to `true` makes Chromium take the exact same WebGL2 renderer path Safari
takes, and `WEBGL_lose_context` reproduces context loss deterministically on any engine.
That verifies the logic, the fix, and the recovery. What it does *not* verify is WebKit's
own context-loss timing and frequency — say so plainly in the write-up rather than
claiming Safari coverage.

Work autonomously throughout. Every open question in requirements.md has a documented
default assumption — take it, note the choice in the write-up, and keep going.

## Setup and reproduction (inner Helix only — never meta.helix.ml)

- [x] Confirm the inner Helix stack is up at `http://localhost:8080` (poll for several minutes; `helix-api-1`, `helix-frontend-1`, `helix-postgres-1` all `Up` and 8080 returning 200)
- [~] Register `test@helix.ml` / `helixtest`, complete onboarding (org → project), create a spec task so a real desktop session with a live video stream exists
- [ ] Open the desktop viewer and confirm a healthy baseline stream (video painting, input working)
- [ ] Temporarily stub `isAppleWebKit()` to `true` so the WebGL2 renderer runs under Chromium (local-only edit; revert before commit) — this is the substitute for Safari, and it is sufficient to drive every finding below

## Instrument before guessing

- [ ] Add diagnostic logging for `videoDecoder?.state`, `receivedFirstKeyframe`, `reconnectAttempts`, `gaveUp`, `gl.isContextLost()`, and a painted-frame counter at the `draw()` call site (`websocket-stream.ts:1114`)
- [ ] Reproduce Finding 1: call `gl.getExtension('WEBGL_lose_context').loseContext()` mid-stream; record observed values and screenshot the black canvas
- [ ] Reproduce Finding 2: toggle quality mode video → screenshot → video (second `setCanvas()` on the same element); record whether both `videoRenderer` and `canvasCtx` end up null
- [ ] Reproduce Finding 3: drive `close()` then `reconnect()` on the same stream object; confirm frames are dropped at the `websocket-stream.ts:1061` guard
- [ ] Confirm whether the Restart button routes through `WebSocketStream.reconnect()` or remounts the viewer (resolves requirements Open Question 2)
- [ ] Record the actual observed values for all of the above in the design write-up

## Fix the renderer (`webgl-video-renderer.ts`)

- [ ] Extract constructor GL setup (program, VAO, texture, uniforms) into a private `initGL()`
- [ ] Add a `webglcontextlost` listener that calls `preventDefault()` and marks the renderer unavailable
- [ ] Add a `webglcontextrestored` listener that reruns `initGL()` and resumes rendering without a reconnect or page refresh
- [ ] Make `draw()` a safe no-op while `gl.isContextLost()` is true, and report it rather than silently consuming the frame
- [ ] Remove the `WEBGL_lose_context.loseContext()` call from `dispose()` — it permanently bricks a canvas that will be reused
- [ ] Expose `isContextLost()` for the stream client and the watchdog

## Fix renderer lifetime (`websocket-stream.ts`)

- [ ] Make `setCanvas()` reuse the existing live renderer when called again with the same canvas element, instead of dispose-then-reacquire
- [ ] Delete the WebKit `catch` → `canvas.getContext("2d")` fallback (forbidden dual code path, and returns null after WebGL2 binding); surface a real error instead
- [ ] Ensure `connect()`/`reconnect()` guarantee a live render target so `close()` → `reconnect()` cannot leave `videoRenderer` and `canvasCtx` both null
- [ ] Delete any code made dead by the above

## Never fail silently

- [ ] Track `lastFramePaintedAt` at the actual `draw()` call site (not at decode / `videoStarted`)
- [ ] Add a paint watchdog: socket open + frames decoding + nothing painted for 5 s → dispatch a new info event
- [ ] Add the new event variant(s) to `websocket-stream.types.ts`
- [ ] Render the new error state in `DesktopStreamViewer.tsx` with a working Retry, reusing the existing `gaveUp` error surface (`:1042-1050`) so there is one error path
- [ ] Verify unrecovered context loss and `gaveUp` both show a message rather than a bare black canvas

## Retry budget (Finding 4 — separate defect, same PR)

- [ ] Refund the retry budget on evidence video is genuinely flowing, decoupled from `receivedFirstKeyframe` (which three unrelated paths clear)
- [ ] Bound the `reconnectWhenVisible` deferral in `scheduleReconnect()` so a missing `visibilitychange` cannot defer a reconnect forever
- [ ] Confirm the Restart button clears `gaveUp`; fix the routing if it does not

## Verify end-to-end in a real browser

- [ ] F1 after fix: force context loss mid-stream → video auto-recovers on restore
- [ ] F2 after fix: quality-mode switch screenshot → video restores live video
- [ ] F3 after fix: `close()` → `reconnect()` paints frames
- [ ] Restart the desktop container under an open viewer, repeatedly → viewer recovers or shows a real error
- [ ] Force ≥10 reconnect cycles → client does not latch permanently once the desktop is healthy
- [ ] Background the tab, restart the desktop, foreground it → stream resumes
- [ ] Exercise the next operation after each recovery (interact with the desktop, confirm input works) — not just the state change
- [ ] Capture before/after screenshots against a real desktop session
- [ ] Revert the `isAppleWebKit()` stub and any temporary diagnostic logging not worth keeping
- [ ] `cd frontend && yarn build`

## Ship

- [ ] Write up findings and observed values in `design/2026-08-04-black-video-stream-regression.md`, explicitly stating that real-Safari confirmation is outstanding and why (no Safari in the sandbox)
- [ ] Commit with conventional-commit messages, open a PR against `helixml/helix`, and report the full PR URL
- [ ] Check CI yourself (`gh pr checks` / Drone MCP tools); fix and re-check until green
