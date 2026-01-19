# The Decoder Flush Hack That Didn't Work

**Date:** 2026-01-19
**Author:** Luke & Claude
**Status:** Abandoned - simpler solution found

## TL;DR

We tried to fix 1-second typing latency by duplicating video frames to flush Chrome's decoder buffer. After extensive debugging, we discovered the approach doesn't work due to timestamp deduplication in the video pipeline. The actual fix was much simpler: increase minimum framerate from 2 FPS to 30 FPS.

## The Problem

We're building a cloud desktop streaming service. Users connect via browser, we stream their Linux desktop as H.264 video over WebSocket, they see it in real-time. Simple, right?

Except for one infuriating bug: **typing had exactly 1 second of latency**, but only when the screen was static. Move your mouse constantly? Instant. Run `vkcube`? Butter smooth. Stop moving and type? Wait a full second to see your keystrokes.

## The Root Cause

Chrome's WebCodecs VideoDecoder **buffers frames internally**. Even with:
- `optimizeForLatency: true`
- `constraint_set3_flag=1`
- Constrained Baseline profile
- Every setting we could find

The decoder still holds 1-2 frames waiting for potential B-frame reordering, even though we never send B-frames.

At 60 FPS, this is invisible (buffer = 16-32ms). At 2 FPS keepalive, buffer = 500-1000ms.

The latency was always exactly 2x our keepalive interval (500ms keepalive = 1s latency). This was the clue.

## The Failed Hack

We tried to duplicate frames when the frame rate drops to "flush" the decoder buffer:

```rust
// Detect rate drop and send 16 flush frames
if state.frame_repeat_remaining > 0 && state.last_buffer.is_some() {
    let buf = state.last_buffer.as_ref().unwrap().copy();
    state.frame_repeat_remaining -= 1;
    return Ok(CreateSuccess::NewBuffer(buf));
}
```

The theory was sound:
1. H.264 encoders encode identical frames as tiny "skip P-frames"
2. Decoders must output something for each frame
3. 16 flush frames should push any buffered frames through

## Why It Didn't Work

After hours of debugging, we discovered multiple issues:

### 1. PTS (Presentation Timestamp) Deduplication

When we called `GstBuffer::copy()`, we created a shallow copy with the same PTS. GStreamer, nvh264enc, and Chrome's decoder all have logic to drop or merge frames with identical timestamps. Our 16 flush frames were being reduced to 1-2 frames.

We tried incrementing PTS by 1ms per flush frame, but this created other problems - the timestamps could advance ahead of the next real frame, causing decoder issues.

### 2. GPU Memory Pointer Deduplication

In zero-copy CUDA mode, `Buffer::copy()` creates a new GstBuffer but shares the underlying CUDAMemory (same GPU pointer). nvh264enc may detect "same GPU address = already encoded" and skip encoding.

### 3. Queue Dropping

The GStreamer queue element (`max-size-buffers=1 leaky=downstream`) was dropping 15 of our 16 flush frames. When we increased it to 16, we got 100+ FPS during activity (excessive flush triggering).

### 4. Flush Triggering Too Often

Using `target_interval` (16ms for 60 FPS) as the timeout meant we flushed after EVERY frame if the actual frame rate was below 60 FPS. GNOME often delivers 30-50 FPS depending on damage, so we were constantly flushing.

## The Actual Solution

After all this complexity, the fix was embarrassingly simple:

**Increase minimum framerate from 2 FPS (500ms) to 30 FPS (33ms).**

```go
// Before
srcPart := fmt.Sprintf("pipewirezerocopysrc ... keepalive-time=500", ...)

// After
srcPart := fmt.Sprintf("pipewirezerocopysrc ... keepalive-time=33", ...)
```

With 30 FPS minimum:
- Decoder buffer latency = 33ms × 2 = ~66ms (imperceptible)
- No extra complexity in the Rust code
- No timestamp manipulation
- No flush frame logic

The tradeoff is slightly higher bandwidth and CPU usage when the screen is static, but modern hardware handles 30 FPS effortlessly.

## Alternative Solutions We Didn't Try

1. **VideoDecoder.flush()** - Chrome's WebCodecs has a `flush()` API that forces immediate output. We could send a flush signal over WebSocket when we detect a pause. This is the "proper" solution but requires frontend changes.

2. **max_num_reorder_frames=0** - Setting this in the H.264 VUI (Video Usability Information) tells the decoder no reordering is needed. Requires SPS modification.

3. **Constrained Baseline with correct signaling** - Ensuring the full codec string matches the actual stream (profile, level, constraint flags).

## Lessons Learned

1. **Simple solutions first.** We spent hours on a complex hack when increasing framerate would have worked immediately.

2. **Video pipelines have many deduplication layers.** PTS, memory pointers, queue elements - there are many places where "duplicate" frames can be dropped.

3. **GstBuffer::copy() is shallow.** For CUDA memory, the underlying GPU pointer is shared. This matters for encoders that track buffer addresses.

4. **Timestamps must be monotonically increasing.** Both encoders and decoders expect strictly increasing PTS. Duplicates or out-of-order timestamps cause undefined behavior.

5. **Test with instrumentation.** Adding `framesReceived` vs `framesDecoded` counters to the frontend immediately showed us frames were being dropped before the decoder.

## Links

- [WebCodecs Issue #698 - Flushing output queue](https://github.com/w3c/webcodecs/issues/698)
- [WebCodecs Issue #732 - 1-in-1-out decoding](https://github.com/w3c/webcodecs/issues/732)
- [Chromium Issue #40857774 - constraint_set3_flag](https://issues.chromium.org/issues/40857774)

---

*Sometimes the clever hack isn't the answer. The simple, boring solution of "just send more frames" works perfectly.*
