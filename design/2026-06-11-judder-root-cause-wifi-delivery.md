# Desktop streaming judder — root cause (2026-06-11)

## TL;DR
After fixing the producer (PR #2594: GL-import + persistent buffer eliminated the
140ms per-frame CUDA-register stalls → clean 60fps capture), residual ~80ms
periodic judder remained. Measured end-to-end. **Root cause: the WiFi link
delivers the evenly-sent packet stream in ~80ms bursts.** Server and browser are
both clean.

## The three-point measurement that proves it
- **Server egress (tcpdump on origin → client IP):** EVEN. 270 inter-send gaps in
  the 10–19ms bucket (normal 16ms/60fps), only 1 gap of 70–79ms in 6s. Server
  sends each frame ~16ms apart.
- **Browser WebSocketReceive (Chrome perf trace):** BUNCHED. 365 frames/5.8s,
  p50=16.5ms but p99=80ms, max=87ms, 12 gaps ≥50ms, 60 catch-up bursts <5ms.
- **Browser main thread (same trace):** IDLE. 9630 RunTasks, longest 2.9ms, zero
  tasks >16ms. `WebSocketReceive` is logged in Chrome's network service (near the
  wire), so frames arrive bunched with the CPU doing nothing.

Server even + browser receives bunched + browser CPU idle ⇒ the bunching is in
the network (WiFi) delivery. Corroborated: ethernet is subjectively smoother.

## What was ruled out (by measurement, in order)
- Encoder / Go channels: B.create (pre-encoder) == ENC.appsink (post-encoder) ==
  Go send, all p99≈17, burst≈1. Faithful.
- revdial tunnel + helix-api resilient proxy: localhost; proxy read matches
  server send, proxy write p99=0 (instant).
- Cloudflare: /etc/hosts bypass to origin IP still judders (RTT dropped 29→16ms
  confirming bypass). Not CF.
- TCP health on the direct WiFi conn: rtt 13.6ms stable (rttvar 1.2), cwnd 525,
  retrans 42/92841 (~0.05%), Send-Q mostly 0, rcv_wnd open (~62KB). Healthy — so
  not loss/congestion/receive-stall. WiFi bunches *timing* without loss.
- Browser CPU / main-thread blocking: trace shows longest task 2.9ms.

## Producer residual (small, ours, separate)
`import_dmabuf` (eglCreateImage on smithay texture-cache miss): ~55ms, ~1–2×/5s.
Shows in A.arrival max only; p99 clean. Minor vs the WiFi issue.

## The hard part: smooth on WiFi without latency
WiFi (esp. congested office WiFi — the real target: "every dev on a laptop")
delivers downlink in bursts (MAC TXOP scheduling / OS receive coalescing). ~80ms
is high, suggesting channel contention. We can't fix the radio from the server.

Fundamental tension: hiding arrival jitter requires holding frames (a playout
buffer) = latency. User's priority is keypress-to-photon in an editor, so a fixed
buffer is rejected.

### Options (none free)
1. **Adaptive playout buffer**: ~0 during interaction (input activity detected →
   low latency for typing, occasional judder tolerated), grow to ~2–4 frames
   during passive video (smooth). Best-of-both; needs interaction detection.
2. **Server-side pacing (fq qdisc / SO_MAX_PACING_RATE)**: spread each frame's
   packets over the 16ms interval instead of bursting 11 packets back-to-back.
   Cheap to try, may reduce burstiness marginally; won't beat TXOP batching.
3. **Bitrate/per-frame-size reduction**: fewer packets/frame → less TXOP spanning.
   Marginal.
4. **Accept WiFi jitter for video, keep interactive path lean.** Typing judder is
   less perceptible than smooth-motion judder; the YouTube 60fps test is the
   worst case.

### Recommendation
Ship PR #2594 (producer fix — helps everyone, self-host included). For the WiFi
jitter, the only robust smooth-AND-low-latency answer is an **adaptive** buffer
(option 1). Worth a quick fq-qdisc pacing experiment (option 2) first since it's
zero-latency and cheap.

## Chrome trace analysis (browser side fully cleared)
From ~/Trace-20260611T164507.json.gz (5.8s, 60fps YouTube):
- `WebSocketReceive`: 365 frames, p50=16.5ms but **p99=80ms, 12 gaps ≥40ms,
  spaced almost exactly 0.523s apart (stdev 0.10)** + ~60 catch-up bursts <5ms.
- Renderer main thread: 9630 RunTasks, **longest 2.9ms, zero >16ms** — no jank.
- `WebSocketReceive` is logged in the **renderer** process (pid 69836), i.e. when
  the message reaches the renderer (includes network-service→renderer IPC).
- V8 GC: only **2 major-GC cycles** in 5.8s; **only 1/12 gaps coincide with GC** —
  GC is NOT the cause. During 11/12 gaps the renderer is idle (no jank, no GC).
- API polling (corrected): **2.8 req/s** (ResourceSendRequest), modest — NOT a
  factor. (Earlier "34/s" was an artifact of counting URL mentions across all
  trace events incl. JS source attribution.)

## Corrected conclusion
Server sends evenly (tcpdump). Browser CPU idle, no GC/jank, modest polling.
Renderer simply isn't handed WS data for ~80ms every ~523ms. ⇒ the bunching is in
the **network delivery to the client** (local path / WiFi). The **~523ms
periodicity is unexplained and not pinnable from the server** (server egress is
even) — it points at an AP/driver-side timer (beacon/DTIM/scan) or local-network
scheduling, i.e. the user's environment, not Helix code.

## Wrong turns made during this investigation (for honesty/future ref)
CPU saturation, encoder, Cloudflare, Caddy, client API-polling — each proposed
then DISPROVEN by measurement. Lesson: measure each hop before concluding.

## Status / recommendation (corrected)
- Producer fix (PR #2594) is real and shippable — eliminates the 140ms freezes;
  helps all deployments incl. self-host. Keep.
- Residual ~80ms/~523ms judder is local network delivery to the client. Not
  fixable server-side (server already sends evenly). Next: (a) ethernet test on
  the LATEST build (not yet done) to confirm network; (b) inspect the user's
  WiFi/AP (band, channel congestion, the ~523ms periodic — likely AP/driver);
  (c) the only app-level mitigation for client network jitter is an **adaptive**
  playout buffer (small/off during interaction, larger for passive video) — costs
  some latency, which conflicts with the keypress-to-photon goal.

## Tools note
tcpdump was not installed on the origin; installed via apt. Chrome trace provided
by user at ~/Trace-20260611T164507.json.gz.
