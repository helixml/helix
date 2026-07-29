# Implementation Tasks: Fix desktop-bridge GPU and FD Leak in GStreamer Pipeline Teardown

## Phase 1 — Reproduce and verify the root cause (before any fix)

- [x] Baseline the idle `desktop-bridge` in this sandbox: `ls -l /proc/$(pgrep -f /usr/local/bin/desktop-bridge)/fd | grep -c nvidia0` and `nvidia-smi --query-compute-apps=pid,used_memory --format=csv`
- [x] Drive N stream connect/disconnect cycles (browser at `localhost:8080`, or `helix spectask stream`/`benchmark`) and record the per-cycle `/dev/nvidia0` fd delta — confirm the ~4–5 fds/cycle ratchet on unmodified code
- [x] Re-run desktop-bridge with `GST_TRACERS="leaks(GstPipeline,GstElement,GstBus,GstSample,GstBuffer)"` and `GST_DEBUG=GST_TRACER:7`; after N cycles send `SIGUSR1`/`SIGUSR2` and capture the live-object dump
- [x] Temporarily log `GST_OBJECT_REFCOUNT(pipeline)` immediately before the `Unref()` in `GstPipeline.Stop()` and record the value
- [~] Confirm or correct the design's holder set (appsink `TransferFull` ref, `gopointer` callback pin, undrained/unreffed bus, forced clock, per-frame samples); write the finding into `design/2026-07-29-desktop-bridge-gpu-leak.md` in the helix repo

## Phase 2 — Regression test (red first)

- [x] Add `api/pkg/desktop/gst_pipeline_leak_test.go` with build tag `cgo && linux`, skipping when `/dev/nvidia0` is absent or `nvh264enc` does not register
- [x] Implement the fd/GPU measurement helpers: count `/dev/nvidia0` symlinks in `/proc/self/fd`; read own-pid GPU MiB from `nvidia-smi --query-compute-apps`
- [x] Sub-test A: 20 × (`NewGstPipeline` → `Stop()` without `Start()`) — the "never started" shape; assert flat after warm-up
- [x] Sub-test B: 20 × (`NewGstPipeline` → `Start()` → consume a few frames → `Stop()`) — the "started" shape; assert flat after warm-up
- [x] Run against unmodified code and capture the failing output for the PR

## Phase 3 — Fix the leak

- [x] Add a `releaseGObject(*glib.Object)` helper that disarms the go-glib finalizer then unrefs, with a comment explaining that the finalizer lives on the embedded `*glib.Object`
- [x] Keep the appsink element wrapper on `GstPipeline` so its `TransferFull` ref can be released explicitly
- [x] Clear the appsink callbacks in `Stop()` so `GDestroyNotify` releases go-gst's global `gopointer` handle (local cgo helper if go-gst v1.4.0 offers no nil-safe form)
- [x] Drain remaining bus messages in `watchBus` before it returns; flush the bus and release it explicitly in `Stop()`
- [x] Release the forced realtime clock explicitly instead of only nil-ing `g.realtimeClock`
- [x] Release the pipeline last, in reverse acquisition order, after `SetState(NULL)` + the state-change wait; delete the incorrect "the GC finalizer releases the other" comment
- [ ] Assert finalization at a known point (weak-ref or refcount check) and log at ERROR if the C pipeline was not actually freed
- [x] Release each sample explicitly in `onNewSample` after the frame bytes are copied
- [x] Release the probe element in `diagnoseGPUEncoderFailure` (and cache the availability result so a CUDA context is not created on every parse failure)
- [ ] Re-run Phase 2 tests — both sub-tests green; capture the output for the PR
- [~] Re-run the leaks tracer and confirm no surviving `GstPipeline`/`GstElement`/`GstBus` per cycle

## Phase 4 — Encoder / GPU detection

- [ ] Add hardware-based `detectGPUVendor()` (device node + `/sys/class/drm/*/device/vendor`), independent of `checkGstElement`, with a test seam
- [ ] Replace `isNvidiaGnome` / `isAmdGnome` derivation in `ws_stream.go` so branch choice comes from detected vendor, not encoder availability
- [ ] On NVIDIA hardware with NVENC unavailable, fail loudly: ERROR log with the real cause + specific client-facing error; never take the `always-copy=true` branch
- [ ] Replace the `GNOME + AMD/Intel detected` log with one naming detected vendor, compositor and chosen encoder
- [ ] Extend `ws_stream_select_encoder_test.go` to cover NVIDIA+NVENC, NVIDIA-without-NVENC, AMD/Intel, Sway, macOS virtio-gpu

## Phase 5 — Reconnect storm

- [ ] Reset the retry budget only on `videoStarted` (first decoded keyframe); remove the 2 s `connectionStabilityTimer` reset and the `resetRetryState()` on `connectionComplete`
- [ ] Consolidate reconnect scheduling to a single owner across `WebSocketStream` and `DesktopStreamViewer`
- [ ] Implement a terminal give-up state with a specific UI error and a working user-driven Retry that resets the budget
- [ ] Suspend reconnection while `document.visibilityState !== 'visible'`; resume on `visibilitychange`
- [ ] Diagnose and eliminate the redundant connections that never send `init` (target 1 WebSocket per intended stream, down from ~3.5)
- [ ] `cd frontend && yarn build`

## Phase 6 — Guard and circuit breaker

- [ ] Add a 30 s self-check reading `/proc/self/fd` nvidia0 count and own GPU MiB; ERROR log above the warn threshold
- [ ] Refuse new pipeline instantiation above the hard ceiling with a clear client-facing error
- [ ] Give the `SharedVideoSource` circuit breaker a terminal state that surfaces an error to the client instead of permitting one instantiation per cooldown indefinitely

## Phase 7 — End-to-end verification in the inner Helix

- [ ] Register/log in at `http://localhost:8080` (`test@helix.ml` / `helixtest`), complete onboarding, create a spec task with a desktop
- [ ] Open the desktop stream in the browser and confirm frames actually flow (static ≈ 10 FPS, terminal 15–35, vkcube 55–60); screenshot into `screenshots/`
- [ ] Repeat 20 connect/disconnect cycles while watching `/dev/nvidia0` fd count and `nvidia-smi` for the desktop-bridge pid — confirm both flat
- [ ] Verify the reconnect give-up UI end-to-end in the browser (kill the stream, observe capped backoff, give-up message, Retry recovery)
- [ ] Confirm the encoder log names NVIDIA correctly and `nvenc` is selected
- [ ] `go build ./...` in `api/`; `cd frontend && yarn build`

## Phase 8 — Ship

- [ ] Write up findings in `design/2026-07-29-desktop-bridge-gpu-leak.md` in the helix repo
- [ ] Open the PR with the red-then-green regression test output, the leaks-tracer before/after, and the fd/GPU cycle measurements
- [ ] Check CI yourself (`gh pr checks` / Drone MCP tools) and fix any failures
