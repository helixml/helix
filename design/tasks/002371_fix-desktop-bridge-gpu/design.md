# Design: Fix desktop-bridge GPU and FD Leak in GStreamer Pipeline Teardown

## 1. Root cause analysis

The brief's hypothesis was "`Stop()` releases only one of two refs; the GC
finalizer never runs because GC pressure never builds". Reading the actual
dependency source (`go-gst v1.4.0`, `go-glib v1.4.0`, both downloaded and
inspected while writing this doc) shows the mechanism is **different and worse
than a probabilistic GC problem** — it is a deterministic pin. The brief was
right to say *verify before fixing*; here is what verification found.

### 1.1 The `Stop()` comment is factually wrong

```go
// The go-gst TransferNone/Take wrapper adds its own ref+finalizer, so
// this Unref releases our usage ref; the GC finalizer releases the other.
g.pipeline.Unref()
```

Trace it:

- `gst.NewPipelineFromString` calls `gst_parse_launch`, which returns a
  **floating** ref (GstObject derives from `GInitiallyUnowned`), and wraps it
  with `FromGstPipelineUnsafeNone`
  (`go-gst@v1.4.0/gst/gst_pipeline.go:47,27`).
- `FromGstPipelineUnsafeNone` → `glib.TransferNone` → `glib.Take`
  (`go-glib@v1.4.0/glib/gobject.go:108,122`):

```go
func Take(ptr unsafe.Pointer) *Object {
	obj := newObject(ToGObject(ptr))
	if obj.IsFloating() {
		obj.RefSink()      // <-- floating case: sink, refcount stays 1
	} else {
		obj.Ref()
	}
	runtime.SetFinalizer(obj, (*Object).Unref)
	return obj
}
```

Because the pipeline is floating, `Take` calls `RefSink()`, so the **net
refcount is 1** and the Go wrapper owns that single ref via its finalizer. There
is no second ref. Consequences:

1. `g.pipeline.Unref()` in `Stop()` drops the wrapper's *only* ref.
2. The finalizer is still armed, so a later GC would `g_object_unref` a second
   time — exactly the double-unref hazard the `watchBus` code already
   documents and defends against for bus messages (`gst_pipeline.go:374-384`).
3. Since 45 hours of this produced no `REFCOUNT_VALUE > 0` assertion and no GPU
   release, the refcount at that point must have been **greater than 1**:
   something else is holding refs. Finding those holders is the actual job.

### 1.2 The ref holders — four leaks in `gst_pipeline.go`

**(a) The appsink element ref — leaked, and it pins GPU buffers.**

```go
elem, err := pipeline.GetElementByName("videosink")
```

`Bin.GetElementByName` wraps `gst_bin_get_by_name` with
`glib.TransferFull` (`gst_bin.go:133`) — the Go wrapper owns a **full ref on the
appsink**, released only by a GC finalizer. `Stop()` does `g.appsink = nil` and
never unrefs. When the pipeline is disposed and removes its children, the
appsink's refcount does not reach zero, so the appsink is **not** disposed. A
live appsink keeps its buffer pool and up to `max-buffers=2` queued samples —
which on the zero-copy path are CUDA memory / DMA-BUF handles, i.e. `/dev/nvidia0`
fds.

**(b) The appsink callbacks — a hard, GC-proof pin on the whole `GstPipeline`.**

```go
g.appsink.SetCallbacks(&app.SinkCallbacks{NewSampleFunc: g.onNewSample})
```

`Sink.SetCallbacks` does `ptr := gopointer.Save(cbs)`
(`gst/app/gst_app_sink.go:151-164`) — the callback struct is stored in a
**process-global map** and freed only by the `GDestroyNotify` GStreamer invokes
when the appsink is disposed or the callbacks are replaced.

Follow the chain: global gopointer map → `*app.SinkCallbacks` →
`NewSampleFunc` (a method value on `g`) → `*GstPipeline` → `g.pipeline`
(`*gst.Pipeline`) → the embedded `*glib.Object` **whose finalizer is the only
thing that would drop the last ref**.

Because (a) prevents the appsink from ever being disposed, `GDestroyNotify` never
fires, so the map entry lives forever, so the `*glib.Object` is permanently
reachable from a GC root, so **its finalizer can never run — regardless of GC
pressure**. This upgrades the brief's "GC pressure never builds" from a
probabilistic hypothesis to a deterministic cycle: appsink ref → gopointer entry
→ Go pipeline object → finalizer never runs → C pipeline never finalized →
appsink never disposed.

**(c) The bus — leaked, and queued messages hold refs on the encoder.**

`watchBus` calls `g.pipeline.GetPipelineBus()`, which wraps
`gst_pipeline_get_bus` (transfer full) with `FromGstBusUnsafeFull`
(`gst_bus.go:82`) and caches it on `Pipeline.bus`. Nothing unrefs it. Worse, the
bus is never flushed at teardown: any messages still queued hold refs on their
source elements — `nvh264enc`, `pipewirezerocopysrc` — keeping the CUDA context
and NVENC session alive. That is the plausible home of the bulk of the 28 MiB.

**(d) The forced realtime clock** (`gst.NewSystemClock` + `ForceClock`) — same
finalizer-only ownership; `g.realtimeClock = nil` does not unref.

### 1.3 Two more leak sites outside `Stop()`

**Samples are never unreffed.** `Sink.PullSample` returns
`FromGstSampleUnsafeFull` (`gst_app_sink.go:130-136`) — finalizer-owned. In
`onNewSample` the sample is used and dropped; every one of them holds a
`GstBuffer` that may wrap CUDA memory or a DMA-BUF fd, released only when the GC
gets round to it. At 60 fps that is a continuous GPU-memory overhang even in the
healthy case.

**`diagnoseGPUEncoderFailure` leaks a CUDA context per call.** It creates a probe
`nvh264enc` via `gst.NewElement` and cleans up with `testElem.SetState(gst.StateNull)`
only — never unreffed. This runs on **every pipeline parse failure**, i.e.
precisely during GPU exhaustion. Self-amplifying.

### 1.4 Why the accounting looked clean

Every one of these is invisible to `activePipelineCount`, to the
"stopped"/"cleaned up" log lines, and to the "pipeline stuck" guard. All of them
are Go-side bookkeeping; none of them observe the C refcount. That is why the
brief's key insight — "Go-side accounting says everything was freed" — is exactly
right and why the fix must add a *C-side* assertion, not another counter.

## 2. Verification plan (do this before writing the fix)

Two independent measurements, both runnable in this sandbox (GPU present,
GStreamer 1.26.6, `nvh264enc` registers, live `desktop-bridge` at PID 945 with a
clean baseline of **0** nvidia0 fds while idle).

**V1 — the ratchet (cheap, proves the leak exists).**

```bash
P=$(pgrep -f /usr/local/bin/desktop-bridge)
watch -n1 "ls -l /proc/$P/fd | grep -c nvidia0"
```

Drive repeated stream connect/disconnect cycles and record the per-cycle delta.
Expect ~+4–5 fds per cycle, never returning.

**V2 — the leaks tracer (proves *which* C objects survive).**

```bash
GST_TRACERS="leaks(GstPipeline,GstElement,GstBus,GstSample,GstBuffer)" \
GST_DEBUG=GST_TRACER:7 desktop-bridge ...
# after N cycles:
kill -USR1 $P   # dump live objects
kill -USR2 $P   # dump creation/refcount activity for the leaked ones
```

This is the definitive answer to §1 and must be captured in the PR. Additionally,
temporarily log `GST_OBJECT_REFCOUNT(pipeline)` immediately before the `Unref()`
in `Stop()`. If it is 1, the analysis in §1.1 holds and the survivors are the
children; if it is >1, follow the tracer to the extra holder.

Only after V1 and V2 agree on a holder set do we write the fix.

## 3. The fix

### 3.1 Explicit ownership discipline in `GstPipeline` (primary)

Principle: **`GstPipeline` owns every go-gst object it acquires and releases all
of them, in reverse acquisition order, at a single known point.** No GPU-backed C
object is left to a GC finalizer.

A helper captures the correct pattern once (the pattern `watchBus` already uses
for messages — disarm, then unref, so the finalizer cannot double-free):

```go
// releaseGObject disarms the GC finalizer go-glib installed in Take/TransferFull
// and drops our ref immediately. The finalizer lives on the embedded *glib.Object,
// so that exact pointer must be passed to SetFinalizer — passing the outer
// *gst.Pipeline / *gst.Element silently does nothing.
func releaseGObject(obj *glib.Object) {
	if obj == nil {
		return
	}
	runtime.SetFinalizer(obj, nil)
	obj.Unref()
}
```

`Stop()` becomes, after the existing `SetState(NULL)` + `GetState` wait and the
existing stuck-pipeline guard:

1. **Clear the appsink callbacks** — releases go-gst's global `gopointer` handle
   via `GDestroyNotify`, breaking the §1.2(b) pin. (See open question 4 in
   requirements.md: go-gst v1.4.0 has no documented nil form; a small local
   cgo helper calling `gst_app_sink_set_callbacks(sink, NULL, NULL, NULL)` may be
   needed.)
2. **Flush and release the bus** — set the bus flushing so queued messages are
   dropped and their element refs released, then `releaseGObject(bus)`. Also make
   `watchBus` drain remaining messages before it returns.
3. **Release the appsink element ref** (`releaseGObject`).
4. **Release the forced clock** if one was created.
5. **Release the pipeline** (`releaseGObject`) — now the last ref.
6. **Assert it worked.** Before step 5, read `GST_OBJECT_REFCOUNT`; after,
   log at ERROR if the pipeline was not actually finalized. A weak-ref /
   `g_object_weak_ref` style check, or a `finalize`-signal probe, turns "we
   believe it is freed" into an observable fact. This is the C-side counterpart
   the current code lacks.

Ordering matters: callbacks must be cleared before the appsink ref is dropped,
and the bus must be flushed before elements are released.

### 3.2 Per-frame sample release

In `onNewSample`, after copying the buffer bytes, explicitly release the sample
(disarm + `Unref`) rather than letting the finalizer do it. The data is already
copied into a Go slice at that point, so this is safe and removes the per-frame
GPU-memory overhang.

### 3.3 `diagnoseGPUEncoderFailure`

Release the probe element after `SetState(NULL)`. Better: cache the availability
result rather than instantiating a CUDA-context-creating element on every parse
failure.

### 3.4 Why not `runtime.GC()`

Explicitly rejected, per the brief and the repo's no-hacks rule. Beyond being a
workaround, §1.2(b) shows it **would not even work**: the gopointer map entry is a
live GC root, so no amount of GC pressure can run that finalizer. Deterministic
release is the only correct answer.

## 4. Regression test

`api/pkg/desktop/gst_pipeline_leak_test.go`, build tag `cgo && linux`.

- Skips (does not fail) when `/dev/nvidia0` is absent or `nvh264enc` does not
  register — so non-GPU CI hosts are unaffected.
- Uses a GPU pipeline that needs no PipeWire session, so the test is
  self-contained:
  `videotestsrc is-live=true ! video/x-raw,width=1920,height=1080 ! cudaupload ! nvh264enc ! h264parse ! appsink name=videosink`
- Two sub-tests matching the two shapes seen in production:
  - `create → Stop()` without `Start()` — the 213-case;
  - `create → Start() → consume a few frames → Stop()` — the 111-case.
- Measurement: count `/dev/nvidia0` entries in `/proc/self/fd`, plus GPU MiB for
  the test's own pid from `nvidia-smi --query-compute-apps=pid,used_memory`.
  Discard the first 3 cycles (CUDA driver's own persistent fds/context) and
  assert cycles 4..N are flat within a small constant.
- N = 20. At the observed 4.66 fds/cycle this is a ~93 fd delta on broken code —
  far outside any noise band.

This must be shown failing on `main` and passing on the branch, with both outputs
in the PR body.

## 5. Reconnect storm (US-3)

### 5.1 Why the existing backoff never engages

`frontend/src/lib/helix-stream/stream/websocket-stream.ts` already implements
exponential backoff with jitter capped at 30 s and `maxReconnectAttempts = 10`.
It never fires because the counter is reset on signals that do **not** mean the
stream works:

- `onOpen` arms a 2 s `connectionStabilityTimer` that sets
  `reconnectAttempts = 0` merely because the socket stayed open
  (`websocket-stream.ts:474-483`);
- `DesktopStreamViewer` calls `resetRetryState()` on `connectionComplete`
  (`DesktopStreamViewer.tsx:972`), which also zeroes
  `manualReconnectAttemptsRef`.

In the incident the WebSocket *did* open and complete — the failure was
downstream (no frames). So the budget reset on every cycle and the give-up branch
was unreachable. The ~14.5 s period matches
`VIDEO_START_TIMEOUT_MS = 15000` (`DesktopStreamViewer.tsx:642`) almost exactly:
that watchdog is the real driver of the cycle.

### 5.2 Changes

- **Reset the retry budget only on a working stream** — the `videoStarted` event
  (first decoded keyframe), not `open`/`connectionComplete`. Remove the 2 s
  stability reset.
- **One owner for reconnection.** `WebSocketStream` and `DesktopStreamViewer`
  both schedule reconnects and race (the code already carries scar tissue for
  this: "Reconnection already pending", "Cancelling previous pending reconnect",
  and `design/2026-05-21-stream-reconnect-storm-root-cause.md`). Make the
  component the single scheduler and have the stream never self-reconnect, or
  vice versa — but exactly one.
- **Terminal give-up state**: after the budget is exhausted, stop connecting and
  render a specific error plus the existing user-driven Retry control
  (`onReconnect`, `DesktopStreamViewer.tsx:5298`), which resets the budget.
- **Suspend while hidden**: no reconnect attempts when
  `document.visibilityState !== 'visible'`; resume on `visibilitychange`. The
  incident was a *finished* task with the tab merely left open.
- **Kill the connections that never send `init`** (1708 of 2383). Investigate:
  React StrictMode double-mount; `connect()` closing `this.ws` and immediately
  opening a new socket; component reconnect racing the stream's pending backoff
  timer. Target: one WebSocket per intended stream.

## 6. Encoder / GPU detection (US-4)

`ws_stream.go:681` derives hardware identity from plugin availability:

```go
isNvidiaGnome := !isSway && (encoder == "nvenc" || checkGstElement("nvh264enc"))
```

When NVENC cannot register, this flips false → `isAmdGnome` true (`:694`) → the
`pipewiresrc always-copy=true` branch (`:721-733`) on NVIDIA hardware, which
delivered ~0 frames.

Change:

- Add `detectGPUVendor()` reading hardware, not plugins: presence of
  `/dev/nvidia0` plus `/sys/class/drm/*/device/vendor == 0x10de` (reuse
  `getRenderDevice()` where it already resolves the render node). Compute once;
  it must not depend on `selectEncoder()`.
- Branch selection uses the detected vendor. `isAmdGnome` becomes "vendor is
  AMD/Intel", not "not NVIDIA and not macOS".
- NVIDIA + no NVENC → **fail loudly**: log at ERROR with the real cause, send a
  specific error to the client, do not stream. Per the repo's NO FALLBACKS rule.
  (Requirements open question 2 — confirm before implementing.)
- Replace the `GNOME + AMD/Intel detected` log with one that states the detected
  vendor, the detected compositor, and the chosen encoder, so the triage
  signal is honest.
- Extend `ws_stream_select_encoder_test.go`, which already has a
  `checkGstElement` stub seam (`withStubbedGstElements`), with a parallel seam
  for the vendor detector, and cover all five environment cases.

## 7. Resource guard and circuit breaker (US-5, US-6)

- **Guard**: a 30 s ticker in desktop-bridge reads `/proc/self/fd` (count
  `/dev/nvidia0` symlinks) and its own GPU MiB. Warn threshold → ERROR log with
  the counts. Hard ceiling → refuse new pipeline instantiation and return a
  clear error to the client. Healthy reference: 52 fds / 252 MiB. Cheap: a
  directory read every 30 s, nothing per-frame.
- **Circuit breaker** (`shared_video_source.go:41-51, 483-521`): today it opens
  after N consecutive failures and permits one instantiation per 30 s cooldown —
  once the leak is fixed that is correct throttling, but it must also reach a
  terminal state that surfaces an error to the client rather than looping
  forever. It stays a failure throttle; it is explicitly not the thing standing
  between us and GPU exhaustion.

## 8. Files

| File | Change |
|---|---|
| `api/pkg/desktop/gst_pipeline.go` | Ownership discipline in `Stop()`, `releaseGObject` helper, sample release in `onNewSample`, bus drain/flush in `watchBus`, probe-element release in `diagnoseGPUEncoderFailure`, post-release finalization assertion |
| `api/pkg/desktop/gst_pipeline_leak_test.go` | **new** — fd/GPU-flatness regression test |
| `api/pkg/desktop/ws_stream.go` | Hardware-based GPU vendor detection, loud failure, honest logging |
| `api/pkg/desktop/ws_stream_select_encoder_test.go` | Cases for the five environments |
| `api/pkg/desktop/shared_video_source.go` | Circuit-breaker terminal state; resource guard hook |
| `frontend/src/lib/helix-stream/stream/websocket-stream.ts` | Retry budget reset only on working stream; single reconnect owner |
| `frontend/src/components/external-agent/DesktopStreamViewer.tsx` | Terminal give-up UI, visibility gating, remove premature `resetRetryState()` |

## 9. Notes for future agents

- **go-gst v1.4.0 ownership cheat sheet** (verified by reading the module
  source; do not trust comments in our code):
  - `glib.Take` / `TransferNone`: ref-sinks a floating object or refs a
    non-floating one, then arms a GC finalizer. **Net refcount 1, owned by Go.**
  - `glib.TransferFull`: takes the caller's ref, arms a GC finalizer.
  - `gst_parse_launch` returns floating → `NewPipelineFromString` gives you
    exactly one Go-owned ref.
  - `Bin.GetElementByName` → `TransferFull`: **you own a ref on the element**.
  - `Pipeline.GetPipelineBus` → `TransferFull`, cached on the Pipeline struct.
  - `Sink.PullSample` → `TransferFull` on every sample.
  - `Sink.SetCallbacks` → `gopointer.Save`, a **process-global map entry**
    released only by `GDestroyNotify`. This is the sharpest edge in the binding:
    it can pin an arbitrary Go object graph forever.
- **Disarm before unref.** `runtime.SetFinalizer(x, nil)` then `x.Unref()`. And
  the finalizer lives on the embedded `*glib.Object` — pass that exact pointer,
  not the outer `*gst.Pipeline`. `watchBus` already does this correctly for
  messages and its comment explains the failure mode well; copy that pattern.
- **Go-side counters prove nothing about C-side lifetime.** The whole incident
  had `activePipelineCount == 0` while 1564 fds were held. Any future "did we
  free it?" question must be answered with the GStreamer leaks tracer or a
  weak-ref, not a counter.
- **The spec-task sandbox is a GPU desktop container.** `/dev/nvidia0`,
  `nvidia-smi`, GStreamer 1.26.6 with `nvh264enc`, and a live `desktop-bridge`
  are all present. GPU-level debugging does not need a special host.
- `design/2026-05-21-stream-reconnect-storm-root-cause.md` in the helix repo
  covers the earlier "superseded" storm; this task is the second storm with a
  different reset path.
