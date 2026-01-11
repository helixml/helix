# Mutter Damage-Based Frame Pacing: Technical Deep Dive

**Date**: 2026-01-11
**Status**: Investigation
**Related**: Video streaming performance, GNOME headless mode

## Executive Summary

This document explains how Mutter (GNOME's compositor) implements damage-based frame pacing for ScreenCast in headless mode. Understanding this is critical for optimizing video streaming from GNOME desktops.

**Key Finding**: In our testing, we achieve ~35fps instead of the expected 60fps with active content (vkcube). This document explains the Mutter frame production mechanism to help identify potential bottlenecks.

## 1. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         GNOME Shell                              │
│  ┌─────────────────┐      ┌──────────────────────────────────┐  │
│  │  Virtual CRTC   │◄────►│  ClutterFrameClock (PASSIVE)     │  │
│  │  3840x2160@60Hz │      │  - No internal timer             │  │
│  └────────┬────────┘      │  - Driven by ScreenCast driver   │  │
│           │               └──────────────────────────────────┘  │
│           ▼                                                      │
│  ┌─────────────────┐      ┌──────────────────────────────────┐  │
│  │  MetaStageView  │─────►│  meta_screen_cast_stream_src     │  │
│  │  (rendering)    │      │  - Records frames to PipeWire    │  │
│  └─────────────────┘      │  - Enforces max_framerate cap    │  │
│                           └───────────────┬──────────────────┘  │
└───────────────────────────────────────────┼─────────────────────┘
                                            │
                    ┌───────────────────────▼───────────────────┐
                    │            PipeWire Buffer Pool           │
                    │  - 2-16 buffers (16 preferred)            │
                    │  - DMA-BUF or SHM backed                  │
                    │  - max_framerate: 60/1                    │
                    └───────────────────────┬───────────────────┘
                                            │
                    ┌───────────────────────▼───────────────────┐
                    │           GStreamer pipewiresrc           │
                    │  - Dequeues buffers from pool             │
                    │  - Processes and forwards to encoder      │
                    └───────────────────────────────────────────┘
```

## 2. Virtual Monitor Creation (Headless Mode)

When GNOME Shell starts with `--headless --virtual-monitor WIDTHxHEIGHT@REFRESH`:

### 2.1 CRTC Mode Creation
**Source**: `src/backends/native/meta-crtc-mode-virtual.c:34-58`

```c
MetaCrtcModeVirtual *
meta_crtc_mode_virtual_new (uint64_t id, const MetaVirtualModeInfo *info)
{
  crtc_mode_info = meta_crtc_mode_info_new ();
  crtc_mode_info->width = info->width;          // e.g., 3840
  crtc_mode_info->height = info->height;        // e.g., 2160
  crtc_mode_info->refresh_rate = info->refresh_rate;  // e.g., 60.0

  mode_virtual = g_object_new (META_TYPE_CRTC_MODE_VIRTUAL,
                               "id", META_CRTC_MODE_VIRTUAL_ID_BIT | id,
                               "name", crtc_mode_name,  // "3840x2160@60.000000"
                               "info", crtc_mode_info,
                               NULL);
  return mode_virtual;
}
```

### 2.2 Virtual Output Creation
**Source**: `src/backends/native/meta-output-virtual.c:35-70`

```c
MetaOutputVirtual *
meta_output_virtual_new (uint64_t id,
                         const MetaVirtualMonitorInfo *info,
                         MetaCrtcVirtual *crtc_virtual,
                         MetaCrtcModeVirtual *crtc_mode_virtual)
{
  output_info = meta_output_info_new ();
  output_info->name = g_strdup_printf ("Meta-%" G_GUINT64_FORMAT, id);
  output_info->is_virtual = TRUE;
  output_info->n_modes = 1;
  output_info->modes[0] = META_CRTC_MODE (crtc_mode_virtual);
  output_info->preferred_mode = output_info->modes[0];
  // ...
}
```

## 3. ScreenCast Frame Clock: PASSIVE Mode

This is the most critical mechanism for understanding frame timing in headless mode.

### 3.1 Frame Clock Modes
**Source**: `clutter/clutter/clutter-frame-clock.c`

Clutter's frame clock has three modes:
1. **VARIABLE**: Driven by VBlank/page flip events (real displays)
2. **FIXED**: Fixed interval timer (legacy)
3. **PASSIVE**: **No internal timer - externally driven by ScreenCast**

### 3.2 Setting PASSIVE Mode
**Source**: `src/backends/meta-screen-cast-virtual-stream-src.c:208-223`

When a ScreenCast stream is enabled on a virtual monitor:

```c
static void
make_frame_clock_passive (MetaScreenCastVirtualStreamSrc *virtual_src,
                          ClutterStageView               *view)
{
  ClutterFrameClock *frame_clock = clutter_stage_view_get_frame_clock (view);
  MetaScreenCastFrameClockDriver *driver;

  driver = g_object_new (META_TYPE_SCREEN_CAST_FRAME_CLOCK_DRIVER, NULL);
  driver->src = src;

  // This removes internal timers and sets external driver
  clutter_frame_clock_set_passive (frame_clock,
                                   CLUTTER_FRAME_CLOCK_DRIVER (driver));
}
```

### 3.3 Frame Clock Passive Implementation
**Source**: `clutter/clutter/clutter-frame-clock.c:1923-1937`

```c
void
clutter_frame_clock_set_passive (ClutterFrameClock *frame_clock,
                                 ClutterFrameClockDriver *driver)
{
  g_set_object (&frame_clock->driver, driver);
  frame_clock->mode = CLUTTER_FRAME_CLOCK_MODE_PASSIVE;

  // CRITICAL: This removes any internal timer
  clear_source (frame_clock);
}
```

**Implication**: In PASSIVE mode, the frame clock has no internal timer. Frame production is entirely driven by the ScreenCast system responding to PipeWire buffer availability.

## 4. Damage-Based Frame Recording

### 4.1 Paint Watch Registration
**Source**: `src/backends/meta-screen-cast-virtual-stream-src.c:226-262`

```c
static void
setup_view (MetaScreenCastVirtualStreamSrc *virtual_src,
            ClutterStageView               *view)
{
  // Register callback AFTER painting completes
  virtual_src->paint_watch =
    meta_stage_watch_view (meta_stage,
                           view,
                           META_STAGE_WATCH_AFTER_PAINT,
                           on_after_paint,
                           virtual_src);

  // Register callback for skipped paints (cursor-only updates)
  virtual_src->skipped_watch =
    meta_stage_watch_view (meta_stage,
                           view,
                           META_STAGE_WATCH_SKIPPED_PAINT,
                           on_skipped_paint,
                           virtual_src);
}
```

### 4.2 Frame Recording After Paint
**Source**: `src/backends/meta-screen-cast-virtual-stream-src.c:159-176`

```c
static void
on_after_paint (MetaStage        *stage,
                ClutterStageView *view,
                const MtkRegion  *redraw_clip,  // Damaged region
                ClutterFrame     *frame,
                gpointer          user_data)
{
  MetaScreenCastStreamSrc *src = META_SCREEN_CAST_STREAM_SRC (user_data);

  flags = META_SCREEN_CAST_RECORD_FLAG_NONE;
  paint_phase = META_SCREEN_CAST_PAINT_PHASE_PRE_SWAP_BUFFER;

  // Record frame to PipeWire
  meta_screen_cast_stream_src_record_frame (src, flags, paint_phase, redraw_clip);
}
```

### 4.3 Paint Only Happens With Damage
**Source**: `clutter/clutter/clutter-stage-view.c:1107`

```c
if (clutter_stage_view_has_redraw_clip (view))
{
  // Actually paint the frame
}
else
{
  // Skip paint - emit skipped_paint signal instead
  clutter_stage_emit_skipped_paint (stage, view, frame);
}
```

**This is damage-based ScreenCast**: Frames are only recorded when there's actual screen damage.

## 5. Frame Rate Limiting

### 5.1 Max Framerate Cap
**Source**: `src/backends/meta-screen-cast-stream-src.c:1266-1288`

```c
static MetaScreenCastRecordResult
meta_screen_cast_stream_src_record_frame (MetaScreenCastStreamSrc  *src,
                                          ...)
{
  // Enforce max_framerate cap
  if (priv->video_format.max_framerate.num > 0 &&
      priv->last_frame_timestamp_us != 0)
  {
    int64_t min_interval_us;
    int64_t time_since_last_frame_us;

    // Calculate minimum interval (e.g., 16.67ms for 60fps)
    min_interval_us =
      ((G_USEC_PER_SEC * priv->video_format.max_framerate.denom) /
       priv->video_format.max_framerate.num);

    time_since_last_frame_us = frame_timestamp_us - priv->last_frame_timestamp_us;

    if (time_since_last_frame_us < min_interval_us)
    {
      // Too soon - schedule a follow-up and skip this frame
      timeout_us = min_interval_us - time_since_last_frame_us;
      maybe_schedule_follow_up_frame (src, timeout_us);
      return record_result;  // Skip frame
    }
  }

  // Frame rate OK - proceed with recording
  return meta_screen_cast_stream_src_record_frame_with_timestamp (...);
}
```

### 5.2 PipeWire Buffer Pool Size
**Source**: `src/backends/meta-screen-cast-stream-src.c:1695`

```c
// Mutter offers 2-16 buffers to PipeWire, preferring 16
SPA_PARAM_BUFFERS_buffers, SPA_POD_CHOICE_RANGE_Int (16, 2, 16),
```

### 5.3 Buffer Starvation Handling
**Source**: `src/backends/meta-screen-cast-stream-src.c:1074-1081`

```c
if (!buffer)
{
  g_set_error (error,
               G_IO_ERROR,
               G_IO_ERROR_FAILED,
               "Couldn't dequeue a buffer from pipewire stream (node id %u), "
               "maybe your encoding is too slow?",
               pw_stream_get_node_id (priv->pipewire_stream));
}
```

**Critical Insight**: If the consumer (pipewiresrc) doesn't return buffers fast enough, Mutter will skip frames due to buffer starvation.

## 6. ScreenCast Driver: Frame Scheduling

### 6.1 Driver Schedule Update
**Source**: `src/backends/meta-screen-cast-virtual-stream-src.c:803-810`

```c
static void
meta_screen_cast_frame_clock_driver_schedule_update (
    ClutterFrameClockDriver *frame_clock_driver)
{
  MetaScreenCastFrameClockDriver *driver =
    META_SCREEN_CAST_FRAME_CLOCK_DRIVER (frame_clock_driver);

  if (driver->src)
    meta_screen_cast_stream_src_request_process (driver->src);
}
```

### 6.2 Request Processing (Triggers PipeWire)
**Source**: `src/backends/meta-screen-cast-stream-src.c:966-980`

```c
void
meta_screen_cast_stream_src_request_process (MetaScreenCastStreamSrc *src)
{
  if (!priv->pending_process &&
      !pw_stream_is_driving (priv->pipewire_stream))
  {
    // Trigger PipeWire to process next buffer
    pw_stream_trigger_process (priv->pipewire_stream);
    priv->pending_process = TRUE;
  }
}
```

## 7. Frame Production Flow

```
1. Screen damage occurs (vkcube renders, window moves, etc.)
           │
           ▼
2. ClutterStageView schedules redraw
           │
           ▼
3. Frame clock dispatches (if PASSIVE: triggered by ScreenCast driver)
           │
           ▼
4. Stage paints to framebuffer
           │
           ▼
5. AFTER_PAINT watch triggers → on_after_paint()
           │
           ▼
6. meta_screen_cast_stream_src_record_frame()
           │
           ├── Check max_framerate cap (skip if too soon)
           │
           ├── Dequeue buffer from PipeWire pool
           │   (if no buffer available: skip frame)
           │
           ▼
7. Copy frame to PipeWire buffer (DMA-BUF or SHM)
           │
           ▼
8. Queue buffer back to PipeWire stream
           │
           ▼
9. Consumer (pipewiresrc) receives buffer
           │
           ▼
10. Consumer processes & returns buffer to pool
           │
           ▼
11. Repeat from step 1
```

## 8. Observed Performance Issue

### 8.1 Benchmark Results

| Test Condition | Expected FPS | Actual FPS | Bottleneck Location |
|----------------|--------------|------------|---------------------|
| Static screen | 10 (keepalive) | 10 | N/A (working correctly) |
| vkcube @ 1080p | 60 | 32-38 | Unknown |
| vkcube @ 4K native | 60 | 34-36 | Unknown |

### 8.2 Component Utilization During Benchmark

| Component | Utilization | Assessment |
|-----------|-------------|------------|
| NVENC encoder | 27-46% | NOT bottleneck |
| GPU (vkcube) | 68-88% | Healthy |
| Mutter max_framerate | 60/1 | Correctly configured |
| PipeWire buffer pool | 16 buffers | Adequate |

### 8.3 Potential Bottleneck Locations

1. **pipewiresrc buffer return latency**: GStreamer may hold buffers too long before returning them to the pool, causing Mutter to skip frames due to buffer starvation.

2. **Pipeline round-trip time**: The total time for a buffer to flow through:
   - pipewiresrc → videorate → videoscale → cudaupload → nvh264enc → output

   This round-trip may exceed 16.67ms (60fps threshold).

3. **4-second startup delay**: Every benchmark shows 4 seconds of 0 FPS before frames start flowing. This suggests pipeline initialization or ScreenCast session setup overhead.

## 9. Recommendations for Further Investigation

1. **Enable GStreamer tracing** to measure per-element latency:
   ```bash
   GST_DEBUG=GST_TRACER:7 GST_TRACERS="latency" gst-launch-1.0 ...
   ```

2. **Monitor PipeWire buffer pool usage**:
   ```bash
   pw-top  # Watch during streaming
   ```

3. **Test with fewer pipeline elements** to isolate bottleneck:
   - Remove videorate/videoscale
   - Test zerocopy mode vs SHM mode

4. **Measure buffer return timing** in pipewiresrc:
   - Log timestamps when buffer is dequeued vs when it's queued back

## 10. Source Code References

All references are from Mutter 49.0 (Ubuntu 25.04):

| File | Key Functions |
|------|---------------|
| `src/backends/meta-screen-cast-stream-src.c` | Frame recording, rate limiting, buffer management |
| `src/backends/meta-screen-cast-virtual-stream-src.c` | Virtual monitor ScreenCast, PASSIVE frame clock driver |
| `clutter/clutter/clutter-frame-clock.c` | Frame clock modes, PASSIVE mode implementation |
| `clutter/clutter/clutter-stage-view.c` | Paint scheduling, damage tracking |
| `src/backends/native/meta-crtc-mode-virtual.c` | Virtual monitor mode creation |
| `src/backends/native/meta-output-virtual.c` | Virtual output creation |

## Appendix A: PipeWire Node Properties (from pw-dump)

```json
{
  "id": 47,
  "info": {
    "state": "running",
    "props": {
      "media.class": "Stream/Output/Video",
      "media.name": "meta-screen-cast-src",
      "node.name": "gnome-shell",
      "stream.is-live": true
    },
    "params": {
      "EnumFormat": [{
        "format": "BGRx",
        "size": { "width": 3840, "height": 2160 },
        "framerate": { "num": 0, "denom": 1 },
        "maxFramerate": {
          "default": { "num": 60, "denom": 1 },
          "min": { "num": 0, "denom": 1 },
          "max": { "num": 60, "denom": 1 }
        }
      }]
    }
  }
}
```

## Appendix B: Related Design Documents

- `design/2026-01-10-zerocopy-format-negotiation-fix.md` - Zero-copy pipeline format issues
