# The Decoder Flush Hack That Didn't Work

**Date:** 2026-01-19
**Author:** Luke & Claude
**Status:** Abandoned - simpler solution found

## TL;DR

We tried to fix 1-second typing latency by duplicating video frames to flush Chrome's decoder buffer. After extensive debugging, we discovered the approach doesn't work due to multiple layers of deduplication and buffering in the video pipeline. The actual fix was much simpler: increase minimum framerate from 2 FPS to 30 FPS.

## The Problem

We're building a cloud desktop streaming service. Users connect via browser, we stream their Linux desktop as H.264 video over WebSocket, they see it in real-time.

**The bug:** Typing had exactly 1 second of latency, but only when the screen was static. Move your mouse constantly? Instant. Run `vkcube`? Butter smooth. Stop moving and type? Wait a full second to see your keystrokes.

The latency was always exactly 2x our keepalive interval (500ms keepalive = 1s latency). This was the clue.

## The Root Cause

Chrome's WebCodecs VideoDecoder **buffers frames internally**. Even with:
- `optimizeForLatency: true`
- `constraint_set3_flag=1`
- Constrained Baseline profile
- Every setting we could find

The decoder still holds 1-2 frames waiting for potential B-frame reordering, even though we never send B-frames.

At 60 FPS, this is invisible (buffer = 16-32ms). At 2 FPS keepalive, buffer = 500-1000ms.

From [w3c/webcodecs issue #698](https://github.com/w3c/webcodecs/issues/698):
> "WebCodecs as it stands is unsuitable for use with variable rate streams, as you have no guarantees about the depth of the internal pending output queue, and no way to flush it without breaking the stream."

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
1. H.264 encoders encode identical frames as tiny "skip P-frames" (~500 bytes)
2. Decoders must output something for each frame
3. 16 flush frames should push any buffered frames through

**Observation:** We sent 16 flush frames from Rust, but only saw 6-9 FPS spike in the browser (expected 16+). Something was dropping our frames.

## Why It Didn't Work: The Full Analysis

After hours of debugging with instrumentation (framesReceived vs framesDecoded counters), we discovered the approach fails due to multiple interacting factors:

### 1. The Decoder Reorder Buffer is Spec-Defined

From the [H.264 specification](https://community.intel.com/t5/Media-Intel-oneAPI-Video/h-264-decoder-gives-two-frames-latency-while-decoding-a-stream/m-p/1099694), even without B-frames:
> "According to AVC spec, a decoder doesn't have to return a decoded surface immediately for displaying. Even in the absence of B frames - the decoder doesn't know in advance that it won't later encounter B frames, so reordering might still be present."

The decoder's Decoded Picture Buffer (DPB) can hold up to 16 reference frames. The `max_dec_frame_buffering` parameter in the SPS controls this, but Chrome's hardware decoders may ignore it.

### 2. PTS (Presentation Timestamp) Deduplication

When we called `GstBuffer::copy()`, we created a shallow copy with the **same PTS**. Multiple layers of the pipeline deduplicate based on timestamp:

- **GStreamer queues** may drop frames with duplicate PTS
- **nvh264enc** may merge identical PTS frames in its rate control logic
- **Chrome's VideoDecoder** expects monotonically increasing timestamps; same-PTS frames may be silently dropped or merged

We tried incrementing PTS by 1ms per flush frame, but this created a new problem: timestamps advancing 16ms ahead of where the next real frame would be, potentially causing frame reordering issues.

### 3. GPU Memory Pointer Detection

In zero-copy CUDA mode, `Buffer::copy()` creates a new GstBuffer but shares the underlying CUDAMemory (same GPU pointer). nvh264enc may optimize based on buffer address:
- "Already encoded this address" → skip or minimal encoding
- Reference counting confusion → encoder sees same memory
- VBV buffer logic may combine identical content

### 4. Hardware Decoder Opacity

Chrome's hardware-accelerated decoders (NVDEC, VAAPI, D3D11) are opaque. From [w3c/webcodecs discussions](https://github.com/w3c/webcodecs/discussions/680):
> "Hardware decoders are much more aggressive at buffering than software fallbacks."

We have no visibility into what NVDEC is doing with our frames. It may:
- Have its own internal deduplication
- Buffer more than the DPB suggests
- Not call output callbacks for "no-change" frames

### 5. VideoDecoder Output Order

From [w3c/webcodecs issue #55](https://github.com/w3c/webcodecs/issues/55):
> "VideoDecoder calls VideoDecoderOutputCallback as soon as a video frame decoding has finished, i.e. it gives VideoFrames in decoding order, not presentation order."

If our flush frames had the same or very close PTS values, Chrome may have been:
- Outputting them in an unexpected order
- Merging them into single output callbacks
- Dropping them as "duplicate presentation time"

### 6. The Queue Element

The GStreamer queue (`max-size-buffers=1 leaky=downstream`) was dropping 15 of our 16 flush frames because they arrived faster than nvenc could consume them. When we increased to 16 buffers, we got 100+ FPS during activity (excessive flush triggering on every frame).

### 7. Flush Triggering Logic

Using `target_interval` (16ms for 60 FPS) as the timeout meant we flushed after EVERY frame if the actual frame rate was below 60 FPS. GNOME often delivers 30-50 FPS depending on screen damage, so we were constantly flushing, creating more problems than we solved.

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

1. **VideoDecoder.flush()** - Chrome's WebCodecs has a `flush()` API that forces immediate output. We could send a flush signal over WebSocket when we detect a pause. However, [flush invalidates the decode pipeline](https://github.com/w3c/webcodecs/issues/698), requiring the next frame to be a keyframe.

2. **max_num_reorder_frames=0 in SPS** - Setting this in the H.264 VUI (Video Usability Information) tells the decoder no reordering is needed. However, hardware decoders may ignore this.

3. **Constrained Baseline with correct signaling** - Ensuring the full codec string matches the actual stream (profile, level, constraint flags).

4. **Async_Depth=1 + DecodedOrder** - For Intel Media SDK, this forces immediate output. No equivalent for Chrome's WebCodecs.

5. **Software decoder fallback** - Force Chrome to use FFmpeg instead of hardware decoder. Would have lower latency but higher CPU usage.

## Hypothesis: What's Really Happening

Based on our research, here's our best hypothesis for why the flush hack failed:

```
Our Pipeline:
┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│ PipeWire    │──▶│ GStreamer   │──▶│ nvh264enc   │──▶│ WebSocket   │──▶│ WebCodecs   │
│ ScreenCast  │   │ Queue       │   │ (NVENC HW)  │   │             │   │ VideoDecoder│
└─────────────┘   └─────────────┘   └─────────────┘   └─────────────┘   └─────────────┘
                        │                  │                                   │
                        ▼                  ▼                                   ▼
                 Drops 15 of 16     May dedupe same     Hardware decoder buffers
                 due to leaky       GPU address or      1-2 frames regardless
                 queue config       same PTS frames     of our attempts
```

**The fundamental problem:** We're trying to trick a hardware decoder into outputting frames immediately by flooding it with duplicates. But hardware decoders are designed to be efficient - they detect and optimize identical content at multiple levels:

1. **Encoder level:** NVENC's rate control sees identical pixels and produces minimal P-frames
2. **Decoder level:** NVDEC may not trigger output callbacks for frames that decode to identical content
3. **Browser level:** Chrome's VideoDecoder may merge same-PTS frames or drop duplicates

The only way to guarantee low-latency output from a hardware decoder is to **give it enough real work to do** - i.e., maintain a high enough frame rate that the buffer latency becomes imperceptible.

## Lessons Learned

1. **Simple solutions first.** We spent hours on a complex hack when increasing framerate would have worked immediately.

2. **Video pipelines have many deduplication layers.** PTS, memory pointers, queue elements, encoder rate control, decoder DPB - there are many places where "duplicate" frames can be optimized away.

3. **Hardware decoders are opaque.** You can't control or even observe what's happening inside NVDEC/VAAPI/D3D11. The WebCodecs API provides no visibility into internal buffering.

4. **The WebCodecs "flush" problem is unsolved.** There's an open issue from 2022 requesting `forceImmediateOutput`. Until Chrome implements this, variable-rate streams will always have latency spikes when framerate drops.

5. **GstBuffer::copy() is shallow for GPU memory.** For CUDA memory, the underlying GPU pointer is shared. Encoders may detect this.

6. **Test with instrumentation.** Adding `framesReceived` vs `framesDecoded` counters to the frontend immediately showed us frames were being dropped before reaching the decoder.

7. **Timestamps must be monotonically increasing.** Both encoders and decoders expect strictly increasing PTS. Duplicates or out-of-order timestamps cause undefined behavior.

## Links

- [WebCodecs Issue #698 - Flushing output queue WITHOUT invalidating pipeline](https://github.com/w3c/webcodecs/issues/698)
- [WebCodecs Issue #55 - Output order vs presentation order](https://github.com/w3c/webcodecs/issues/55)
- [Intel Community - H.264 decoder gives two frames latency](https://community.intel.com/t5/Media-Intel-oneAPI-Video/h-264-decoder-gives-two-frames-latency-while-decoding-a-stream/m-p/1099694)
- [AVBlocks - Controlling H.264 decoding latency](http://blog.avblocks.com/controling-h-264-decoding-latency)
- [Chrome WebCodecs best practices](https://developer.chrome.com/docs/web-platform/best-practices/webcodecs)
- [NVIDIA Blog - Optimizing Video Memory with NVDEC](https://developer.nvidia.com/blog/optimizing-video-memory-usage-with-the-nvdecode-api-and-nvidia-video-codec-sdk/)

---

*Sometimes the clever hack isn't the answer. Hardware video pipelines are designed to be efficient, which means they actively resist attempts to send "useless" duplicate frames. The simple, boring solution of "just send more frames" works because it gives the pipeline real work to do.*
