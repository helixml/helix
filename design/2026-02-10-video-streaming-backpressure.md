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

## File Summary

| File | Lines | Change |
|------|-------|--------|
| `hw/display/helix/helix-frame-export.h` | 208 | Removed legacy fields, gl_block_pending |
| `hw/display/helix/helix-frame-export.m` | ~1380 | -1400 lines legacy, +fixes |
| `api/pkg/desktop/scanout_source.go` | - | No changes (already correct) |
| `api/pkg/desktop/shared_video_source.go` | - | No changes (already correct) |
| `api/pkg/desktop/ws_stream.go` | - | No changes (already correct) |
