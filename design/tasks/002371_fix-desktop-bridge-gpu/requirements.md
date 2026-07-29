# Requirements: Fix desktop-bridge GPU and FD Leak in GStreamer Pipeline Teardown

## Background

On 2026-07-28/29 a single `desktop-bridge` process on `meta/node01` accumulated
**9324 MiB of GPU memory** and **1564 open `/dev/nvidia0` fds** over 45 hours on a
shared 16 GB RTX 2000 Ada. 324 GStreamer pipelines were created and destroyed in
that window:

- (9324 − 252) MiB ÷ 324 = **28.0 MiB leaked per pipeline**
- (1564 − 52) fds ÷ 324 = **4.66 fds leaked per pipeline**

Go-side accounting is perfectly balanced (213 "cleaned up (was never started)" +
111 "stopped" = 324, `activePipelineCount` = 0, zero "pipeline stuck"). The leak
is entirely below the Go layer: the C-side `GstPipeline`/`GstElement` objects are
never finalized.

The exhausted GPU then broke a *different* user's desktop: `nvh264enc` could not
register, `selectEncoder()` silently fell back to `openh264`, which flipped
`isNvidiaGnome` false → `isAmdGnome` true, taking the `pipewiresrc
always-copy=true` branch on NVIDIA hardware. That path delivered ~0 frames
(`no_frames_for=30000`). The user saw a frozen stream, no Zed, and broken mouse
buttons.

Counts reproduced from `attachments/leaker-digest.log`:

| signal | count |
|---|---|
| `stream WebSocket connected` | 2383 |
| `failed to read init message` | 1708 |
| `stream init received` | 674 |
| `[GST_PIPELINE] Starting pipeline` | 324 |
| `Circuit breaker OPEN` | 209 |
| `using NVIDIA NVENC encoder` | 674 (always nvenc on the leaker) |
| close code 1005 / 1006 | 600 / 652 |

So ~3.5 WebSocket connections per successful `init`, and ~2 inits per pipeline
instantiation (`SharedVideoSource` dedupes the rest).

## User Stories

### US-1 — Pipeline teardown must free GPU resources at a known point

**As** a Helix user sharing a GPU host with other tenants,
**I want** every desktop stream pipeline to release its CUDA context, NVENC
session and DMA-BUF fds the moment it is stopped,
**so that** a long-lived session cannot starve the GPU for everyone else.

Acceptance criteria:

- [ ] AC-1.1 The actual refcount / finalization behaviour is **measured** before
      the fix is written (GStreamer leaks tracer + logged refcount), and the
      finding is recorded in the task's design notes. The existing comment in
      `Stop()` is not treated as ground truth.
- [ ] AC-1.2 After `GstPipeline.Stop()` returns, every go-gst object the pipeline
      owns (pipeline, appsink element, bus, forced clock, in-flight samples) has
      been explicitly released; nothing is left to a GC finalizer.
- [ ] AC-1.3 No GC finalizer can double-unref a released object (finalizers are
      disarmed before the explicit `Unref`, mirroring the existing correct
      handling of bus messages in `watchBus`).
- [ ] AC-1.4 An unconditional `runtime.GC()` after teardown is **not** part of the
      fix.
- [ ] AC-1.5 Over 20 create/destroy cycles the process's `/dev/nvidia0` fd count
      and its GPU memory (`nvidia-smi --query-compute-apps`) are flat within a
      small constant after warm-up.
- [ ] AC-1.6 Both cycle shapes are covered: create → `Stop()` without ever
      starting (the 213-case), and create → `Start()` → frames → `Stop()` (the
      111-case). This also resolves whether the 28 MiB/4.7 fd figures attribute
      to all 324 pipelines or only the 111 that reached PLAYING.

### US-2 — A regression test that fails today and passes after the fix

**As** a Helix maintainer,
**I want** an automated test that detects this class of leak,
**so that** it cannot silently return.

Acceptance criteria:

- [ ] AC-2.1 A Go test in `api/pkg/desktop` (build-tagged `cgo && linux`) creates
      and destroys N ≥ 20 GPU pipelines and asserts the `/proc/self/fd`
      `/dev/nvidia0` count is flat.
- [ ] AC-2.2 It also asserts GPU MiB attributed to the test process is flat.
- [ ] AC-2.3 It is demonstrated to **fail on the current code** and pass after
      the fix; both runs are captured in the PR description.
- [ ] AC-2.4 It skips cleanly (not fails) when `/dev/nvidia0` or `nvh264enc` is
      absent, so non-GPU CI hosts are unaffected.

### US-3 — Reconnect storm is bounded

**As** a user who leaves a finished task's tab open,
**I want** the desktop stream client to stop hammering the backend,
**so that** an idle tab cannot drive thousands of pipeline create/destroy cycles.

Acceptance criteria:

- [ ] AC-3.1 The retry budget is only reset by a **working stream** (first video
      frame / `videoStarted`), never by a bare WebSocket `open` or
      `connectionComplete`. Today the 2 s `connectionStabilityTimer` and
      `resetRetryState()` on `connectionComplete` make the existing backoff and
      the `maxReconnectAttempts=10` give-up unreachable.
- [ ] AC-3.2 Backoff is exponential with jitter and a cap; after the budget is
      exhausted the client enters a terminal state and stops connecting.
- [ ] AC-3.3 The terminal state shows a clear, specific error in the UI plus an
      explicit user-driven Retry — not a silent loop and not "Reconnecting…"
      forever.
- [ ] AC-3.4 Reconnection is suspended while the tab is hidden
      (`document.visibilityState !== 'visible'`) and resumes on visibility.
- [ ] AC-3.5 The ~3.5 WebSocket connections per `init` are explained and reduced
      to 1: connections that never send `init` are eliminated (2383 → ~674 in the
      equivalent trace).
- [ ] AC-3.6 Verified end-to-end in the inner Helix browser: connect, kill the
      stream, observe capped backoff, observe give-up UI, click Retry, observe
      recovery.

### US-4 — Encoder/GPU detection is correct and failures are loud

**As** an on-call engineer triaging a broken desktop,
**I want** logs that name the real hardware and a loud failure instead of a
zero-frame path,
**so that** triage is not actively misled.

Acceptance criteria:

- [ ] AC-4.1 NVIDIA hardware is detected independently of encoder plugin
      availability (device node / DRM vendor id), not via
      `checkGstElement("nvh264enc")`.
- [ ] AC-4.2 NVIDIA hardware with NVENC unavailable never takes the
      `isAmdGnome` / `pipewiresrc always-copy=true` branch.
- [ ] AC-4.3 That condition logs at ERROR naming the real cause and surfaces a
      specific error to the client; the misleading `GNOME + AMD/Intel detected`
      line can no longer appear on an NVIDIA host.
- [ ] AC-4.4 Unit tests cover: NVIDIA+NVENC, NVIDIA-without-NVENC,
      AMD/Intel, Sway, macOS virtio-gpu.

### US-5 — Resource guard

**As** an operator of a shared GPU host,
**I want** desktop-bridge to notice it is hoarding GPU resources,
**so that** a leak is visible in hour one rather than hour 45.

Acceptance criteria:

- [ ] AC-5.1 A periodic self-check logs at ERROR when the process's
      `/dev/nvidia0` fd count or GPU MiB crosses a warn threshold.
- [ ] AC-5.2 Above a hard ceiling, new pipeline instantiation is refused with a
      clear error to the client rather than deepening the exhaustion.
- [ ] AC-5.3 The check is cheap (reads `/proc/self/fd`, no per-frame cost).

### US-6 — The circuit breaker is a failure throttle, not a leak governor

Acceptance criteria:

- [ ] AC-6.1 After the leak fix, repeated failures for a node reach a terminal
      state that surfaces an error to the client instead of permitting one
      leaking instantiation per 30 s cooldown forever.

## Out of Scope

- The mouse-button D-Bus `"Invalid button event"` failure and the `ghostty`
  OpenGL segfault. Both were downstream symptoms of GPU exhaustion on the victim
  container; they should resolve once the GPU is not exhausted. If they persist,
  they are separate tasks.
- Upgrading the `go-gst` dependency.

## Definition of Done

1. Reproduction demonstrated first: nvidia0 fd count ratchets up ~4–5 per
   create/destroy cycle on unmodified code, with the raw numbers captured.
2. AC-1.x and AC-2.x met — leak fixed, regression test red-then-green.
3. AC-3.x, AC-4.x, AC-5.x, AC-6.x met.
4. **End-to-end in the inner Helix**: a desktop stream watched in the browser
   with frames flowing at the expected FPS (static ≈ 10, terminal 15–35, vkcube
   55–60 per `CLAUDE.md`), with `nvidia-smi` and the nvidia0 fd count flat across
   repeated connect/disconnect cycles. A passing unit test alone is not
   acceptable evidence.

## Environment Notes (verified in this sandbox)

The agent sandbox for this task is itself a GPU desktop container, so everything
above is directly testable here:

- `/dev/nvidia0` present; `nvidia-smi` reports an RTX 2000 Ada.
- GStreamer 1.26.6; `gst-inspect-1.0 nvh264enc` resolves (NVENC H.264, CUDA mode).
- A live `desktop-bridge` runs at PID 945 (`/usr/local/bin/desktop-bridge`),
  supervised by `start-desktop-bridge.sh`.
- With no client streaming, `ls -l /proc/945/fd | grep -c nvidia0` = **0** — a
  clean baseline for the ratchet measurement.
- Inner Helix at `http://localhost:8080` for the browser E2E.

## Open Questions

1. **CI coverage of the regression test.** The test needs an NVIDIA GPU. Should
   it run on a GPU-equipped Drone runner, or is a locally-run, PR-attached
   red/green result plus an auto-skip on non-GPU hosts sufficient? Assumed the
   latter unless told otherwise.
2. **NVIDIA-without-NVENC behaviour.** The brief offers "fail loudly **or** use a
   software path that actually delivers frames". The repo rule is NO FALLBACKS,
   so the plan assumes **fail loudly** with a specific client-facing error. If
   you want a working software path instead (plain `pipewiresrc` without
   `always-copy=true` → `videoconvert` → `x264enc`), say so — it is a different
   design.
3. **Guard thresholds.** Assumed warn at >200 `/dev/nvidia0` fds or >1500 MiB,
   hard refuse at >400 fds or >3000 MiB, checked every 30 s. A healthy bridge
   measured 52 fds / 252 MiB. Confirm or adjust.
4. **`gst_app_sink_set_callbacks` teardown.** The plan clears the appsink
   callbacks during `Stop()` to release go-gst's global `gopointer` handle.
   go-gst v1.4.0 gives no documented nil-safe form, so this may need a tiny
   upstream-shaped local helper or a cgo call. Acceptable, or should the fix stay
   strictly within the public go-gst API even if that leaves the handle pinned?
5. **Full 82 MB log.** The digest answered every question so far. The full
   `03-leaker-full.log` / `11-proc-maps.txt` on the meta host are not needed
   unless the leaks-tracer run contradicts the analysis — confirm they will stay
   available for a while in case they are.
6. **Frontend scope.** `WebSocketStream` and `DesktopStreamViewer` both own
   reconnect logic and race each other. Consolidating to a single owner is the
   clean fix but touches a 5.5k-line component. Confirm that refactor is in
   scope rather than a narrower patch to the reset conditions.
