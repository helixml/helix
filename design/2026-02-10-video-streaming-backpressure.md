# Video Streaming Backpressure Design

**Date:** 2026-02-10
**Status:** Implemented (QEMU side), Go side already correct

## Context

The Helix video streaming pipeline encodes VM scanout frames as H.264 and delivers them to browser clients over WebSocket. With multiple agents (each with their own display/scanout), backpressure must be isolated per-scanout — one slow client must never affect other clients or other scanouts.

## Architecture

```
QEMU (host)                    desktop-bridge (guest container)              browser
┌──────────────┐               ┌─────────────────────────────────┐
│ virtio-gpu   │               │                                 │
│ page flip    │               │  ScanoutSource                  │
│     │        │               │  ┌───────────────────────────┐  │
│     ▼        │    TCP        │  │ readFrames (goroutine)    │  │
│ GL blit →    │───────────────│──│ frameCh [16-frame buffer] │  │
│ VT encode →  │  10.0.2.2    │  │ DROPS on full (non-block) │  │
│ send(DONTWAIT)│  :15937      │  └───────────┬───────────────┘  │
└──────────────┘               │              │                  │
                               │              ▼                  │
                               │  SharedVideoSource              │
                               │  ┌───────────────────────────┐  │
                               │  │ broadcastFrames           │  │
                               │  │ GOP buffer (keyframe +    │  │
                               │  │ subsequent P-frames)      │  │
                               │  │                           │  │
                               │  │ Per-client channels:      │  │     ┌─────────┐
                               │  │  Client A [120 frames] ───│──│────▶│ Browser │
                               │  │  Client B [120 frames] ───│──│────▶│ Browser │
                               │  │                           │  │     └─────────┘
                               │  │ If client buffer full:    │  │
                               │  │   DISCONNECT slow client  │  │
                               │  └───────────────────────────┘  │
                               └─────────────────────────────────┘
```

## Backpressure Layers

### Layer 1: QEMU Per-Scanout Ring (pre-encode)

Each scanout has a triple-buffered IOSurface ring. Before encoding a frame, we check if the next ring slot is still being encoded by VideoToolbox:

```c
if (helix_atomic_load(&enc->blit_slot_busy[slot])) {
    return;  // Drop frame — VT still encoding this slot
}
```

**Properties:**
- Per-scanout isolation (each scanout has its own ring)
- Pre-encode drop — no encoder state corruption
- No gl_block — guest GPU keeps running, frames just get dropped
- With `MaxFrameDelayCount=0`, VT produces one-in-one-out, so slots clear quickly

### Layer 2: QEMU TCP Send (post-encode)

After VT encodes a frame, the callback sends it to subscribed TCP clients with `MSG_DONTWAIT`:

```c
ssize_t sent = send(c->fd, response_data, response_size, MSG_DONTWAIT);
```

**Properties:**
- Non-blocking: if TCP buffer full, frame silently dropped for that client
- Per-client: other clients still receive the frame
- Post-encode drop: the encoder has already updated its reference frames

**Risk:** Post-encode drops can corrupt the decoder's P-frame state. The next keyframe (within 60 frames / ~1 second) will self-heal. This is acceptable because:
1. It only happens under extreme backpressure (kernel TCP buffer full)
2. The Go layer has its own 16-frame buffer that absorbs bursts
3. In practice each scanout has one TCP client (desktop-bridge)

### Layer 3: Go ScanoutSource (16-frame buffer)

The TCP reader goroutine reads frames and pushes them into a 16-frame channel:

```go
select {
case s.frameCh <- frame:
    // Queued
default:
    // Channel full, drop (never blocks TCP read)
}
```

**Properties:**
- TCP reader NEVER blocks — always consuming from QEMU
- Absorbs encoding bursts (VT produces frames faster than WS can send)
- Pre-distribution drop — only affects this source, not other scanouts

### Layer 4: Go SharedVideoSource (per-client 120-frame buffer)

Each WebSocket client gets its own 120-frame channel. The broadcaster does non-blocking sends:

```go
select {
case client.frameCh <- frame:
    // Sent
default:
    // Buffer full — disconnect slow client
    s.disconnectClient(clientID)
}
```

**Properties:**
- Per-client isolation: slow client is disconnected, others unaffected
- Large buffer (120 frames = 2 seconds at 60fps) absorbs temporary slowdowns
- New clients get a GOP replay (keyframe + P-frames) to start clean
- Disconnected clients reconnect and get fresh GOP — no corruption

### Layer 5: WebSocket Write (per-client mutex)

Each client has its own mutex for WebSocket writes. A slow write only blocks that client's goroutine:

```go
v.wsMu.Lock()
defer v.wsMu.Unlock()
err := v.ws.WriteMessage(messageType, data)
```

**Properties:**
- Per-client: blocking write only affects one client
- If write errors, client is disconnected

## Why No gl_block

SPICE uses `gl_block` to throttle the guest GPU when the SPICE client can't keep up. This works for SPICE because:
1. There's one SPICE client
2. The client is local (no network latency)
3. gl_block operates on the virtio-gpu command queue (global)

For Helix, gl_block is wrong because:
1. Multiple scanouts share one virtio-gpu command queue
2. One slow scanout would stall ALL scanouts
3. Clients are remote (network latency, disconnections)
4. A disconnected client would freeze the entire VM

Instead, per-scanout ring slot busy flags provide backpressure at the encode level, and the Go layer provides per-client backpressure at the distribution level.

## Key Design Decisions

### glFlush vs glFinish

We use `glFlush()` on virglrenderer's context before the GL blit, not `glFinish()`. `glFinish()` blocks the QEMU main loop until ALL GPU work completes. `glFlush()` submits commands without blocking. The triple-buffer ring provides enough latency for the blit to complete before VT reads the IOSurface (by the time we wrap back to a slot, the blit from 3 frames ago is done).

If blit-before-read corruption is observed, the fix is `glFinish()` on the *helix* EGL context (not virgl's), which only blocks our blit operation, not the QEMU main loop.

### Ring Slot Advance on Drop

The ring index advances even when a frame is dropped. This means if slot 0 is busy but slot 1 is free, we drop on slot 0 and check slot 1 next frame (wasting one frame). Worst case: 2 unnecessary drops when cycling past busy slots. This is acceptable because:
1. With `MaxFrameDelayCount=0`, VT clears slots within milliseconds
2. At 60fps, 2 dropped frames = 33ms of missed content
3. Scanning all slots would add complexity for a rare edge case

### Post-Encode TCP Drop Impact

When a TCP send drops a post-encode P-frame, the browser decoder is out of sync until the next keyframe (up to 60 frames / ~1 second). This is mitigated by:
1. The Go layer's 16-frame TCP read buffer absorbs most bursts
2. The Go layer's 120-frame per-client buffer handles slow WebSocket clients
3. The Go layer disconnects persistently slow clients (who reconnect with a fresh GOP)
4. In practice, the kernel TCP buffer (~128KB on localhost) handles the rest

The only scenario where a post-encode TCP drop occurs is if the Go process is completely stalled (GC pause, CPU starvation). In that case, 1 second of corruption is acceptable.

## Changes Made (2026-02-10)

### QEMU (helix-frame-export.m)

1. **Removed legacy vsock path** (-1400 lines): `encoder_output_callback`, `vsock_server_thread`, `handle_frame_request`, `create_encoder_session`, `helix_encode_iosurface`, `helix_get_iosurface_for_resource`, `helix_get_iosurface_from_scanout`, `helix_create_iosurface_from_pixels`. Desktop-bridge only uses SUBSCRIBE, never FRAME_REQUEST.

2. **Removed gl_block**: Per-scanout ring slot busy flags instead. One slow scanout can't stall others.

3. **glFinish -> glFlush**: On virglrenderer's context. No more main loop stalls.

4. **Fixed ring slot/index mismatch**: Slot selection and ring advance moved into `helix_scanout_frame_ready`. `helix_gl_blit_frame` takes slot as parameter.

5. **Fixed stdatomic.h conflict**: Replaced `<stdatomic.h>` with `__atomic` builtins via `helix_atomic_load/store/xchg` macros.

6. **Fixed MSG_NOSIGNAL**: Added fallback `#define MSG_NOSIGNAL 0` and `SO_NOSIGPIPE` on client accept (macOS).

7. **Fixed partial send**: Check `sent == response_size`, log partial sends.

8. **Fixed CFDictionary leak**: IOSurface properties dict in `create_scanout_encoder` now properly released.

9. **Constrained Baseline + MaxFrameDelayCount=0**: Encoder matches what the SPS rewriter and browser decoder expect. One-in-one-out encoding.

### Go (no changes needed)

The Go desktop-bridge already has correct backpressure:
- `ScanoutSource.readFrames`: 16-frame non-blocking buffer, never stops reading TCP
- `SharedVideoSource.broadcastFrames`: per-client 120-frame buffer, disconnects slow clients
- `VideoStreamer.readFramesAndSend`: per-client WebSocket mutex, blocking write only affects one client

## Open Issue: VM Hang on Video Stream Reconnect

**Status:** Investigating — reproducible
**Symptom:** Toggling the chat box (WebSocket disconnect/reconnect) causes video to freeze on last frame. VM stays alive. Then pressing the reconnect button in the streaming desktop component causes the **entire VM to hang** — SSH frozen, UTM Linux console frozen.

### Sequence of Events

1. Browser toggles chat box → React re-renders → WebSocket disconnects/reconnects
2. Go SharedVideoSource handles the WebSocket reconnect
3. Browser shows one frame then freezes (or shows old frame forever) — VM still alive
4. User presses reconnect button in streaming desktop component
5. Full video stream reconnect attempt
6. **Entire VM hangs** — SSH, console, everything frozen

### Root Cause — CONFIRMED via `sample` (2026-02-10)

**`MSG_DONTWAIT` does not prevent `send()` from blocking on macOS.**

Thread backtrace from frozen VM (`sample $(pgrep qemu) 5`):

```
Thread_1045152 (VT encoder callback — dispatch queue):
  scanout_encoder_callback                    helix-frame-export.m:406
    → helix_send_to_subscribed_clients        helix-frame-export.m:219
      → __sendto                              ← BLOCKED IN KERNEL (holds clients_lock)

Thread_1043863 (QEMU main loop):
  virtio_gpu_process_cmdq                     virtio-gpu.c:1063
    → helix_scanout_frame_ready               helix-frame-export.m:881
      → pthread_mutex_lock (clients_lock)     ← BLOCKED (VT callback holds it)

Thread_1043872..1043891 (20 vCPU threads):
  hvf_vcpu_exec → bql_lock_impl              ← BLOCKED (main loop holds BQL)
```

**The deadlock chain:**
1. VT callback fires on Apple's `vtencoder-callback-queue` dispatch thread
2. Callback calls `helix_send_to_subscribed_clients`, acquires `clients_lock`
3. Calls `send(fd, data, size, MSG_DONTWAIT)` — **blocks in `__sendto` despite `MSG_DONTWAIT`**
4. macOS kernel does not honor `MSG_DONTWAIT` for sockets in closing/half-open TCP states
5. Main loop calls `helix_scanout_frame_ready`, tries to acquire `clients_lock` → blocked
6. Main loop holds BQL (Big QEMU Lock) → all 20 vCPU threads waiting for BQL → VM frozen

**Why it correlates with WebSocket reconnect:**
When the browser disconnects the WebSocket, the Go desktop-bridge's SharedVideoSource
removes the client. The TCP connection between Go and QEMU may enter a half-close state
(Go stops reading but the connection isn't fully closed yet). The next VT callback tries
to `send()` on this half-closed socket. Despite `MSG_DONTWAIT`, macOS blocks in the kernel.

### Fix Applied

**Primary fix: `O_NONBLOCK` on client sockets via `fcntl()`**

`MSG_DONTWAIT` is a per-call flag that macOS doesn't honor reliably. `O_NONBLOCK` is a
per-fd flag set via `fcntl()` that the kernel enforces at a lower level. With `O_NONBLOCK`,
`send()` truly cannot block — it returns `EAGAIN` immediately.

```c
// At accept time:
int flags = fcntl(client_fd, F_GETFL, 0);
fcntl(client_fd, F_SETFL, flags | O_NONBLOCK);
```

Since the socket is now non-blocking for reads too, `read_exact_bytes()` was updated to
use `poll()` with a 600-second timeout to wait for data.

**Secondary fix: `vt_busy` flag**

Even with `O_NONBLOCK`, the VT callback holds `clients_lock` while iterating clients.
If there are many clients or the iteration is slow, the main loop could stall briefly.
The `vt_busy` flag prevents `VTCompressionSessionEncodeFrame` from being called while
the previous callback is still running — if `EncodeFrame` would block (because VT's
internal queue is full with `MaxFrameDelayCount=0`), we drop the frame instead.

**Additional fixes:**
- `SO_SNDTIMEO` removed (O_NONBLOCK makes it redundant)
- Partial sends → disconnect client (prevents permanent stream desync)
- `g_helix_export` set after all init succeeds (fixes use-after-free on init failure)

### Diagnostic Method

To diagnose a frozen VM, run on the macOS host:

```bash
# Non-destructive — shows all thread backtraces
sample $(pgrep -f "qemu-system-aarch64") 5 -file /tmp/qemu-hang-sample.txt
```

Key things to look for:
- Main loop thread stuck in `pthread_mutex_lock` → lock contention
- Main loop thread stuck in `VTCompressionSessionEncodeFrame` → VT blocking
- VT callback thread stuck in `__sendto` → macOS send() blocking
- `renderer_blocked` non-zero → SPICE gl_block issue (separate problem)

## File Summary

| File | Lines | Change |
|------|-------|--------|
| `hw/display/helix/helix-frame-export.h` | 208 | Removed legacy fields, gl_block_pending |
| `hw/display/helix/helix-frame-export.m` | ~1380 | -1400 lines legacy, +fixes |
| `api/pkg/desktop/scanout_source.go` | - | No changes (already correct) |
| `api/pkg/desktop/shared_video_source.go` | - | No changes (already correct) |
| `api/pkg/desktop/ws_stream.go` | - | No changes (already correct) |
