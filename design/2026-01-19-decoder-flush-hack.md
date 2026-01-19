# Hardware Decoder Latency: The Real Fix

**Date:** 2026-01-19
**Author:** Luke & Claude
**Status:** SOLVED - Software decoding eliminates latency

## TL;DR

We had 1-second typing latency on static screens. After trying complex flush hacks, we discovered the real cause: **hardware decoders buffer frames for B-frame reordering, even when the stream has no B-frames**. The fix is simple: `hardwareAcceleration: "prefer-software"` in WebCodecs config. Software decoding is 1-in-1-out with no buffering.

## The Problem

We're building a cloud desktop streaming service. Users connect via browser, we stream their Linux desktop as H.264 video over WebSocket, they see it in real-time.

**The bug:** Typing had exactly 1 second of latency, but only when the screen was static. Move your mouse constantly? Instant. Run `vkcube`? Butter smooth. Stop moving and type? Wait a full second to see your keystrokes.

The latency was always exactly 2x our keepalive interval (500ms keepalive = 1s latency). This was the clue.

## Root Cause: Hardware Decoder Reorder Buffer

Chrome's WebCodecs VideoDecoder with hardware acceleration buffers frames internally. Even with:
- `optimizeForLatency: true`
- `constraint_set3_flag=1`
- Constrained Baseline profile
- Every setting we could find

The hardware decoder still holds 1-4 frames waiting for potential B-frame reordering, even though we never send B-frames.

From [w3c/webcodecs issue #732](https://github.com/w3c/webcodecs/issues/732):
> "An example using the BASELINE profile works perfectly with 1-in-1-out decoding, but two other examples using the MAIN profile do not have 1-in-1-out behavior. These MAIN profile streams require filling up the decode queue with 4 frames after each keyframe before returning the first frame passed."

**Critical insight:** Setting `hardwareAcceleration: "prefer-software"` removes this delay completely.

## The Failed Hack: Frame Duplication

Before discovering the real fix, we tried to "flush" the decoder buffer by duplicating frames:

```rust
// Detect rate drop and send 16 flush frames
if state.frame_repeat_remaining > 0 && state.last_buffer.is_some() {
    let buf = state.last_buffer.as_ref().unwrap().copy();
    state.frame_repeat_remaining -= 1;
    return Ok(CreateSuccess::NewBuffer(buf));
}
```

This failed for multiple reasons:
1. GStreamer queue dropped duplicate frames
2. PTS deduplication at multiple layers
3. GPU memory pointer detection by encoder
4. Hardware decoder opacity - no visibility into internal buffering

See "Why The Flush Hack Failed" section below for full analysis.

## The Real Solution: Software Decoding

```typescript
// Before - hardware decoding with 1-4 frame buffer
const config: VideoDecoderConfig = {
  codec: codecString,
  hardwareAcceleration: "prefer-hardware",  // BUFFERED!
  optimizeForLatency: true,
}

// After - software decoding, 1-in-1-out
const config: VideoDecoderConfig = {
  codec: codecString,
  hardwareAcceleration: "prefer-software",  // INSTANT!
  optimizeForLatency: true,
}
```

**Testing:** Add `?softdecode=1` to the URL to force software decoding. The latency disappears completely.

With software decoding:
- True 1-in-1-out behavior
- No reorder buffer
- Works with damage-based rendering (2 FPS keepalive)
- Slight CPU increase (acceptable tradeoff)

## Interim Workaround: 30 FPS Minimum

Before discovering the software decoding fix, we worked around the issue by increasing minimum framerate:

```go
// Before - 500ms keepalive = 1s latency with 2-frame buffer
srcPart := fmt.Sprintf("pipewirezerocopysrc ... keepalive-time=500", ...)

// After - 33ms keepalive = 66ms latency with 2-frame buffer (imperceptible)
srcPart := fmt.Sprintf("pipewirezerocopysrc ... keepalive-time=33", ...)
```

This works but wastes bandwidth/CPU when the screen is static. With software decoding, we can revert to true damage-based rendering.

## Fix for Hardware Decoding: VUI Rewriting

Software decoding works, but hardware decoding is more efficient. We implemented VUI rewriting to fix hardware decoding latency.

### What We Implemented

1. **Changed encoder profile caps to `constrained-baseline`** (was `main`)
   - Updated qsv, vaapi, vaapi-lp h264parse caps
   - This tells the encoder to output Baseline profile

2. **Patching `constraint_set3_flag=1` in SPS**
   - This signals no B-frames/no reordering to decoder
   - Done in `patchSPSForZeroLatencyDecode()`

3. **Already using `ref-frames=1` on most encoders**
   - qsv: `ref-frames=1`
   - vaapi: `ref-frames=1`
   - vaapi-lp: `ref-frames=1`
   - x264: `ref=1`
   - nvenc: does NOT expose ref-frames property (checked via gst-inspect)

4. **NEW: VUI Rewriting in Go** (`api/pkg/desktop/h264_sps.go`)
   - Uses mp4ff library for H.264 SPS parsing and writing
   - Rewrites the VUI `bitstream_restriction` section with:
     - `max_num_reorder_frames=0` (no frame reordering)
     - `max_dec_frame_buffering=1` (or max_num_ref_frames if higher)
   - Automatically adds VUI if not present
   - Uses EBSPWriter for proper Exp-Golomb encoding and emulation prevention bytes

### VUI Rewriting Implementation

The key parameters that tell hardware decoders to output immediately (matching [WebRTC's sps_vui_rewriter.cc](https://webrtc.googlesource.com/src/+/refs/heads/main/common_video/h264/sps_vui_rewriter.cc#400)):

```go
// CRITICAL: These are the key fields for zero-latency decode
// Reference: WebRTC sps_vui_rewriter.cc line 400
w.WriteExpGolomb(0) // max_num_reorder_frames = 0 (no frame reordering)

// max_dec_frame_buffering = max_num_ref_frames (per WebRTC and H.264 spec)
// The key insight is that max_num_reorder_frames=0 is what eliminates buffering.
maxDecBuf := sps.NumRefFrames
if maxDecBuf == 0 {
    maxDecBuf = 1
}
w.WriteExpGolomb(maxDecBuf)
```

**Key insight from WebRTC**: `max_num_reorder_frames=0` is what eliminates decoder output buffering. `max_dec_frame_buffering` just needs to satisfy the spec requirement of being >= `max_num_ref_frames`.

The implementation:
- `RewriteSPSForZeroLatency()` - parses SPS, rewrites VUI, returns new SPS
- `patchSPSForZeroLatencyDecode()` - processes Annex B stream, rewrites all SPS NAL units
- Uses mp4ff's `EBSPWriter` which handles emulation prevention bytes automatically

### Testing Hardware Decoding

Use `?hwdecode=1` URL parameter to test hardware decoding:
- If latency is fixed: VUI rewriting is working correctly
- If latency persists: check logs for "H.264 SPS after VUI rewrite" to verify patching

## Why The Flush Hack Failed: Full Analysis

### 1. The Decoder Reorder Buffer is Spec-Defined

From the [H.264 specification](https://community.intel.com/t5/Media-Intel-oneAPI-Video/h-264-decoder-gives-two-frames-latency-while-decoding-a-stream/m-p/1099694):
> "According to AVC spec, a decoder doesn't have to return a decoded surface immediately for displaying. Even in the absence of B frames - the decoder doesn't know in advance that it won't later encounter B frames, so reordering might still be present."

### 2. PTS Deduplication

When we called `GstBuffer::copy()`, we created a shallow copy with the **same PTS**. Multiple layers deduplicate based on timestamp:
- GStreamer queues may drop frames with duplicate PTS
- nvh264enc may merge identical PTS frames
- Chrome's VideoDecoder may silently drop same-PTS frames

### 3. GPU Memory Pointer Detection

In zero-copy CUDA mode, `Buffer::copy()` shares the underlying CUDAMemory. nvh264enc may optimize:
- "Already encoded this address" → skip or minimal encoding
- VBV buffer logic may combine identical content

### 4. Hardware Decoder Opacity

Chrome's hardware decoders (VideoToolbox, NVDEC, VAAPI) are opaque. From [w3c/webcodecs discussions](https://github.com/w3c/webcodecs/discussions/680):
> "Hardware decoders are much more aggressive at buffering than software fallbacks."

### 5. Queue Element Drops

The GStreamer queue (`max-size-buffers=1 leaky=downstream`) dropped 15 of 16 flush frames because they arrived faster than nvenc could consume them.

## Lessons Learned

1. **Hardware decoders buffer aggressively.** Even with `optimizeForLatency: true`, they buffer 1-4 frames for B-frame reordering.

2. **Software decoding is 1-in-1-out.** No reorder buffer, immediate output.

3. **Profile matters.** BASELINE profile may work 1-in-1-out on hardware; MAIN profile definitely buffers.

4. **Test the hypothesis directly.** Adding `?softdecode=1` immediately proved the hardware decoder was the problem.

5. **Video pipelines have many deduplication layers.** Trying to "trick" them with duplicate frames fails at multiple levels.

6. **The WebCodecs "flush" problem is unsolved.** There's an open issue from 2022 requesting `forceImmediateOutput`. Until Chrome implements this, variable-rate streams need software decoding for low latency.

## Links

- [WebCodecs Issue #732 - 1-in-1-out decoding for H.264](https://github.com/w3c/webcodecs/issues/732) - **KEY ISSUE**
- [WebCodecs Issue #698 - Flushing output queue WITHOUT invalidating pipeline](https://github.com/w3c/webcodecs/issues/698)
- [WebCodecs Issue #55 - Output order vs presentation order](https://github.com/w3c/webcodecs/issues/55)
- [WebCodecs Discussion #680 - VideoDecoder stalls](https://github.com/w3c/webcodecs/discussions/680)
- [Intel Community - H.264 decoder gives two frames latency](https://community.intel.com/t5/Media-Intel-oneAPI-Video/h-264-decoder-gives-two-frames-latency-while-decoding-a-stream/m-p/1099694)
- [AVBlocks - Controlling H.264 decoding latency](http://blog.avblocks.com/controling-h-264-decoding-latency)

---

*The fix was hiding in plain sight: hardware decoders buffer, software decoders don't. Sometimes the "less efficient" option is actually correct.*
