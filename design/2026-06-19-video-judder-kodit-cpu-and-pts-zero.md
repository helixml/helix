# Video judder / out-of-order frames — root cause (2026-06-19)

Autonomous root-cause session. Symptom (Luke): out-of-order / stale-frame
"judder" on the desktop video stream, **both Safari and Chrome**, "even just
moving windows around," worse when the system is busy. Has resisted every
producer-side fix (the GL→CUDA fence in image `9d32f0` was real but only fixed
the drag-stale-frame; this is a different cause).

## TL;DR — two independent, confirmed problems

1. **PRIMARY (judder): CPU saturation from Kodit ONNX inference starves the
   realtime video pipeline.** `helix-api-1` runs Kodit code-indexing, which does
   **local ONNX ML inference** (code embeddings + zero-shot classification). A
   5s CPU profile of the live API was **95.13% inside `libonnxruntime.so`**.
   libonnxruntime defaults to intra-op parallelism across **all 48 cores**, and
   the API container has **no CPU limit and no ONNX thread cap**. Result: box at
   **91% user / 0.1% idle, load 80–94**, `helix-api-1` burning ~13 cores (1333–
   1640%). The video relay goroutines live in that same saturated process (cgo
   ONNX pins OS threads and won't yield to the Go scheduler), and the single-
   threaded PipeWire capture loops get descheduled → uneven/stalled frame
   delivery. Measured: a CLI stream client saw arrival stalls up to **230 ms**
   (p99 ~60 ms) under this load while the producer's own `frame_interval` was
   comparatively clean — i.e. jitter is added downstream of capture, in the
   starved relay. Both browsers see it because it's pre-client. Tracks "system
   busy / warm today" exactly (Luke's own instinct was right).

2. **SECONDARY (confirmed bug): every delivered frame has `pts=0`.** Wire
   capture (oooprobe) shows header[3:11] = 0 on every frame
   (`hdr=01 01 00 00 00 00 00 00 00 00 00 0f 00 08 70`, width/height correct).
   The browser feeds that straight into the decoder:
   `new EncodedVideoChunk({ timestamp: Number(ptsUs)=0, ... })`
   (`websocket-stream.ts:1330`), so **every VideoFrame.timestamp is 0**. The
   `PlayoutScheduler` and the `[PRESENT-OOO]` detector key on `frame.timestamp`,
   so with all-zero timestamps the client cannot detect, order, or dedupe frames
   by time — it has no safety net when delivery goes bursty/out-of-order, and the
   frame-latency/drift stats are dead. NOTE: WebSocket is TCP (in-order in
   transit), so pts=0 does **not** by itself reorder a single stream — it removes
   the client's ability to *cope* with reordering/bursting and to measure it.
   The start-of-session GOP-replay catchup (`IsReplay` 0x02 frames) is the one
   place real reordering can be introduced before TCP, and Luke notes the OOO is
   "especially at the beginning of the session" — consistent.

## Why pts=0 (producer trace)

- Client reads pts from header[3:11] (`websocket-stream.ts:1181`).
- Server writes it: `ws_stream.go:1402 PutUint64(header[3:11], pts)` where
  `pts = frame.PTS`.
- `frame.PTS` set in `gst_pipeline.go:269-303`: `buffer.PresentationTimestamp()
  .AsDuration()`; if nil → `pts` stays 0. **No fallback.**
- Zerocopy producer is supposed to stamp it: `pipewiresrc/imp.rs:1097-1099`
  `if let Some(buffer_ref) = buffer.get_mut() { buffer_ref.set_pts(wall_clock_ns) }`.
  `buffer.get_mut()` returns None when the GstBuffer is **not uniquely owned**
  (pooled CUDA buffers are Arc/pool-backed) → the set_pts is silently skipped →
  PTS stays GST_CLOCK_TIME_NONE → Go reads nil → pts=0.
- `ext_image_copy_capture.rs:512` explicitly emits `pts_ns: 0` too.
  → Multiple capture paths can produce pts=0. Fix belongs at the Go chokepoint
  that ALL paths pass through.

## Fixes

### Fix A — pts=0 (producer, all capture paths)
`api/pkg/desktop/gst_pipeline.go`: after reading the buffer PTS, if it's
zero/invalid synthesize a **monotonic** PTS from the appsink wall clock
(`time.Now().UnixMicro()`). The appsink callback fires in pipeline (capture)
order, so this is monotonic and reflects true frame order; it is also consistent
with the existing wall-clock-µs values the zerocopy path uses when set_pts DOES
run (both ~1.7e15 µs), so the client's drift math keeps working. Deploy = rebuild
desktop image (`./stack build-ubuntu`) → fresh session. Verify with oooprobe:
header pts must be non-zero & monotonically increasing.
(Optional proper zerocopy fix: in `imp.rs` set PTS before the buffer can be
shared, or stop relying on `get_mut()` succeeding.)

### Fix B — Kodit ONNX must not saturate CPU (PRIMARY)
A background code-indexer doing unbounded local ONNX inference on the same box
(and same process) as the realtime video relay is the architectural fault.
**The reliable lever is a hard CPU cap on the API container** (`cpus: 36` of 48,
staged in docker-compose.dev.yaml, tunable via `HELIX_API_CPUS`): it leaves the
separate desktop containers guaranteed CPU headroom so their capture loops never
starve, regardless of how ONNX threads itself.
- IMPORTANT: `OMP_NUM_THREADS`/`ORT_*` env vars are **best-effort only** — this
  libonnxruntime links NO OpenMP (ldd: no libgomp) and exposes
  `SetIntraOpNumThreads` (a SessionOptions/code control), so it sizes intra-op
  threads in code and likely IGNORES the env. Don't rely on them alone.
- And/or run Kodit embeddings via an EXTERNAL provider instead of local ONNX
  (`KODIT_TEXT_EMBEDDING_BASE_URL` / `KODIT_VISION_EMBEDDING_BASE_URL` proxy),
  moving the ML CPU off this box entirely.
- And/or give the API a cgroup CPU reservation for the relay, or move Kodit
  indexing to a separate process/container with a CPU quota.
Confirmation experiment (needs the restart, so left for Luke since the shared box
has other users' live sessions kaiya/phil): with `OMP_NUM_THREADS=4`, re-run the
oooprobe arrival-jitter measurement under indexing load — stalls should collapse
toward a clean ~16.7 ms cadence.

## Evidence artifacts
- CPU profile: `go tool pprof` top = `[libonnxruntime.so] 95.13%`, plus
  `kodit/...OpenAIProvider.Embed`, `hugot ZeroShotClassificationPipeline`,
  `ortgenai`. API logs: continuous `INSERT INTO vectorchord_code_embeddings`
  (snippet_id ~1.23M), `periodic sync ... pending=8`.
- `helix-api-1` 1333–1640% CPU, 171 threads ~17 @ 78%; box 0.1% idle, load 80–94.
- oooprobe (`cmd/oooprobe/main.go`, throwaway): pts=0 on every frame; arrival
  jitter p50≈16.7 ms, p99≈60 ms, max 230 ms under load; ooo(pts)=0 but pts is
  unusable so that count is not meaningful.
- GPU is NOT the issue: 68 °C, all throttle/slowdown "Not Active", 32% util,
  1 NVENC session. Thermal ruled out.
- Running desktop image is `helix-ubuntu:d97cc3` (a colleague rebuild, NOT main;
  it does contain the fence — `cuda sync wait` string present). Image churn all
  day: 9d32f0 → 89fb36 → fc2da1(mine, from main) → d97cc3. Shared dev env.
```
