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
- [x] Register `test@helix.ml` / `helixtest`, complete onboarding (org → project), create a spec task so a real desktop session with a live video stream exists
- [x] Open the desktop viewer and confirm a healthy baseline stream (video painting, input working)
- [x] Temporarily stub `isAppleWebKit()` to `true` so the WebGL2 renderer runs under Chromium (local-only edit; revert before commit) — this is the substitute for Safari, and it is sufficient to drive every finding below

## Instrument before guessing

- [x] Add diagnostic logging for `videoDecoder?.state`, `receivedFirstKeyframe`, `reconnectAttempts`, `gaveUp`, `gl.isContextLost()`, and a painted-frame counter at the `draw()` call site (`websocket-stream.ts:1114`)
- [x] **ROOT CAUSE FOUND — not the planning hypothesis.** Reproduced a live freeze: `targetFrames=152`, `prerolling=true`, `queueLen=30` pinned at `MAX_QUEUE`, 0 frames presented in 6s, `framesDropped` +44/s, `glContextLost=false`, `reconnectAttempts=0`, `gaveUp=false`, decoder `configured`, rAF at 53Hz. `PlayoutScheduler` deadlock — see design.md "ROOT CAUSE FOUND"
- [x] Record the actual observed values in the design write-up

## Fix the playout scheduler deadlock (`playout-scheduler.ts`) — PRIMARY FIX

- [x] Derive source cadence from PTS deltas, not median socket arrival spacing (bursts collapse arrival spacing and inflated the depth target ~20x)
- [x] Cap the depth target below `MAX_QUEUE` so the preroll-satisfied test is structurally always reachable
- [x] End preroll at `q.length >= target` (was `>`, unreachable at the queue cap)
- [x] Reset PTS cadence tracking in `clear()` on discontinuity
- [x] Verify live: 56.4 fps presented, `targetFrames=1`, depth 50ms, `queueLen=4` (was 0 fps / 152 / 2508ms / 30)

## Remaining hypothesis-B defects (real, but NOT the incident cause — still worth fixing)

- [x] Reproduce Finding 2: toggle quality mode video → screenshot → video (second `setCanvas()` on the same element); record whether both `videoRenderer` and `canvasCtx` end up null
- [x] Reproduce Finding 3: `close()` nulled canvas/renderer and `reconnect()` never restored them — confirmed by code + the 0-fps result; note the console-driven close+reconnect race is masked by server-side `client_id` dedup (`Close received after close`)
- [x] Confirm whether the Restart button routes through `WebSocketStream.reconnect()` or remounts the viewer (resolves requirements Open Question 2)

## Fix the renderer (`webgl-video-renderer.ts`)

- [x] Extract constructor GL setup (program, VAO, texture, uniforms) into a private `initGL()`
- [x] Add a `webglcontextlost` listener that calls `preventDefault()` and marks the renderer unavailable
- [x] Add a `webglcontextrestored` listener that reruns `initGL()` and resumes rendering without a reconnect or page refresh
- [x] Make `draw()` a safe no-op while `gl.isContextLost()` is true, and report it rather than silently consuming the frame
- [x] Remove the `WEBGL_lose_context.loseContext()` call from `dispose()` — it permanently bricks a canvas that will be reused
- [x] Expose `isContextLost()` for the stream client and the watchdog

## Fix renderer lifetime (`websocket-stream.ts`)

- [x] Make `setCanvas()` reuse the existing live renderer when called again with the same canvas element, instead of dispose-then-reacquire
- [x] Delete the WebKit `catch` → `canvas.getContext("2d")` fallback (forbidden dual code path, and returns null after WebGL2 binding); surface a real error instead
- [x] Ensure `connect()`/`reconnect()` guarantee a live render target so `close()` → `reconnect()` cannot leave `videoRenderer` and `canvasCtx` both null
- [x] Delete any code made dead by the above

## Never fail silently

- [x] Track `lastFramePaintedAt` at the actual `draw()` call site (not at decode / `videoStarted`)
- [x] Add a paint watchdog: socket open + frames decoding + nothing painted for 5 s → dispatch a new info event
- [x] Add the new event variant(s) to `websocket-stream.types.ts`
- [x] Render the new error state in `DesktopStreamViewer.tsx` with a working Retry, reusing the existing `gaveUp` error surface (`:1042-1050`) so there is one error path
- [x] Verify unrecovered context loss and `gaveUp` both show a message rather than a bare black canvas

## Retry budget (Finding 4 — separate defect, same PR)

- [x] Refund the retry budget on evidence video is genuinely flowing, decoupled from `receivedFirstKeyframe` (which three unrelated paths clear)
- [x] Bound the `reconnectWhenVisible` deferral in `scheduleReconnect()` so a missing `visibilitychange` cannot defer a reconnect forever
- [x] Confirm the Restart button clears `gaveUp`; fix the routing if it does not

## Verify end-to-end in a real browser

- [x] F1 after fix: force context loss mid-stream → video auto-recovers on restore
- [x] F2 after fix: second `setCanvas()` on the same element keeps video live (59.3 → 59.0 paint fps); driven programmatically, not via the quality-mode UI control
- [x] F3 after fix: renderer + canvas now survive `close()`, but a synchronous console `close()`+`reconnect()` still yields 0 fps because the server dedups the reused `client_id`. NOT a viewer path; the real RECONNECT button path is verified separately
- [x] Restart the desktop container under an open viewer, repeatedly → viewer recovers or shows a real error
- [x] Force ≥10 reconnect cycles → client does not latch permanently once the desktop is healthy
- [x] Background the tab, restart the desktop, foreground it → stream resumes
- [x] Exercise the next operation after each recovery (interact with the desktop, confirm input works) — not just the state change
- [x] Capture before/after screenshots against a real desktop session
- [x] Revert the `isAppleWebKit()` stub and any temporary diagnostic logging not worth keeping
- [x] `cd frontend && yarn build`

## Ship

- [x] Write up findings and observed values in `design/2026-08-04-black-video-stream-regression.md`, explicitly stating that real-Safari confirmation is outstanding and why (no Safari in the sandbox)
- [x] Commit with conventional-commit messages and push the feature branch (platform opens the PR)
- [~] Check CI yourself (`gh pr checks` / Drone MCP tools); fix and re-check until green

## Added during implementation

- [x] Fix visibility-deferred reconnect: the heartbeat-based self-heal was dead code (`stopHeartbeat()` runs in `onClose`); replaced with a poll that survives disconnection. Verified it failed before and works after
- [x] Track `framesPainted` separately from `framesDecoded` — the latter counts decode attempts and climbed at 60/s while nothing was on screen
- [x] Remount the `<canvas>` when its GPU context is unrecoverable (only way to get a working context back)
- [x] Write PR description (`pull_request_helix.md`)
