# Mutter Headless 60 FPS Investigation

**Date:** 2026-01-11
**Status:** In Progress
**Author:** Claude Code

## Problem Statement

GNOME video streaming is limited to 37-40 FPS instead of 60 FPS even when content is actively changing (e.g., 60fps YouTube video, vkcube rendering at 60fps). This investigation traces the frame production chain in Mutter's headless mode and identifies the bottlenecks.

## Root Causes Identified

### 1. Mutter's dynamic-max-render-time Throttling

In headless mode, Mutter lacks a physical VSync signal and defaults to an internal timer. The `dynamic-max-render-time` algorithm frequently caps at 40 FPS or 30 FPS if it detects even micro-millisecond delays in the consumer pipeline.

**Frame Pacing Issue**: Mutter's algorithm tries to ensure the compositor never "misses" a frame. If the GStreamer/CUDA pipeline holds a DMA-Buf just a bit too long, Mutter assumes it can't hit 60 FPS and drops to a "safe" 40 FPS tier.

**The 40 FPS Trap**: This creates a feedback loop where the total round-trip time (Request -> Render -> DMA-Buf -> CUDA -> Signal Ready) exceeds the 16.6ms window, causing Mutter to drop into a lower stable tier.

### 2. Implicit Sync Bottleneck on NVIDIA

NVIDIA's proprietary driver doesn't always support implicit synchronization for DMA-Bufs in Wayland. When Mutter finishes rendering a frame and flushes to DMA-Buf, if the CUDA code attempts to access that buffer before the GPU has fully "posted" it, Mutter may block the main compositor thread.

This blocking behavior results in a "sawtooth" framerate that averages ~40 FPS as the compositor misses every other vblank interval.

### 3. Buffer Starvation

If PipeWire only provides 2-3 buffers and the CUDA encoder takes just long enough to process one, Mutter has no available buffer to paint the next frame into. This causes frame skipping.

**The Math**: If Buffer A is being processed by CUDA/NVENC and Buffer B is being rendered by Mutter, there is zero room for error. If Buffer A isn't released back before the next vblank, Mutter skips that frame.

### 4. Caps Negotiation Issues

Mutter's screen-cast sometimes negotiates a variable framerate that defaults to a lower range if the sink doesn't explicitly demand a fixed rate. Without explicit `framerate=60/1` in caps, Mutter may "helpfully" throttle the stream.

## Frame Production Chain (Passive Mode)

```
Actor queues redraw
    → clutter_stage_schedule_update
    → clutter_stage_view_schedule_update
    → clutter_frame_clock_schedule_update
    → (passive mode) clutter_frame_clock_driver_schedule_update
    → meta_screen_cast_stream_src_request_process
    → pw_stream_trigger_process (request PipeWire to process)

PipeWire callback fires:
    → on_stream_process
    → meta_screen_cast_virtual_stream_src_dispatch
    → clutter_frame_clock_dispatch (THIS PAINTS)
    → on_after_paint captures frame → PipeWire buffer

Frame is sent to consumer (GStreamer pipeline)
```

### Key Code Locations

- **Frame Rate Limiter**: `mutter/src/backends/meta-screen-cast-stream-src.c:1322-1345`
- **Request Process (with coalescing)**: `meta-screen-cast-stream-src.c:970-983`
- **on_stream_process callback**: `meta-screen-cast-stream-src.c:1522-1539`
- **Virtual Stream Dispatch**: `meta-screen-cast-virtual-stream-src.c:731-748`
- **Frame Clock Passive Mode**: `clutter/clutter-frame-clock.c:1911-1926`

## Fixes Applied

### Environment Variables (Dockerfile.ubuntu-helix)

```dockerfile
ENV MUTTER_DEBUG_KMS_THREAD_TYPE=user \    # Workaround for Mutter issue #3788 (60→40fps drops)
    MUTTER_DEBUG=screen-cast \             # Debug logging for screen-cast events
    CLUTTER_DEBUG=frame-clock,frame-timings \  # Frame clock timing analysis
    CLUTTER_PAINT=disable-dynamic-max-render-time \  # KEY FIX: Disable throttling
    CLUTTER_DEFAULT_FPS=60                 # Force 60 FPS render target
```

### Experimental Features (startup script)

```bash
gsettings set org.gnome.mutter experimental-features "['variable-refresh-rate', 'triple-buffering']"
```

### GStreamer Pipeline Changes (ws_stream.go)

1. **Queue Buffer Size: 1 → 3**
   - Prevents PipeWire buffer starvation
   - Decouples pipewiresrc from encoding pipeline
   ```go
   args = append(args, "!", "queue", "max-size-buffers=3", "leaky=downstream")
   ```

2. **Explicit Framerate in Caps**
   - Prevents Mutter from defaulting to variable/lower framerate
   ```go
   // Native mode:
   "!", fmt.Sprintf("video/x-raw,framerate=%d/1", v.config.FPS),

   // SHM mode:
   "!", fmt.Sprintf("video/x-raw,format=BGRx,framerate=%d/1", v.config.FPS),
   ```

### Debug Logging

With `MUTTER_DEBUG=screen-cast`, these log messages appear:
- "Request processing on stream %u" - when `pw_stream_trigger_process` called
- "Processing stream %u" - when `on_stream_process` callback fires
- "Recording frame on stream %u" - when frame is actually recorded
- "Skipped recording frame, too early" - when frame rate limiter blocks

## Testing

To test the changes:

1. Start a new session (existing sessions won't get the new image)
2. Run vkcube or play a 60fps video
3. Connect to the video stream
4. Observe framerate - should be closer to 60fps than 40fps

Check for debug logs:
```bash
docker compose exec -T sandbox docker logs <container_name> 2>&1 | grep -E "Request processing|Processing stream|Recording frame|Late frame|Skipping frame"
```

## Future Improvements

### 1. Buffer Pool Size in PipeWire Negotiation

The current PipeWire buffer negotiation only specifies dataType (DmaBuf vs MemFd), not buffer count. We could try adding buffer count parameters, though previous attempts caused negotiation failures.

### 2. CUDA Context Sharing

Ensure the Rust zerocopy plugin and nvh264enc share the same GstCudaContext to avoid hidden copies between contexts.

### 3. nvidia-smi Monitoring

Monitor during streaming:
```bash
nvidia-smi dmon
```
- High "sm" but low "enc" usage indicates blocking
- Check for context switching overhead

### 4. Mutter Debug Paint

Run with `MUTTER_DEBUG=paint-setup` to look for:
- "Late frame" logs
- "Skipping frame" logs

These confirm the consumer is returning buffers too slowly.

## References

- [Mutter Issue #3788](https://gitlab.gnome.org/GNOME/mutter/-/issues/3788) - Frame rate drops workaround
- Wolf project - Buffer management approach
- gnome-remote-desktop - Reference implementation for PipeWire negotiation
- `meta-screen-cast-stream-src.c` - Frame rate limiter implementation
- `clutter-frame-clock.c` - Passive mode frame clock

## Image History

| Hash | Changes |
|------|---------|
| `77bc69` | Previous baseline |
| `bdf77e` | Added MUTTER_DEBUG, CLUTTER_DEBUG, VRR, triple-buffering |
| `2ae2da` | Added CLUTTER_PAINT, CLUTTER_DEFAULT_FPS, queue buffer size, explicit framerate caps |

## New Finding: 16ms/17ms Timing Boundary Issue (2026-01-11)

### Observation

Despite all environment variables being correctly set, Mutter logs still show ~50% of frames being skipped:

```
libmutter-Message: 19:40:04.845: SCREEN_CAST: Skipped recording frame on stream 44, too early
libmutter-Message: 19:40:04.862: SCREEN_CAST: Recording full frame on stream 44
libmutter-Message: 19:40:04.878: SCREEN_CAST: Skipped recording frame on stream 44, too early
libmutter-Message: 19:40:04.895: SCREEN_CAST: Recording full frame on stream 44
libmutter-Message: 19:40:04.912: SCREEN_CAST: Skipped recording frame on stream 44, too early
libmutter-Message: 19:40:04.928: SCREEN_CAST: Recording full frame on stream 44
```

### Timing Analysis

The frame timestamps show an alternating pattern of 16ms and 17ms intervals:

```
845 → 862: 17ms (recorded)
862 → 878: 16ms (skipped)
878 → 895: 17ms (recorded - 33ms from 862)
895 → 912: 17ms (skipped)
912 → 928: 16ms (recorded - 33ms from 895)
```

### Hypothesis: min_interval Boundary Effect

Mutter's frame rate limiter uses this calculation:
```c
min_interval_us = (G_USEC_PER_SEC * priv->video_format.max_framerate.denom) /
                   priv->video_format.max_framerate.num;
```

With `max_framerate = 60/1`:
- min_interval_us = 1,000,000 / 60 = **16,666µs = 16.666ms**

The problem:
- GNOME produces frames at alternating 16ms and 17ms intervals (due to system timer granularity)
- 16ms < 16.666ms → frame rejected as "too early"
- 17ms > 16.666ms → frame accepted
- Result: ~50% of frames skipped → **effective 30fps instead of 60fps**

### Proposed Fix

Request a higher `max_framerate` in PipeWire negotiation:

| max_framerate | min_interval | 16ms frames | 17ms frames |
|---------------|--------------|-------------|-------------|
| 60/1 | 16.666ms | ❌ rejected | ✓ accepted |
| 120/1 | 8.333ms | ✓ accepted | ✓ accepted |

With `max_framerate = 120/1`:
- min_interval_us = 1,000,000 / 120 = **8,333µs = 8.33ms**
- Both 16ms and 17ms intervals easily pass the "too early" check
- The actual frame rate is still limited by GNOME's production (~60fps)

### Mutter Source Code Verification (2026-01-11)

Traced the "too early" message to exact source location in Mutter:
- **File**: `src/backends/meta-screen-cast-stream-src.c` lines 1322-1345
- **Source**: https://gitlab.gnome.org/GNOME/mutter/-/blob/main/src/backends/meta-screen-cast-stream-src.c

```c
if (priv->video_format.max_framerate.num > 0 &&
    priv->last_frame_timestamp_us != 0)
  {
    int64_t min_interval_us;
    int64_t time_since_last_frame_us;

    min_interval_us =
      ((G_USEC_PER_SEC * ((int64_t) priv->video_format.max_framerate.denom)) /
       ((int64_t) priv->video_format.max_framerate.num));

    time_since_last_frame_us = frame_timestamp_us - priv->last_frame_timestamp_us;
    if (time_since_last_frame_us < min_interval_us)
      {
        // ...
        meta_topic (META_DEBUG_SCREEN_CAST,
                    "Skipped recording frame on stream %u, too early",
                    priv->node_id);
        // ...
      }
  }
```

This confirms the fix WILL work:
| max_framerate | min_interval_us | 16ms frame | 17ms frame |
|---------------|-----------------|------------|------------|
| 60/1 | 16,666µs | 16,000 < 16,666 ❌ | 17,000 > 16,666 ✓ |
| **120/1** | **8,333µs** | 16,000 > 8,333 ✓ | 17,000 > 8,333 ✓ |

### Implementation

Changed `desktop/gst-pipewire-zerocopy/src/pipewire_stream.rs`:
- Added `target_fps` parameter to `PipeWireStream::connect()`
- Calculate `negotiated_max_fps = target_fps * 2` dynamically
- Pass `negotiated_max_fps` to format pod builders
- `build_video_format_params_with_modifiers()` and `build_video_format_params_no_modifier()` now use dynamic max_framerate

Changed `desktop/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs`:
- Added `target-fps` GStreamer property (default: 60, range: 1-240)
- Wolf can set this via pipeline to match session's configured FPS

### Image History

| Hash | Changes |
|------|---------|
| `5c9ba3` | Hardcoded max_framerate=120/1 for testing |
| `f829ec` | Dynamic max_framerate = target_fps * 2 |

### Status

Built and transferred (f829ec). Ready for testing.

## New Finding: Mutter Ignores Consumer's max_framerate Offer (2026-01-11)

### Observation

The 120/1 max_framerate fix didn't work. Logs showed Mutter negotiated 60/1 anyway:

```
[PIPEWIRE_DEBUG] PipeWire video format: 3840x2160 format=8 (VideoFormat::BGRx) framerate=0/1 max_framerate=60/1
```

Despite offering 120/1, Mutter selected its own value (60/1), which still causes ~50% of frames to be skipped due to the 16ms/17ms timing boundary issue.

### Root Cause

In PipeWire format negotiation, the **producer** (Mutter) has final say on parameter values. Our consumer-side `max_framerate` offer is just a suggestion - Mutter picks what it wants based on its own constraints.

### Solution: Request max_framerate=0 to Disable Limiter

Looking at Mutter's source code (`meta-screen-cast-stream-src.c:1322-1345`):

```c
if (priv->video_format.max_framerate.num > 0 &&
    priv->last_frame_timestamp_us != 0)
  {
    // ... frame rate limiting logic
    if (time_since_last_frame_us < min_interval_us)
      {
        meta_topic (META_DEBUG_SCREEN_CAST,
                    "Skipped recording frame on stream %u, too early",
                    priv->node_id);
        // ... skip frame
      }
  }
```

The key insight: **if `max_framerate.num == 0`, the entire limiter is bypassed**.

Changed `pipewire_stream.rs` to request `negotiated_max_fps = 0` instead of `target_fps * 2`.

### Image History

| Hash | Changes |
|------|---------|
| `5c9ba3` | Hardcoded max_framerate=120/1 for testing |
| `f829ec` | Dynamic max_framerate = target_fps * 2 |
| `be6a1f` | Fixed zerocopy plugin inclusion |
| `e43b95` | max_framerate=0 to disable limiter |
| `6722a2` | Fixed libgstcuda-1.0 linking (undefined symbol: gst_cuda_stream_new) |
| `cda8e1` | Added cargo clean to force rebuild with new linking |
| `f27405` | **WORKING** - Fixed min max_framerate from 1/1 to 0/1 |

## Final Fix: min max_framerate Range (2026-01-11)

### Problem

Even with `negotiated_max_fps = 0`, Mutter was negotiating `max_framerate=1/1` (1 FPS!) instead of `0/1`.

### Root Cause

The format pod offered a max_framerate range with `min: 1/1`:

```rust
ChoiceEnum::Range {
    default: Fraction { num: 0, denom: 1 },
    min: Fraction { num: 1, denom: 1 },  // <-- BUG: prevents 0 from being negotiated!
    max: Fraction { num: 360, denom: 1 },
}
```

PipeWire enforces the min value during negotiation, so Mutter clamped our request of 0 to 1.

### Fix

Changed `min` to `0/1` in both `build_video_format_params_with_modifiers()` and `build_video_format_params_no_modifier()`:

```rust
ChoiceEnum::Range {
    default: Fraction { num: 0, denom: 1 },
    min: Fraction { num: 0, denom: 1 },  // Allow 0 to disable limiter!
    max: Fraction { num: 360, denom: 1 },
}
```

### Results

**Before fix:**
- Instant FPS: 10 fps ⚠️
- Average FPS: ~7.8 fps
- Mutter logs: "Skipped recording frame on stream, too early" on ~50% of frames

**After fix (f27405):**
- Instant FPS: 60 fps ✅
- Average FPS: 52.9 fps (1587 frames in 30 seconds)
- Mutter logs: 100% "Recording full frame", zero skipped frames
- Frame interval: ~16-17ms (exactly 60fps timing)

### Additional Fixes Required

1. **libgstcuda-1.0 linking**: The zerocopy plugin uses `waylanddisplaycore` which calls GStreamer CUDA functions via FFI. These symbols must be resolved at plugin load time. Fixed by:
   - Adding `println!("cargo:rustc-link-lib=gstcuda-1.0");` to `build.rs`
   - Adding `libgstreamer-plugins-bad1.0-0` runtime package to Dockerfile
   - Creating symlink: `ln -sf libgstcuda-1.0.so.0 libgstcuda-1.0.so`
   - Running `cargo clean` before build to force recompilation with new linker flags

### Status

**FIXED** - 60 FPS streaming working with GNOME headless mode.
