# desktop-bridge GPU leak — root cause and fix

2026-07-29. Follows the 2026-07-28 incident on `meta/node01` where one
`desktop-bridge` accumulated **9.3 GB of GPU memory and 1564 open `/dev/nvidia0`
fds** over 45 hours on a shared 16 GB RTX 2000 Ada, exhausted the card, and broke
a different user's desktop (no NVENC → software `openh264` → wrong "AMD/Intel"
pipeline branch → zero frames; `ghostty` GL segfault → Zed never launched).

## Result

`/dev/nvidia0` fds on a live `desktop-bridge`, driving real stream
connect/disconnect cycles (each cycle creates and destroys a GStreamer pipeline):

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

**+14 per cycle, unbounded → flat.** Production leaked 1512 fds across 111
*started* pipelines = 13.6 per pipeline. The GStreamer leaks tracer goes from a
per-cycle set of survivors to zero live objects.

## How to reproduce and measure

The leak needs the real capture path (`pipewirezerocopysrc` + CUDA DMA-BUF
import). A synthetic `videotestsrc ! cudaupload ! nvh264enc` pipeline does **not**
reproduce it — worth knowing before you spend an hour on a unit test that passes
on broken code.

```bash
# 1. Shorten the pipeline grace period so each cycle really tears down.
#    (wrap /usr/local/bin/desktop-bridge, or set in the container env)
export VIDEO_GRACE_PERIOD_SECONDS=5
export GST_TRACERS=leaks GST_LEAKS_TRACER_SIG=1 GST_DEBUG=GST_TRACER:7

# 2. Drive cycles: connect to the bridge's own listener, send init, read frames,
#    disconnect, wait out the grace period, repeat.
#    ws://localhost:9876/ws/stream  with {"type":"init","width":1920,...}

# 3. Watch the only number that cannot lie.
P=$(pgrep -f /usr/local/bin/desktop-bridge)
watch -n1 "ls -l /proc/$P/fd | grep -c nvidia0"

# 4. Ask GStreamer what is still alive.
kill -USR1 $P     # dumps object-alive records
```

Note: `GST_LEAKS_TRACER_STACK_TRACE=1` produced empty traces on this build (no
backtrace support compiled in), so the dump gives you type + refcount only. That
was still enough — the type histogram is the diagnosis.

## Two independent bugs

### Bug 1 — Go side, `api/pkg/desktop/gst_pipeline.go`

Objects, not GPU. After three cycles the tracer showed alive: 21 × `GstSample`,
20 × `GstBuffer`, 40 × `GstMemory`, 1 × `GstAppSink <videosink>`,
1 × `GstBus <bus7>`.

go-gst hands back Go wrappers that own a C reference and release it from a
**runtime finalizer**. Four of those were never released explicitly:

| call | transfer | what leaked |
|---|---|---|
| `Sink.PullSample()` | full | a `GstSample` per frame |
| `Sample.GetBuffer()` | takes a ref despite the "None" name | a `GstBuffer` per frame |
| `Bin.GetElementByName()` | full | the appsink; `Stop()` only did `g.appsink = nil` |
| `Pipeline.GetPipelineBus()` | full | the bus, never released *or flushed* |

It also had a **latent crash**. `gst_parse_launch` returns a *floating* reference
which `glib.Take` ref-sinks, so the net refcount is 1 and the Go wrapper owns it.
`Stop()`'s `Unref()` therefore freed the pipeline while the finalizer stayed
armed; the next GC ran `g_object_unref` on freed memory. The first Go test
written for this task hit it immediately:

```
g_object_unref: assertion 'G_IS_OBJECT (object)' failed
SIGSEGV ... runtime.runFinalizers -> glib.(*Object).Unref
```

`start-desktop-bridge.sh` already carries a restart loop whose comment reads
*"The desktop-bridge can crash (e.g. segfault during WebSocket reconnection)"*.
That was this.

Fix: `GstPipeline` owns every go-gst object it acquires and releases all of them
in reverse acquisition order in `Stop()`, each via a helper that **disarms the
finalizer before unreffing**. `watchBus` already did exactly this for bus
messages and its comment explains the failure mode well — that pattern is now
applied everywhere.

### Bug 2 — Rust plugin. This was the entire GPU leak.

`desktop/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs`:

```rust
if settings.cuda_context.is_none() {
    settings.cuda_context = Some(Arc::new(Mutex::new(ctx)));
} else {
    std::mem::forget(ctx); // Prevent double-unref
}
```

`gst_cuda_ensure_element_context()` runs a context query, and GStreamer answers
it by calling our own `set_context()` **synchronously on the same thread**, which
populates `settings.cuda_context`. So the `else` branch is not a rare race — it is
taken on **every single pipeline start**. Confirmed from the logs:
`Received CUDA context via set_context` appears exactly once per start, between
`Using PipeWire DmaBuf (GNOME+NVIDIA CUDA mode)` and the blitter init.

At that point `ctx` aliases a `GstCudaContext` pointer it does not own (so
unreffing it really would double-free — the comment's fear was legitimate) but it
*does* own the `GstCudaStream` created alongside it in
`CUDAContext::new_from_gstreamer`. `mem::forget` leaked that stream, and a
`GstCudaStream` holds a reference on its context, so **every pipeline stranded a
whole CUDA context**. The tracer showed it cleanly: N pipelines → N live
`GstCudaContext` + N live `GstCudaStream`, both at refcount 1, every `GstElement`
and the `GstPipeline` correctly finalized, and N `cuda-EvtHandlr` driver threads.

Fix: `CUDAContext::release_stream_only()` in `wayland-display-core` — drop the
`StreamHandle` (releasing its reference on the context), then `mem::forget` only
the borrowed context pointer.

## Notes for whoever hits the next one of these

- **Go-side counters prove nothing about C-side lifetime.** Throughout the
  45-hour leak `activePipelineCount` read 0, teardown accounting balanced exactly
  (213 never-started + 111 stopped = 324), and there were zero "pipeline stuck"
  logs. Every one of those numbers was true and none of them mattered.
- **Fix, deploy, then measure — in that order, every time.** The Go-side fix was
  written, deployed and measured: object leaks gone, crash gone, **fd count
  completely unchanged at +14/cycle**. Without that measurement it would have
  shipped as "the fix".
- **go-gst v1.4.0 ownership cheat sheet** (read from the module source; do not
  trust comments in our code):
  - `glib.Take` / `TransferNone` — ref-sinks a floating object or refs a
    non-floating one, arms a finalizer. Net refcount 1, owned by Go.
  - `glib.TransferFull` — adopts the caller's ref, arms a finalizer.
  - `gst_parse_launch` returns floating, so `NewPipelineFromString` gives you
    exactly one Go-owned ref.
  - `Bin.GetElementByName`, `Pipeline.GetPipelineBus`, `Sink.PullSample`,
    `Sample.GetBuffer` — **all give you a reference you must release.**
  - `Sink.SetCallbacks` stores the callback struct in a process-global map
    (`gopointer.Save`), released only by the `GDestroyNotify` GStreamer fires
    when the callbacks are replaced or the sink is disposed. It can pin an
    arbitrary Go object graph forever.
- **Disarm before unref**: `runtime.SetFinalizer(x, nil)` then `x.Unref()`. For
  GstObject-derived types the finalizer lives on the embedded `*glib.Object` —
  pass that exact pointer, not the outer `*gst.Pipeline`, or `SetFinalizer`
  silently does nothing.
- **The per-pipeline figures in the incident report should be read per *started*
  pipeline**: ~82 MiB and ~13.6 fds across the 111 that reached PLAYING, not
  28 MiB / 4.66 fds across all 324. Pipelines that never started do not leak, and
  a `create → Stop()` loop is flat even on unmodified code.
- `scripts/build-zerocopy-plugin.sh` pinned Rust 1.85.0 while
  `Dockerfile.ubuntu-helix` uses 1.87.0, so the dev script could not build the
  plugin at all. Bumped. If you touch one, check the other.

## Everything else in this change

- **Encoder/GPU detection** (`api/pkg/desktop/gpu_vendor.go`). `isNvidiaGnome`
  was derived from `checkGstElement("nvh264enc")`, so an exhausted GPU
  reclassified NVIDIA hardware as AMD/Intel and took a branch that delivers zero
  frames — while logging `GNOME + AMD/Intel detected` on an RTX 2000 Ada. Vendor
  now comes from `/dev/nvidia0` and the DRM PCI vendor id. NVIDIA without NVENC
  fails loudly with a specific client-facing error instead of streaming nothing.
  An explicit `HELIX_ENCODER` override still wins.
- **Reconnect storm** (`websocket-stream.ts`). Backoff and a 10-attempt give-up
  were already there and never fired: a 2 s "connection stabilized" timer and
  `resetRetryState()` on `connectionComplete` zeroed the counter on every cycle.
  Both reset on signals that do not mean the stream works — in the incident the
  socket kept opening fine and the *video* was what failed. The budget now resets
  only on `videoStarted` (first decoded keyframe), give-up is terminal with a
  real error plus Retry, and reconnection is suspended while the tab is hidden.
- **Circuit breaker** (`shared_video_source.go`). It opened 209 times during the
  incident and let 209 more pipelines through, one per cooldown, forever. It now
  latches after 10 trips and only an explicit user retry (`user_retry` on the
  init message) clears it.
- **Resource guard** (`gpu_guard.go`). 30 s self-check on this process's
  `/dev/nvidia0` fd count: warn at 200, refuse new pipelines at 400. A healthy
  bridge sits at ~52. This would have caught the incident in hour one.
