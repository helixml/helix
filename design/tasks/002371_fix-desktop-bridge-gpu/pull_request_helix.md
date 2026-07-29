# Fix desktop-bridge GPU leak: ~28 MiB and ~14 nvidia fds per GStreamer pipeline

## Summary

One `desktop-bridge` accumulated **9.3 GB of GPU memory and 1564 open
`/dev/nvidia0` fds** over 45 hours on a shared 16 GB RTX 2000 Ada, exhausted the
card, and broke a different user's desktop. This fixes the leak and the three
things that made it worse or harder to diagnose.

Measured on a live `desktop-bridge`, driving real stream connect/disconnect
cycles (each cycle creates and destroys a GStreamer pipeline), counting
`/dev/nvidia0` entries in `/proc/<pid>/fd`:

| cycle | before | after |
|---|---|---|
| 1 | 24 | 10 |
| 2 | 38 | 10 |
| 3 | 52 | 10 |
| 4 | 66 | 10 |
| 5 | 80 | 10 |
| 6 | 94 | 10 |
| 7 | 108 | 10 |
| 8 | 122 | 10 |

**+14 per cycle, unbounded → flat.** The GStreamer leaks tracer goes from a
per-cycle set of survivors to zero live objects. Full write-up in
`design/2026-07-29-desktop-bridge-gpu-leak.md`.

## The leak was two independent bugs

**1. Rust plugin — this was the entire GPU leak.**
`desktop/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs` had
`std::mem::forget(ctx)` on what its comment assumed was a rare race. It isn't:
`gst_cuda_ensure_element_context()` runs a context query that GStreamer answers by
calling our own `set_context()` synchronously on the same thread, so that branch
is taken on **every pipeline start**. `ctx` aliases a `GstCudaContext` it doesn't
own (unreffing it really would double-free) but it *does* own the `GstCudaStream`
created alongside it — and a stream holds a reference on its context, so every
pipeline stranded a whole CUDA context. New `CUDAContext::release_stream_only()`
drops the stream and forgets only the borrowed pointer.

**2. Go side — leaked GStreamer objects plus a latent crash.**
`api/pkg/desktop/gst_pipeline.go` relied on GC finalizers for four transfer-full
references (`PullSample`, `Sample.GetBuffer`, `GetElementByName`,
`GetPipelineBus`). It also unreffed the pipeline while leaving its finalizer
armed — `gst_parse_launch` returns a *floating* ref that go-glib ref-sinks, so
that `Unref` freed the object and the next GC double-freed it. The new leak test
reproduced that as `g_object_unref: assertion 'G_IS_OBJECT (object)' failed` +
SIGSEGV in `runtime.runFinalizers`; `start-desktop-bridge.sh`'s restart-loop
comment ("can crash ... segfault during WebSocket reconnection") was this.

The Go fix alone was deployed and measured first: object leaks gone, crash gone,
**fd count unchanged**. Only the tracer dump pointed at the plugin.

## Changes

- `api/pkg/desktop/gst_pipeline.go` — explicit, ordered release of every go-gst
  object in `Stop()`; `releaseGObject`/`releaseSample`/`releaseBuffer` helpers
  that disarm the finalizer before unreffing; bus flushed and released; per-frame
  sample and buffer released in `onNewSample`; probe element released in
  `diagnoseGPUEncoderFailure`.
- `desktop/gst-pipewire-zerocopy`, `desktop/wayland-display-core` — the CUDA
  stream leak.
- `scripts/build-zerocopy-plugin.sh` — Rust 1.85.0 → 1.87.0 to match
  `Dockerfile.ubuntu-helix`. The script could not build the plugin at all.
- `api/pkg/desktop/gpu_vendor.go` (new) — GPU vendor from `/dev/nvidia0` and the
  DRM PCI vendor id, not from encoder plugin availability. An exhausted GPU used
  to make `checkGstElement("nvh264enc")` go false, reclassifying NVIDIA hardware
  as AMD/Intel and taking a branch that delivers zero frames — while logging
  `GNOME + AMD/Intel detected` on an RTX 2000 Ada. NVIDIA without NVENC now fails
  loudly with a specific client-facing error; explicit `HELIX_ENCODER` still wins.
- `api/pkg/desktop/gpu_guard.go` (new) — 30 s self-check on this process's
  `/dev/nvidia0` fd count; warn at 200, refuse new pipelines at 400 (healthy is
  ~52). Nothing detected the incident for 45 hours.
- `api/pkg/desktop/shared_video_source.go` — circuit breaker latches after 10
  trips instead of permitting one leaking instantiation per cooldown forever (it
  opened 209 times during the incident); cleared only by an explicit user retry.
- `frontend/.../websocket-stream.ts`, `DesktopStreamViewer.tsx` — the retry
  budget now resets only on `videoStarted`, not on a socket that merely opened. A
  2 s "stabilized" timer and `resetRetryState()` on `connectionComplete` were
  zeroing it every cycle, which is why the existing backoff and 10-attempt
  give-up never fired across 2383 connections. Give-up is now terminal with a
  real error plus Retry, and reconnection is suspended while the tab is hidden.

## Tests

- `api/pkg/desktop/gst_pipeline_leak_test.go` (new) — N × pipeline
  create/destroy, asserting `/dev/nvidia0` fd count and GPU MiB are flat, in both
  the "never started" and "started + frames" shapes. Skips cleanly without a GPU.
  Note it does **not** reproduce the CUDA leak: a synthetic
  `videotestsrc ! cudaupload ! nvh264enc` pipeline is flat even on broken code —
  that one needs the real `pipewirezerocopysrc` path, so the live-cycle
  measurement above is its regression evidence. It does fail on unmodified code,
  via the finalizer double-unref crash.
- `api/pkg/desktop/ws_stream_gpu_detect_test.go` (new) — NVIDIA+NVENC,
  NVIDIA-without-NVENC, AMD, AMD-with-nvenc-element-present, Sway, macOS
  virtio-gpu, and explicit-override.

## Verified end-to-end

Deployed the fixed binary and plugin into a live desktop container and watched
the stream in the browser: **55–60 FPS**, fd count flat across repeated
create/destroy cycles, no crashes. Not a counter in a unit test — actual frames.
