# Video pipeline jitter — full map + instrumentation plan (2026-06-11)

## Problem
Intermittent stutter in desktop streaming. User sees ~50-100ms freezes every few
seconds, even on Ethernet, even playing 60fps YouTube (so NOT damage-based Mutter
idling). Confirmed NOT thermal (GPU 53°C, full clocks), NOT CPU saturation (48
cores, load ~20, scheduler canary 0/0/0), NOT GPU encoder throughput (enc util
6-18%, encoder_latency 0.1ms).

## Key reframing (user's insight)
Don't hunt big gaps (ambiguous — could be Mutter legitimately idle). Hunt **bursts**:
inter-frame intervals < ~8ms. Mutter cannot render at >120fps, so a sub-8ms arrival
is necessarily a buffered-then-released frame = a queue drained after a pileup.
`burst_count` (samples < 8ms per window) is the real signal.

## VERIFIED pipeline (GNOME+NVIDIA, VideoModePlugin=zerocopy, nvenc, dmabuf→CUDA)
Active container confirmed on this path: logs `[PIPEWIRESRC] CudaBuffer ... Bgra`.

| # | Stage | Thread | Buffer/queue | Drop policy | Instrumented? |
|---|-------|--------|--------------|-------------|---------------|
| 0 | Mutter renders | Mutter | — | — | no (3rd party) |
| 1 | Mutter→PW ScreenCast pool | Mutter→PW | negotiated count — **we set dataType ONLY, not count** (pipewire_stream.rs:1249-1302) | Mutter stalls/drops if pool exhausted | **NO** |
| 2 | PW delivers to `.process()` | PW loop thread | — | — | **NO ← Point A** |
| 3 | Hold Mutter DMA-BUF during CUDA copy (dequeue_raw_buffer:851 → queue_raw_buffer:962) | PW loop thread | 1 (Mutter's buf) | — | **NO ← hold-time + CUDA stage time** |
| 4 | `.process()`→`.create()` | PW thread → GST streaming thread | **crossbeam bounded(8)** (pipewire_stream.rs:207) | `try_send` DROPS on full, silent `let _=` (945) | partial (TIMING at create only) **← Point B + depth + drop count** |
| 5 | `create()`→encoder | GST streaming thread | `queue max-size-buffers=1 leaky=downstream` (ws_stream.go:778) | drops OLDEST | NO (GST internal) |
| 6 | cudascale→nvh264enc→h264parse | GST streaming thread | GST internal | — | encoder_latency (appsink−frame.Timestamp) |
| 7 | →appsink | GST | `appsink max-buffers=2 drop=true` (962) | drops on full | NO directly |
| 8 | appsink onNewSample→frameCh | GST→Go | `chan VideoFrame, 8` (gst_pipeline.go:135) | select/default, counted `pipeline_dropped` (=0 observed) | YES |
| 9-10 | frameCh→broadcastFrames→per-client chan | Go | GOP-sized chan | disconnect-on-full | partial |
| 11 | readFramesAndSend→ws.WriteMessage | Go | wsMu serialized | — | YES `avg_send_us` (AVG ONLY) **← add p99/max** |
| 12 | TCP localhost (TCP_NODELAY on, ws_stream.go:1718) | — | socket buf | — | no |
| 13 | revdial WS tunnel (wsconnadapter→gorilla→TCP) | Go | gorilla write buf | — | NO |
| 14 | helix-api resilientProxy 32KB Read→Write (resilient.go:274) | Go | 32KB | — | NO |
| 15 | hijacked TCP→browser (WAN) | — | kernel+WAN | retrans/HoL | mtr only |
| 16 | browser ws.onmessage | JS main | — | — | YES Receive Jitter sparkline **← add burst count** |
| 17 | VideoDecoder.decode | decode | Decode Queue (peak 26 seen!) | — | YES queue size |
| 18 | decoder→render (rAF) | render | — | — | YES Render Jitter |

## Decisive measurement: Point A vs Point B
- **Point A** = inter-arrival at PW `.process()` (imp via pipewire_stream.rs:901, after extract_frame)
- **Point B** = inter-arrival at `.create()` (imp.rs:854, after recv_frame_timeout Ok) — partially done (we added CudaBuffer TIMING; currently records interval but interval is measured create-side)

Logic:
- **A smooth (~16ms) + B bursts** → boundary #4/#5: encoder backpressure through the
  bounded(8) channel. create() blocks on downstream nvh264enc; channel fills; drains
  in a burst. This is OUR coupling.
- **A itself bursts** → boundary #1/#3: Mutter/PW upstream — most likely caused by US
  holding Mutter's DMA-BUF during a variable-length CUDA copy (GPU contention with
  nvh264enc), starving Mutter's small pool. Confirmed/refuted by hold-time + CUDA
  stage timing.

## Instrumentation points (MEASURE ONLY, no behavior change)
1. **Rust `.process()` (Point A)**: inter-arrival hist (p50/p95/p99/max + burst<8ms).
   pipewire_stream.rs process callback.
2. **Rust buffer hold time**: dequeue_raw_buffer(851)→queue_raw_buffer(962) duration
   p50/p99/max. Detects Mutter starvation caused by us.
3. **Rust CUDA stage timing**: real egl/reg/copy in process_dmabuf_to_cuda (currently
   CudaBuffer path logs 0 — record_frame(0) in imp.rs CudaBuffer arm).
4. **Rust channel depth + try_send drops**: depth at send (frame_tx capacity 8), and
   count of dropped frames at line 945.
5. **Rust `.create()` (Point B)**: already have CudaBuffer TIMING interval; add burst<8ms.
6. **Go appsink→send**: extend `frame_interval_ms` to p50/p95/p99 + burst<8ms; add
   writeMessage duration p99/max (have avg_send_us only). ws_stream.go readFramesAndSend.
7. **Client**: receiveBurstCount/renderBurstCount (<8ms); tint sub-8ms sparkline samples;
   add to Copy output. websocket-stream.ts + StatsOverlay.tsx.

## Facts established this session
- Producer-side `frame_interval_ms` max matches client Receive Jitter max (e.g. 94ms ≈
  96ms) → wire is faithful, no downstream amplification; burst originates producer-side.
- Steady-state YouTube: TIMING CudaBuffer interval avg=16ms max=89-97ms (59fps) —
  i.e. 60fps mostly, but a ~90ms hiccup every few sec with sub-8ms catch-up after.
- pipeline_dropped=0 throughout → boundary #8 (appsink→Go chan,8) is NOT the bottleneck.
- avg_send_us 64-100µs → ws write not blocking on average (but no p99/max yet).
- prod IS node01.lukemarsden.net (ping 0.045ms) — we are ON the streaming host.

## Hot-swap loop (no full rebuild needed for Rust plugin)
```
cd api && ... (frontend: yarn build; .env has FRONTEND_URL=/www so dist is live)
# Rust plugin .so:
ID=$(docker create helix-ubuntu:latest); docker cp $ID:/usr/lib/x86_64-linux-gnu/gstreamer-1.0/libgstpipewirezerocopy.so /tmp/; docker rm $ID
docker cp /tmp/libgstpipewirezerocopy.so helix-sandbox-nvidia-1:/tmp/
docker exec helix-sandbox-nvidia-1 docker cp /tmp/libgstpipewirezerocopy.so <ubuntu-external>:/usr/lib/x86_64-linux-gnu/gstreamer-1.0/
docker exec helix-sandbox-nvidia-1 docker exec <ubuntu-external> pkill -9 -x desktop-bridge   # respawns, reloads .so
```
(Building the .so itself needs pipewire-dev etc — use `./stack build-ubuntu` then extract.)
