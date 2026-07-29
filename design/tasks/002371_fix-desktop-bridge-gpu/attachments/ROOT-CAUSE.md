# GPU memory leak — desktop-bridge — root cause

Snapshot: 2026-07-29 08:27–08:34 BST
Leaker: PID 2515110, container `ubuntu-external-01kyhghrzj83vm9x188mbhfmhc`
(session `ses_01kyhghrzj83vm9x188mbhfmhc`, owner luke@helix.ml, task **done**)
Started Mon Jul 27 11:05:09 2026, alive 45h. Image `helix-ubuntu:c9cb78`.

## Measured

| metric | leaker | healthy (963531) | ratio |
|---|---|---|---|
| GPU memory | 9324 MiB | 252 MiB | 37× |
| `/dev/nvidia0` fds | 1564 | 52 | 30× |
| total fds | 1823 | 92 | 20× |
| threads | 256 | 36 | 7× |
| RSS | 9130 MB | 456 MB | 20× |

Pipelines instantiated (from log): **324**

- leaked GPU 9324−252 = 9072 MiB → **28.0 MiB per pipeline**
- leaked fds 1564−52 = 1512 → **4.66 fds per pipeline**

Both independent measures scale linearly with pipeline-instantiation count.

## Pipeline accounting is BALANCED (leak is below GStreamer)

- `[GST_PIPELINE] Starting pipeline` = 324
- `cleaned up (was never started)` = 213 (failed SetState(PLAYING))
- `stopped (active pipelines: N)` = 111
- 213 + 111 = 324 ✓, `activePipelineCount` = **0**
- 0 × `FATAL: pipeline stuck`
- 0 × `Trying to dispose element ... in PLAYING`

Go-side teardown is correct. The C-side objects are not being finalized.

## Bug B — the leak

`api/pkg/desktop/gst_pipeline.go` `GstPipeline.Stop()` releases only ONE ref;
the second is left to go-gst's GC finalizer, per its own comment:

```go
// The go-gst TransferNone/Take wrapper adds its own ref+finalizer, so
// this Unref releases our usage ref; the GC finalizer releases the other.
g.pipeline.Unref()
```

Go's GC cannot see GPU memory. The Go object is a few hundred bytes; the C
object it pins holds ~28 MiB GPU + ~4.7 nvidia0 fds + OS threads. GC pressure
never builds → finalizers never run → CUDA contexts accumulate forever.

Corroboration: 256 threads while `activePipelineCount` = 0.

## Bug A — the amplifier (trigger)

Infinite frontend reconnect storm, no backoff, no give-up. ~3 WS connections
per ~14.5 s cycle; 2383 connections, 1708 `failed to read init message`.
Live cycle still running at 08:33.

## Timeline

- Jul 27 11:05:09 — container starts
- Jul 27 11:06:40 — first `failed to read init message` (storm begins, +90s)
- ~35 h of leaking at ~10 pipeline cycles/hour
- Jul 28 22:21:26 — first `failed to change state to PLAYING` (GPU exhausted)
- Jul 28 22:24:18 — first `Circuit breaker OPEN`
- Jul 28 22:00/23:00 — 160/179 capture starts (storm peak, user watching)
- Jul 28 23:20 — victim desktop (Kai, spt_01kyd24…) restarts, cannot register
  NVENC → openh264 software + wrong "AMD/Intel" branch → frozen video;
  ghostty GL segfault → Zed never launched
- Jul 29 08:34 — plateaued at 9324 MiB (CUDA context creation itself now fails)

Self-reinforcing: exhaustion → start fails → frontend retries → more attempts.

## Fixes

1. **Bug B (real fix)**: deterministic release of GPU resources — do not rely on
   GC finalizers. Drop *all* refs after `SetState(NULL)` + state-change wait so
   the C object finalizes at a known point.
2. **Bug A**: exponential backoff + give-up in the frontend stream client.
3. Circuit breaker still permits one instantiation per cooldown — it throttles
   the leak but does not stop it.

## Files

- `01-nvidia-smi-full.txt`, `02-gpu-procs.csv`, `06-mem-timeseries.txt`
- `03-leaker-full.log` (85 MB, 667k lines — full 45 h history)
- `05-control-full.log` (healthy control)
- `04-victim-full.log` — EMPTY, victim container was recycled before snapshot
