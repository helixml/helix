# macOS ARM Video Streaming - Scanout Fallback Working

**Date:** 2026-02-06 04:50
**Branch:** feature/macos-arm-desktop-port

## Summary

Successfully implemented scanout resource fallback for video streaming on macOS ARM. QEMU is now reading frames from GPU memory, encoding with VideoToolbox, and sending H.264 data over the socket. The remaining issue is vsockenc not forwarding the H.264 responses to the output pipeline.

## What's Working ✅

1. **Scanout Resource Fallback** - When vsockenc sends resource_id=0, QEMU now falls back to current scanout resource instead of rejecting
2. **Frame Reading** - Successfully reading 8294400 bytes (1920x1080 BGRA) from virtio-gpu via virgl_renderer_transfer_read_iov()
3. **IOSurface Creation** - Creating IOSurface from raw pixel data
4. **VideoToolbox Encoding** - Encoder callback is being invoked with encoded frames
5. **Socket Transmission** - QEMU sends H.264 frames over vsock (22376 bytes for keyframe, 7340 bytes for P-frame)

## Current Blocker 🔴

**vsockenc not outputting H.264 frames to pipeline**

QEMU logs show successful frame sends:
```
[HELIX] encoder_output_callback called: status=0, sampleBuffer=0x73ec2e300
[HELIX] Frame sent successfully: 22376 bytes, pts=103331525, keyframe=1
[HELIX] encoder_output_callback called: status=0, sampleBuffer=0x73ec2d880
[HELIX] Frame sent successfully: 7340 bytes, pts=327136525, keyframe=0
```

But WebSocket client receives 0 video frames. The H.264 data is being sent from QEMU to the TCP socket (10.0.2.2:5900), but vsockenc is not pushing it downstream to h264parse → appsink.

## Code Changes

### helix-frame-export.m (QEMU)

**Scanout Fallback Logic (lines 592-609)**
```objc
/*
 * Handle resource_id extraction:
 * - If guest provides explicit resource_id (from DmaBuf), use it
 * - If resource_id=0, fall back to current scanout resource
 *
 * On virtio-gpu (macOS/UTM), Mutter does NOT support DmaBuf export in headless mode,
 * so the scanout fallback is required for video streaming to work.
 */
uint32_t resource_id = req->resource_id;
if (resource_id == 0) {
    resource_id = helix_get_scanout_resource(fe->virtio_gpu);
    error_report("[HELIX] Using scanout resource_id=%u", resource_id);

    if (resource_id == 0) {
        error_report("[HELIX] No scanout resource available");
        return HELIX_ERR_RESOURCE_NOT_FOUND;
    }
}
```

**Debug Logging in Callback (lines 79-81, 181-184)**
```objc
error_report("[HELIX] encoder_output_callback called: status=%d, sampleBuffer=%p",
             (int)status, sampleBuffer);

...

error_report("[HELIX] Frame sent successfully: %zu bytes, pts=%lld, keyframe=%d",
             sent, pts, is_keyframe);
```

## Architecture Flow

```
┌─────────────────────────────────────────────┐
│ Guest (Ubuntu VM)                           │
├─────────────────────────────────────────────┤
│ GNOME ScreenCast → PipeWire SHM buffers     │
│         ↓                                   │
│ pipewiresrc (triggers frame requests)       │
│         ↓                                   │
│ vsockenc sends resource_id=0 to QEMU        │
│         ↓                                   │
│ TCP 10.0.2.2:5900 (via socat proxy)         │
└─────────────────────────────────────────────┘
              ↓
┌─────────────────────────────────────────────┐
│ Host (macOS)                                │
├─────────────────────────────────────────────┤
│ QEMU helix-frame-export                     │
│  - Receive resource_id=0                    │
│  - Fall back to scanout resource (e.g. 104) │
│  - virgl_renderer_transfer_read_iov()       │
│  - Read 8294400 bytes (1920x1080 BGRA)      │
│  - Create IOSurface from pixel data         │
│  - VideoToolbox H.264 encode (async)        │
│  - Callback: send H.264 over socket         │
│         ↓                                   │
│ ⚠️ H.264 sent but vsockenc not reading? ⚠️   │
└─────────────────────────────────────────────┘
              ↓
┌─────────────────────────────────────────────┐
│ Guest (Ubuntu VM)                           │
├─────────────────────────────────────────────┤
│ vsockenc receives H.264 (socket read)       │
│         ↓                                   │
│ ⚠️ NOT outputting to pipeline ⚠️             │
│         ↓                                   │
│ h264parse → appsink → WebSocket            │
│         ↓                                   │
│ 📊 WebSocket client: 0 frames received      │
└─────────────────────────────────────────────┘
```

## Next Debugging Steps

### 1. Enable GStreamer Debug for vsockenc
```bash
# In container environment
GST_DEBUG=vsockenc:5,basesrc:5

# Look for:
# - Socket read operations
# - Buffer push operations
# - Any errors in _create() or _push_buffer()
```

### 2. Check vsockenc Source Code
The vsockenc element should:
- Have a socket read loop in a separate thread OR in the _create() method
- Read HelixFrameResponse messages from the socket
- Parse NAL units and push as GstBuffer downstream
- Set proper caps (video/x-h264) on source pad

Possible issues:
- vsockenc _create() method not reading from socket
- Socket not connected (but we see "Guest connected!" in QEMU logs)
- Response parsing broken (endianness, struct packing)
- Buffer push failing silently

### 3. Test Socket Connectivity
```bash
# Inside container, check if vsockenc has an open connection to 10.0.2.2:5900
lsof -i TCP:5900
netstat -an | grep 5900
```

### 4. Minimal Test - Bypass vsockenc
Temporarily modify pipeline to use regular encoder to confirm rest of stack works:
```
pipewiresrc ! videoconvert ! x264enc ! h264parse ! appsink
```

## Performance Expectations (Once Fixed)

- 60 FPS with active content (vkcube)
- 10 FPS with static screen (damage-based keepalive)
- <100ms latency (hardware encoding)
- Stable (no crashes - resource validation working)

## Files Modified

- `/Users/luke/pm/qemu-utm/hw/display/helix/helix-frame-export.m` - Added scanout fallback and debug logging
- Built, installed, and path-fixed QEMU dylib
- VM restarted with patched QEMU

## Test Session

- Session ID: ses_01kgrmckj836d5ge0ny474t09t
- Container: ubuntu-external-01kgrmckj836d5ge0ny474t09t
- QEMU sending frames successfully
- WebSocket client receiving 0 frames
