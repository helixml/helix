# PipeWire Keepalive Mechanism for GNOME Damage-Based Frame Delivery

**Date:** 2026-01-06
**Author:** Claude (with Luke)
**Status:** Implemented and Deployed

## Reference

Official GStreamer PipeWire source code:
- File: `pipewire-src/src/gst/gstpipewiresrc.c`
- Key lines: 1600-1650 (keepalive implementation)
- Properties: `keepalive-time` (lines 440-448), `resend-last` (lines 431-438)

## Why Not Configure PipeWire for Constant Framerate?

The xdg-desktop-portal ScreenCast API does not have an option to request constant framerate delivery.
GNOME's Mutter uses damage tracking for efficiency - it only sends frames when pixels change.
This is by design for power savings and bandwidth reduction.

The official `pipewiresrc` solves this with the keepalive mechanism, not by requesting constant framerate.
See: [xdg-desktop-portal ScreenCast API](https://github.com/flatpak/xdg-desktop-portal/blob/main/data/org.freedesktop.portal.ScreenCast.xml)

## Problem Statement

GNOME 49+ uses damage-based frame delivery for ScreenCast via PipeWire. This means:
- Frames are only sent when screen content changes
- A static desktop produces NO frames
- Long gaps (30+ seconds) between frames are normal

Our `pipewirezerocopysrc` GStreamer element currently blocks waiting for frames from PipeWire with a 30-second timeout. When no frames arrive:
1. The downstream pipeline (interpipesink) has no buffers to pass
2. The consumer pipeline (interpipesrc → nvh264enc → Moonlight) receives 0 frames
3. Moonlight client shows black screen or connection timeout

## Analysis of Official pipewiresrc

The official GStreamer PipeWire source (`/prod/home/luke/pm/pipewire-src/src/gst/gstpipewiresrc.c`) solves this with two key mechanisms:

### 1. `keepalive_time` Property (lines 440-448)

```c
g_object_class_install_property (gobject_class,
    PROP_KEEPALIVE_TIME,
    g_param_spec_int ("keepalive-time",
        "Keepalive Time",
        "Periodically send last buffer (in milliseconds, 0 = disabled)",
        0, G_MAXINT, DEFAULT_KEEPALIVE_TIME,  // DEFAULT = 0 (disabled)
        G_PARAM_READWRITE | G_PARAM_STATIC_STRINGS));
```

### 2. `resend_last` Property (lines 431-438)

```c
g_object_class_install_property (gobject_class,
    PROP_RESEND_LAST,
    g_param_spec_boolean ("resend-last",
        "Resend last",
        "Resend last buffer on EOS",
        DEFAULT_RESEND_LAST,  // DEFAULT = false
        G_PARAM_READWRITE | G_PARAM_STATIC_STRINGS));
```

### 3. Implementation in `gst_pipewire_src_create()` (lines 1547-1674)

The key logic is in the main buffer dequeue loop:

```c
while (TRUE) {
    // ... state checks ...

    if (pwsrc->eos) {
        // EOS handling with resend_last
        if (pwsrc->last_buffer == NULL)
            goto streaming_eos;
        buf = pwsrc->last_buffer;
        pwsrc->last_buffer = NULL;
        update_time = TRUE;
        break;
    } else if (timeout && pwsrc->last_buffer != NULL) {
        // KEEPALIVE: timeout expired, resend last buffer
        update_time = TRUE;
        buf = gst_buffer_ref(pwsrc->last_buffer);
        GST_LOG_OBJECT(pwsrc, "timeout, send keepalive buffer");
        break;
    } else {
        buf = dequeue_buffer(pwsrc);
        if (buf != NULL) {
            // Store last buffer for potential keepalive
            if (pwsrc->resend_last || pwsrc->keepalive_time > 0)
                gst_buffer_replace(&pwsrc->last_buffer, buf);
            break;
        }
    }

    // Wait with timeout
    timeout = FALSE;
    if (pwsrc->keepalive_time > 0) {
        if (!have_abstime) {
            pw_thread_loop_get_time(loop, &abstime,
                pwsrc->keepalive_time * SPA_NSEC_PER_MSEC);
            have_abstime = TRUE;
        }
        if (pw_thread_loop_timed_wait_full(loop, &abstime) == -ETIMEDOUT)
            timeout = TRUE;
    } else {
        pw_thread_loop_wait(loop);  // Wait forever
    }
}

// Update timestamps for keepalive buffers
if (update_time) {
    GstClock *clock = gst_element_get_clock(GST_ELEMENT_CAST(pwsrc));
    if (clock != NULL) {
        pts = dts = gst_clock_get_time(clock);
        gst_object_unref(clock);
    } else {
        pts = dts = GST_CLOCK_TIME_NONE;
    }
    GST_BUFFER_PTS(*buffer) = pts;
    GST_BUFFER_DTS(*buffer) = dts;
}
```

## Proposed Solution

Implement the same keepalive mechanism in our Rust `pipewirezerocopysrc`:

### 1. Add Properties

```rust
pub struct Settings {
    // ... existing fields ...
    keepalive_time_ms: u32,  // 0 = disabled, else milliseconds
    resend_last: bool,
}
```

### 2. Add State for Last Buffer

```rust
pub struct State {
    // ... existing fields ...
    last_buffer: Option<gst::Buffer>,
}
```

### 3. Modify `create()` Implementation

```rust
fn create(&self, _buffer: Option<&mut gst::BufferRef>) -> Result<CreateSuccess, gst::FlowError> {
    let keepalive_time_ms = self.settings.lock().keepalive_time_ms;

    let mut g = self.state.lock();
    let state = g.as_mut().ok_or(gst::FlowError::Eos)?;
    let stream = state.stream.as_ref().ok_or(gst::FlowError::Error)?;

    // Use keepalive timeout if configured, otherwise use default 30s
    let timeout = if keepalive_time_ms > 0 {
        Duration::from_millis(keepalive_time_ms as u64)
    } else {
        Duration::from_secs(30)
    };

    // Try to receive frame with timeout
    match stream.recv_frame_timeout(timeout) {
        Ok(frame) => {
            let buffer = self.process_frame(frame, state)?;
            // Store for keepalive
            if keepalive_time_ms > 0 {
                state.last_buffer = Some(buffer.clone());
            }
            Ok(CreateSuccess::NewBuffer(buffer))
        }
        Err(_timeout) if keepalive_time_ms > 0 => {
            // Timeout: resend last buffer with updated timestamps
            if let Some(ref last_buf) = state.last_buffer {
                let mut buf = last_buf.copy();
                // Update PTS/DTS to current time
                if let Some(clock) = self.obj().clock() {
                    let now = clock.time().unwrap_or(gst::ClockTime::NONE);
                    buf.get_mut().unwrap().set_pts(now);
                    buf.get_mut().unwrap().set_dts(now);
                }
                Ok(CreateSuccess::NewBuffer(buf))
            } else {
                // No last buffer, continue waiting
                Err(gst::FlowError::Error)
            }
        }
        Err(e) => {
            gst::error!(CAT, imp = self, "Frame receive failed: {}", e);
            Err(gst::FlowError::Error)
        }
    }
}
```

### 4. Modify `pipewire_stream.rs`

Add a timeout-aware receive method:

```rust
impl PipeWireStream {
    pub fn recv_frame_timeout(&self, timeout: Duration) -> Result<FrameData, RecvError> {
        if let Some(err) = self.error.lock().take() {
            return Err(RecvError::Error(err));
        }
        self.frame_rx.recv_timeout(timeout)
            .map_err(|e| match e {
                mpsc::RecvTimeoutError::Timeout => RecvError::Timeout,
                mpsc::RecvTimeoutError::Disconnected => RecvError::Disconnected,
            })
    }
}

pub enum RecvError {
    Timeout,
    Disconnected,
    Error(String),
}
```

## Recommended Default Configuration

For Helix streaming use case:
- `keepalive_time_ms = 100` (10 FPS minimum during static screens)
- This ensures Moonlight always has frames to display

In the Wolf producer pipeline:
```
pipewirezerocopysrc pipewire-node-id=45 render-node=/dev/dri/renderD128
    output-mode=cuda keepalive-time=100 ! ...
```

## Alternative Approaches Considered

### 1. Use `videorate` Element
Add `videorate` after the source to maintain constant framerate:
```
pipewirezerocopysrc ! videorate ! video/x-raw,framerate=60/1 ! ...
```
**Rejected:** Adds latency and CPU overhead; doesn't solve the root cause.

### 2. Request Fixed Framerate from PipeWire
Some PipeWire sources support requesting a fixed framerate.
**Rejected:** GNOME ScreenCast doesn't support this; it only provides damage-based delivery.

### 3. Use Frame Clock in Pipeline
Add a synthetic frame clock that duplicates frames.
**Rejected:** More complex than keepalive; same effect achieved with keepalive.

## Implementation Plan

1. **Phase 1**: Add `keepalive-time` and `resend-last` properties to `pipewirezerocopysrc`
2. **Phase 2**: Modify `recv_frame()` to support timeout-based receive
3. **Phase 3**: Update Wolf pipeline to use `keepalive-time=100`
4. **Phase 4**: Test with static desktop scenarios

## Files to Modify

1. `/prod/home/luke/pm/wolf/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs`
   - Add properties: `keepalive-time`, `resend-last`
   - Add `last_buffer` to State
   - Modify `create()` for keepalive logic

2. `/prod/home/luke/pm/wolf/gst-pipewire-zerocopy/src/pipewire_stream.rs`
   - Add `recv_frame_timeout()` method
   - Add `RecvError` enum

3. `/prod/home/luke/pm/helix/wolf/config.toml.template` (or Wolf startup)
   - Add `keepalive-time=100` to pipewirezerocopysrc pipeline

## Success Criteria

1. Static desktop shows last frame (not black screen)
2. Moonlight client maintains connection during long static periods
3. Frame rate is at least 10 FPS during static screens (with keepalive-time=100)
4. No increase in GPU memory usage (keepalive reuses last buffer)
