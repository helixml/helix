# The One Second Latency Bug: A Video Streaming Debugging Story

**Date:** 2026-01-19
**Author:** Luke

I spent today chasing down one of those bugs that makes you question everything you thought you knew about video encoding. Here's the story.

## The Setup

We're building a cloud desktop streaming service. Think of it like a remote desktop, but running in the browser. We stream the Linux desktop as H.264 video over WebSocket, users see it in real-time. Simple enough concept, tricky to get the latency right.

We use damage-based rendering - when nothing on the screen changes, we only send keepalive frames every 500ms. This saves bandwidth and CPU. Run `vkcube` or scroll through code? 60 FPS. Static terminal? 2 FPS. Makes sense.

## The Bug

Typing had exactly one second of latency. Not "about a second" - precisely 1000ms. But only when the screen was static. Move your mouse constantly? Instant feedback. Run a spinning cube? Butter smooth. Stop moving and start typing? Wait a full second to see your keystrokes appear.

The latency was always exactly 2x our keepalive interval. 500ms keepalive = 1 second latency. Change keepalive to 250ms? Get 500ms latency. This was the clue that eventually led us to the answer.

## First Theory: Pipeline Buffering

Obviously something was buffering two frames, right? My first thought was the GStreamer pipeline. We've got queues in there, maybe they were holding onto frames. Spent a few hours adding debug logging, checking buffer levels. Nothing. Frames were flying through instantly.

## Second Theory: The Flush Hack

OK, so if frames are getting stuck in a buffer somewhere, let's flush it. When we detect the frame rate dropping (screen going static), we'll duplicate the last frame a bunch of times. Send 16 copies - that should push any buffered frames through.

```rust
// Detect rate drop and send 16 flush frames
if state.frame_repeat_remaining > 0 && state.last_buffer.is_some() {
    let buf = state.last_buffer.as_ref().unwrap().copy();
    state.frame_repeat_remaining -= 1;
    return Ok(CreateSuccess::NewBuffer(buf));
}
```

This failed spectacularly. Turns out video pipelines really don't like duplicate frames:
- GStreamer queues dropped them (same PTS)
- The encoder noticed the same GPU memory pointer
- Even if they got through, Chrome's decoder deduplicated them

Video pipelines have deduplication at every layer. They've seen this trick before.

## Third Theory: 30 FPS Minimum

At this point I'm getting desperate. Fine, let's just always send at least 30 FPS. If the screen is static, send the same frame 30 times per second. Brute force it.

```go
srcPart := fmt.Sprintf("pipewirezerocopysrc ... keepalive-time=33", ...)
```

This works! Latency gone. But now we're wasting bandwidth and CPU encoding identical frames. Not a real solution.

## The Breakthrough: Software Decoding

I'm reading through [WebCodecs issue #732](https://github.com/w3c/webcodecs/issues/732) for the fifth time and I notice this comment:

> "An example using the BASELINE profile works perfectly with 1-in-1-out decoding, but two other examples using the MAIN profile do not have 1-in-1-out behavior."

Wait. What if it's not our encoder that's buffering, but Chrome's decoder?

I add a URL parameter to force software decoding:

```typescript
const config: VideoDecoderConfig = {
  codec: codecString,
  hardwareAcceleration: "prefer-software",  // <-- THIS
  optimizeForLatency: true,
}
```

Add `?softdecode=1` to the URL. Reload. Type.

Holy shit. Instant. Zero latency. The bug is gone.

## What's Actually Happening

Chrome's hardware video decoder (VideoToolbox on Mac, NVDEC on Windows/Linux with NVIDIA, VAAPI on Linux) buffers frames internally. The H.264 spec allows B-frames to be encoded out of order, so decoders hold onto frames in case they need to be reordered before display.

The thing is, we never send B-frames. Our stream is strictly I and P frames. But the hardware decoder doesn't know that. It looks at the profile (MAIN), sees it could theoretically have B-frames, and buffers 1-4 frames just in case.

From an [Intel forum post](https://community.intel.com/t5/Media-Intel-oneAPI-Video/h-264-decoder-gives-two-frames-latency-while-decoding-a-stream/m-p/1099694):
> "According to AVC spec, a decoder doesn't have to return a decoded surface immediately for displaying. Even in the absence of B frames - the decoder doesn't know in advance that it won't later encounter B frames, so reordering might still be present."

With 60 FPS content, a 2-frame buffer means 33ms latency - imperceptible. With 2 FPS keepalive frames, a 2-frame buffer means 1000ms latency. There's the bug.

Software decoding doesn't have this problem. It's 1-in-1-out: decode a frame, output a frame, immediately.

## The Real Fix: VUI Rewriting

Software decoding works, but hardware decoding is more efficient. Is there a way to tell the hardware decoder "hey, this stream has no B-frames, don't buffer"?

Yes. The H.264 spec has a section called VUI (Video Usability Information) that includes bitstream restriction parameters. Two fields matter:
- `max_num_reorder_frames` - how many frames might need reordering
- `max_dec_frame_buffering` - how many frames the decoder needs to buffer

If we set `max_num_reorder_frames=0`, we're telling the decoder "these frames are in order, output them immediately."

WebRTC already does this. Their [sps_vui_rewriter.cc](https://webrtc.googlesource.com/src/+/refs/heads/main/common_video/h264/sps_vui_rewriter.cc#400) modifies the SPS on the fly to add these parameters.

So I wrote a Go implementation. Parse the SPS NAL unit, rewrite the VUI section with our zero-latency parameters, reassemble the stream. The key code:

```go
// CRITICAL: These are the key fields for zero-latency decode
w.WriteExpGolomb(0) // max_num_reorder_frames = 0

maxDecBuf := sps.NumRefFrames
if maxDecBuf == 0 {
    maxDecBuf = 1
}
w.WriteExpGolomb(maxDecBuf) // max_dec_frame_buffering
```

This was trickier than it sounds. H.264 SPS uses Exp-Golomb coding (variable length integers) and emulation prevention bytes (avoiding 0x000001 patterns in the data). Luckily the [mp4ff library](https://github.com/Eyevinn/mp4ff) has proper writers for all of this.

We also changed the encoder to output Constrained Baseline profile and set `constraint_set3_flag=1` in the SPS header, which is another signal to decoders that there's no reordering.

Testing with `?hwdecode=1` now shows the same instant latency as software decoding. The VUI rewriting works.

## Final State

- Software decoding is now the default (guaranteed 1-in-1-out)
- Hardware decoding available via `?hwdecode=1` with VUI rewriting for zero-latency
- Damage-based rendering works correctly at any frame rate
- The 1-second latency bug is fixed

## Lessons

1. **Hardware decoders make assumptions.** They're designed for video playback, not real-time streaming. Without explicit signals in the bitstream, they assume the worst case.

2. **The latency formula matters.** 2 frames at 60 FPS = 33ms (fine). 2 frames at 2 FPS = 1000ms (disaster). Frame buffering latency scales inversely with frame rate.

3. **Software can be faster than hardware.** Not in raw decode speed, but in end-to-end latency. Sometimes the "slow" path is actually faster.

4. **Read the spec.** The H.264 VUI parameters existed for exactly this use case. We just didn't know to look for them.

5. **WebRTC solved this already.** Their sps_vui_rewriter.cc was the template for our fix. Standing on the shoulders of giants.

## Links

- [WebCodecs Issue #732](https://github.com/w3c/webcodecs/issues/732) - the issue that explained everything
- [WebCodecs Issue #698](https://github.com/w3c/webcodecs/issues/698) - flush without reset (not implemented)
- [WebRTC sps_vui_rewriter.cc](https://webrtc.googlesource.com/src/+/refs/heads/main/common_video/h264/sps_vui_rewriter.cc#400) - the reference implementation
- [Intel forum post](https://community.intel.com/t5/Media-Intel-oneAPI-Video/h-264-decoder-gives-two-frames-latency-while-decoding-a-stream/m-p/1099694) - explaining decoder buffering

---

*The fix was hiding in plain sight. Sometimes you spend hours looking at your encoder pipeline when the problem is in the browser's decoder. Test your assumptions.*
