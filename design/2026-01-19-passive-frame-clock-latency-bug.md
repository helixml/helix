# Mutter PASSIVE Frame Clock Latency Bug Investigation

**Date:** 2026-01-19
**Status:** Investigating
**Issue:** 1-second typing latency on static screens with 500ms keepalive

## Problem Statement

When using GNOME headless with `pipewirezerocopysrc` and `keepalive-time=500` (2 FPS floor on static screens), typing latency is exactly **1 second** - precisely 2x the keepalive interval. This delay occurs even though:

1. We poll PipeWire every 1ms
2. We return buffers immediately after CUDA copy
3. We disable Mutter's frame rate limiter by negotiating `max_framerate=0/1`

With vkcube running at 60 FPS, typing is instantaneous. The issue only manifests when the screen is static.

## Architecture Overview

### Components

```
┌─────────────────────────────────────────────────────────────────────────┐
│ GNOME/Mutter (headless)                                                  │
│                                                                          │
│  ┌─────────────────────┐     ┌─────────────────────────────────────┐   │
│  │ ClutterFrameClock   │     │ MetaScreenCastVirtualStreamSrc      │   │
│  │ (PASSIVE mode)      │────▶│ (ScreenCast driver)                  │   │
│  └─────────────────────┘     └─────────────────────────────────────┘   │
│          │                              │                               │
│          │ damage                       │ dispatch()                    │
│          ▼                              ▼                               │
│  ┌─────────────────────┐     ┌─────────────────────────────────────┐   │
│  │ Wayland surfaces    │     │ pw_stream (PRODUCER, non-driving)    │   │
│  │ (terminal, apps)    │     │ pending_process flag                 │   │
│  └─────────────────────┘     └─────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
                                          │
                                          │ PipeWire
                                          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ pipewirezerocopysrc (CONSUMER)                                           │
│                                                                          │
│  ┌─────────────────────┐     ┌─────────────────────────────────────┐   │
│  │ PipeWire thread     │     │ GStreamer thread                     │   │
│  │ (1ms poll interval) │────▶│ recv_frame_timeout(500ms)            │   │
│  │                     │     │                                      │   │
│  │ process callback:   │     │ Keepalive: resend last buffer        │   │
│  │ 1. dequeue_raw      │     │            when timeout              │   │
│  │ 2. CUDA copy        │     │                                      │   │
│  │ 3. try_send(channel)│     │                                      │   │
│  │ 4. queue_raw (back) │     │                                      │   │
│  └─────────────────────┘     └─────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

### Frame Flow (Normal Case - 60 FPS with vkcube)

1. vkcube constantly damages screen
2. Mutter's frame clock is continuously cycling (DISPATCHED_ONE ↔ IDLE)
3. Frames flow at 60 FPS
4. Terminal typing piggybacks on this frame rhythm
5. **Latency: ~16ms (one frame)**

### Frame Flow (Problem Case - Static Screen with 500ms Keepalive)

1. **t=0:** GStreamer calls `create()`
2. **t=0:** We call `recv_frame_timeout(500ms)`
3. **t=0-500ms:** Blocking wait on channel (no frames from Mutter, screen static)
4. **t=500ms:** Timeout fires, we resend last buffer as keepalive
5. **t=500ms:** GStreamer calls `create()` again
6. **t=500ms:** We call `recv_frame_timeout(500ms)` again
7. **t=700ms:** User presses key
8. **t=700ms:** Terminal damages Wayland surface
9. **t=700ms:** Mutter receives damage notification
10. **t=700ms:** `clutter_stage_view_schedule_update()` called
11. **t=700ms:** Frame clock (PASSIVE) calls `driver->schedule_update()`
12. **t=700ms:** `meta_screen_cast_stream_src_request_process()` called
13. **??? DELAY HAPPENS HERE ???**
14. **t=1000ms:** Timeout fires, we resend keepalive
15. **t=1200ms:** Eventually frame arrives
16. **Latency: ~1000ms (2x keepalive)**

## Mutter's PASSIVE Frame Clock Mode

For virtual monitors (headless), Mutter uses `CLUTTER_FRAME_CLOCK_MODE_PASSIVE`. In this mode:

- Frame clock has no internal timer
- External driver (ScreenCast) controls when frames are painted
- `schedule_update()` delegates to `driver->schedule_update()` and returns immediately
- No GSource ready time is set (unlike FIXED/VARIABLE modes)

### Key Code Paths

**clutter-frame-clock.c:1208-1210**
```c
case CLUTTER_FRAME_CLOCK_MODE_PASSIVE:
  clutter_frame_clock_driver_schedule_update (frame_clock->driver);
  return;
```

**meta-screen-cast-virtual-stream-src.c:810-817**
```c
static void
meta_screen_cast_frame_clock_driver_schedule_update (ClutterFrameClockDriver *driver)
{
  MetaScreenCastFrameClockDriver *driver = ...;
  if (driver->src)
    meta_screen_cast_stream_src_request_process (driver->src);
}
```

**meta-screen-cast-stream-src.c:969-983**
```c
void
meta_screen_cast_stream_src_request_process (MetaScreenCastStreamSrc *src)
{
  if (!priv->pending_process &&
      !pw_stream_is_driving (priv->pipewire_stream))
    {
      pw_stream_trigger_process (priv->pipewire_stream);
      priv->pending_process = TRUE;  // ← BLOCKS SUBSEQUENT TRIGGERS
    }
}
```

## The `pending_process` Gatekeeper

**Critical Observation:** If `pending_process` is TRUE, all subsequent damage events are IGNORED until `on_stream_process` fires.

`pending_process` is set to TRUE in `request_process()` and only cleared in `on_stream_process()`:

```c
static void
on_stream_process (void *user_data)
{
  priv->pending_process = FALSE;  // ← Only cleared here
  klass->dispatch (src);
}
```

### What Triggers on_stream_process?

For Mutter's non-driving pw_stream:
1. `pw_stream_trigger_process()` signals PipeWire
2. PipeWire schedules `on_stream_process` callback
3. **Mutter's GSource must dispatch** (polls PipeWire loop FD)
4. `on_stream_process` fires

For `on_stream_process` to fire, Mutter needs:
- An available buffer in its pool (to fill with painted frame)
- The PipeWire GSource to dispatch

## Buffer Pool Dynamics

Mutter allocates 2-16 DMA-BUF buffers for the pw_stream:
```c
SPA_PARAM_BUFFERS_buffers, SPA_POD_CHOICE_RANGE_Int (16, 2, 16),
```

Buffer lifecycle:
1. Mutter: `dequeue_pw_buffer()` - gets empty buffer from pool
2. Mutter: `do_record_frame()` - paints into buffer (GPU blit)
3. Mutter: `pw_stream_queue_buffer()` - sends to PipeWire
4. PipeWire: Delivers to consumer (us)
5. Us: `dequeue_raw_buffer()` - receives filled buffer
6. Us: CUDA copy (synchronous)
7. Us: `queue_raw_buffer()` - returns buffer to pool
8. Mutter: Buffer available again for step 1

**Key:** If we don't return buffers (step 7), Mutter can't produce new frames.

## Hypothesis: Why 2x Keepalive Delay?

### Theory A: Buffer Starvation (Unlikely)

We return buffers immediately after CUDA copy (~1ms). With 4+ buffers, Mutter should never starve.

### Theory B: PipeWire Wake-up Latency

When the screen is static:
1. Mutter's main loop is idle (no vsync, no input, no timers)
2. GLib main loop may be in power-saving mode, polling infrequently
3. When we return a buffer, PipeWire notifies Mutter's loop FD
4. But Mutter's GSource may not dispatch immediately

This could add latency but doesn't explain the EXACT 2x keepalive timing.

### Theory C: Mutter's pending_process Getting Stuck

Something causes `pending_process` to remain TRUE across multiple keepalive cycles:

1. Damage occurs
2. `request_process()` triggers PipeWire, sets `pending_process = TRUE`
3. PipeWire should call `on_stream_process`, but doesn't
4. Subsequent damage events are IGNORED
5. After 2 keepalive cycles (~1000ms), something clears the blockage
6. Frame finally delivers

**Possible cause:** PipeWire stream state issue when no frames have flowed for a while?

### Theory D: Keepalive Interaction (New Hypothesis)

When we're in keepalive mode (resending cached frame):
1. GStreamer calls `create()`, we timeout and return cached buffer
2. We're NOT consuming from PipeWire channel
3. BUT our PipeWire thread IS running (1ms poll)
4. Should still receive and queue frames...

**Wait:** During keepalive timeout, `recv_frame_timeout` blocks on the channel. If a frame arrives, it should wake up immediately.

But maybe the frame ISN'T arriving because:
- Mutter isn't producing (pending_process stuck?)
- PipeWire isn't delivering (stream stalled?)
- Our process callback isn't firing (????)

## Testing Observations

| Keepalive | vkcube Running | Typing Latency |
|-----------|----------------|----------------|
| 33ms      | No             | ~50-100ms      |
| 500ms     | No             | ~1000ms        |
| 500ms     | Yes (60 FPS)   | ~16ms          |
| 33ms      | Yes (60 FPS)   | ~16ms          |

**Key insight:** With vkcube driving 60 FPS, everything works. The issue only manifests when frame production drops to the keepalive floor.

## Files Involved

### Mutter (GNOME)
- `mutter/clutter/clutter/clutter-frame-clock.c` - Frame clock state machine
- `mutter/src/backends/meta-screen-cast-virtual-stream-src.c` - Virtual monitor ScreenCast
- `mutter/src/backends/meta-screen-cast-stream-src.c` - PipeWire stream handling

### Helix (our code)
- `desktop/gst-pipewire-zerocopy/src/pipewire_stream.rs` - PipeWire consumer thread
- `desktop/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs` - GStreamer element
- `api/pkg/desktop/ws_stream.go` - Pipeline configuration

## Current Workaround

Use high keepalive (33ms / 30 FPS) to keep the frame clock "warm". This wastes bandwidth and GPU resources on static screens but avoids the latency bug.

## Next Steps

1. Add detailed logging to trace exact timing of:
   - When damage occurs in Mutter
   - When request_process() is called
   - When on_stream_process fires
   - When we receive the buffer
   - When we return the buffer

2. Investigate whether PipeWire stream goes into an idle/paused state that delays wake-up

3. Check if Mutter's GLib main loop has any idle detection that reduces poll frequency

4. Consider alternative: Instead of PASSIVE mode, could we drive the frame clock ourselves?

## Full Frame Pipeline: Mutter → GStreamer → WebSocket → Browser

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ MUTTER                                                                       │
│                                                                              │
│  damage → frame_clock → dispatch() → paint → record_to_buffer               │
│                                                ↓                             │
│                                         pw_stream_queue_buffer               │
└─────────────────────────────────────────────────────────────────────────────┘
                                                ↓ PipeWire
┌─────────────────────────────────────────────────────────────────────────────┐
│ PIPEWIREZEROCOPYSRC (PipeWire thread)                                        │
│                                                                              │
│  process callback:                                                           │
│    1. dequeue_raw_buffer()     ← Get filled buffer from Mutter              │
│    2. extract_frame()          ← Parse DmaBuf from SPA buffer               │
│    3. process_dmabuf_to_cuda() ← EGL import + CUDA copy (sync)              │
│    4. try_send(channel)        ← Send to GStreamer thread                   │
│    5. queue_raw_buffer()       ← Return buffer to Mutter                    │
└─────────────────────────────────────────────────────────────────────────────┘
                                                ↓ mpsc channel (buffer=8)
┌─────────────────────────────────────────────────────────────────────────────┐
│ PIPEWIREZEROCOPYSRC (GStreamer thread, in create())                          │
│                                                                              │
│  recv_frame_timeout(500ms):                                                  │
│    - Block until frame in channel OR timeout                                 │
│    - On timeout: return copy of last_buffer (keepalive)                      │
│    - On receive: return new frame as GstBuffer                              │
│                                                                              │
│  Returns: CreateSuccess::NewBuffer(buffer)                                   │
└─────────────────────────────────────────────────────────────────────────────┘
                                                ↓ GstBuffer (CUDAMemory)
┌─────────────────────────────────────────────────────────────────────────────┐
│ GSTREAMER PIPELINE                                                           │
│                                                                              │
│  pipewirezerocopysrc                                                         │
│       ↓                                                                      │
│  queue max-size-buffers=1 leaky=downstream   ← Drops old frames if backup   │
│       ↓                                                                      │
│  nvh264enc preset=low-latency-hq zerolatency=true                           │
│       ↓                                                                      │
│  h264parse                                                                   │
│       ↓                                                                      │
│  appsink                                                                     │
└─────────────────────────────────────────────────────────────────────────────┘
                                                ↓ H.264 NAL units
┌─────────────────────────────────────────────────────────────────────────────┐
│ GO WEBSOCKET HANDLER                                                         │
│                                                                              │
│  appsink callback → WebSocket send                                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                                ↓ WebSocket binary frames
┌─────────────────────────────────────────────────────────────────────────────┐
│ BROWSER (JavaScript)                                                         │
│                                                                              │
│  WebSocket.onmessage                                                         │
│       ↓                                                                      │
│  Decode queue (ManagedMediaSource / WebCodecs)                              │
│       ↓                                                                      │
│  Video element / Canvas render                                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

## GStreamer Behavior at Low Frame Rates

### The n-1 Frame Problem

H.264 encoders and decoders often require multiple frames in the pipeline before output appears. This is due to:

1. **B-frames**: Reference future frames (we use `bframes=0` to disable)
2. **Encoder lookahead**: Some presets buffer frames for quality
3. **Decoder reordering buffer**: Handles out-of-order decoding

With `zerolatency=true` and `bframes=0`, nvh264enc should NOT buffer frames. But the decoder might still wait for additional frames before displaying.

### Observed Jitter

From browser console:
```
Receive Jitter: 0-541 ms (avg 401ms) 2-4fps
Render Jitter: 0-1989 ms (avg 401ms)
```

**Key observation:** Render jitter (0-1989ms) is MUCH worse than receive jitter (0-541ms).

This means:
- Frames arrive at ~500ms intervals (expected with 500ms keepalive)
- But RENDERING is delayed up to 2 seconds

### Where Is The Extra Buffering?

1. **Browser decode queue**: Video decoders often buffer frames for smooth playback
2. **ManagedMediaSource**: May have internal buffering before playback
3. **Video element**: May wait for certain buffer level before displaying

### Why It's Worse at Low Frame Rates

At 60 FPS:
- Frames every ~16ms
- Any decode buffer fills quickly
- Latency amortized across many frames

At 2 FPS (500ms keepalive):
- Frames every ~500ms
- Decode buffer takes AGES to fill
- If decoder waits for N frames, latency = N * 500ms

**Example:** If decoder waits for 2 frames:
- At 60 FPS: 2 * 16ms = 32ms latency
- At 2 FPS: 2 * 500ms = 1000ms latency

This matches the observed ~1 second latency!

## GStreamer Push Source Behavior

`pipewirezerocopysrc` is a **push source** (BaseSrc subclass). GStreamer's flow:

1. Pipeline starts
2. GStreamer calls `create()` on source
3. Source returns buffer
4. GStreamer pushes buffer downstream
5. GOTO step 2

**Critical:** GStreamer only calls `create()` after the previous buffer flows through. If downstream blocks (encoder full, sink not pulling), `create()` isn't called.

### Impact at Low Frame Rates

At 60 FPS:
- `create()` returns every ~16ms
- Pipeline stays saturated
- Any frame in channel is consumed immediately

At 2 FPS with keepalive:
- `create()` blocks for 500ms (timeout)
- Returns keepalive frame
- GStreamer calls `create()` again
- Blocks for another 500ms
- ...

**Key:** During the 500ms block, if a REAL damage frame arrives in the channel, `recv_frame_timeout` should wake up immediately and return it.

But if it's NOT waking up, frames queue in the channel until the next poll.

## Fixes Applied

### 1. WebCodecs optimizeForLatency (frontend)

Added `optimizeForLatency: true` to VideoDecoderConfig in `websocket-stream.ts:798`.

This tells the browser's WebCodecs VideoDecoder to prioritize latency over throughput.
Without it, the decoder's internal output queue holds frames waiting for the next input.
At 2 FPS, even a queue depth of 2 means 1000ms latency!

See: https://github.com/w3c/webcodecs/issues/698

### 2. Replace std::sync::mpsc with crossbeam-channel (Rust)

Changed from `std::sync::mpsc::sync_channel` to `crossbeam_channel::bounded` in both
`pipewire_stream.rs` and `ext_image_copy_capture.rs`.

Rust's std mpsc has a known race condition (https://github.com/rust-lang/rust/issues/94518):
When `recv_timeout` wakes up due to timeout, there's a window where a message sent during
the timeout processing can be missed until the next `recv` call.

This could explain the exact 2x keepalive delay - the race happens at the 500ms timeout,
and the frame isn't consumed until the NEXT timeout at 1000ms.

crossbeam-channel doesn't have these race conditions.

## THE FIX: Decoder Flush Hack

**Status: SOLVED**

See: `design/2026-01-19-decoder-flush-hack.md`

The root cause was Chrome's WebCodecs VideoDecoder buffering 1-2 frames internally, even with
`optimizeForLatency: true` and `constraint_set3_flag=1`. At low frame rates, this buffer
caused the exact latency we observed (buffer_depth × frame_interval = 2 × 500ms = 1s).

The fix: When frame rate drops (detected by inter-frame interval), rapid-fire 4 copies of the
last frame to flush the decoder buffer. The encoder encodes these as tiny skip P-frames
(identical pixels = no residual data). The decoder outputs them, flushing the real frame.

Changes:
1. `pipewiresrc/imp.rs`: Added adaptive frame repeat logic - track inter-frame timing, on rate drop send 4 copies
2. `ws_stream.go`: Added `bframes=0` explicitly to nvh264enc, patched SPS for `constraint_set3_flag=1`
3. `codecs.ts`: Fixed codec string to match actual encoder output (Constrained Baseline)
4. `websocket-stream.ts`: Added decode latency tracking for debugging

## Related

- `design/2026-01-19-decoder-flush-hack.md` - **THE FIX** (Hacker News style writeup)
- `design/2026-01-19-gnome-headless-typing-latency.md` - Initial investigation
- `design/2026-01-11-mutter-damage-based-frame-pacing.md` - Frame pacing analysis
- `design/2026-01-06-pipewire-keepalive-mechanism.md` - Keepalive implementation
