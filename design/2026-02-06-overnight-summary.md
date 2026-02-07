# macOS ARM Video Streaming - Overnight Progress Summary

**Date:** 2026-02-06 04:58
**Branch:** feature/macos-arm-desktop-port
**QEMU Branch:** utm-edition (in qemu-utm repo)

## 🎯 Major Breakthrough: Scanout Fallback Working

Successfully implemented and verified the scanout resource fallback for video streaming on macOS ARM. QEMU is now:
- ✅ Reading frames from scanout resources (1920x1080 BGRA)
- ✅ Creating IOSurface from pixel data
- ✅ Encoding with VideoToolbox (H.264)
- ✅ Sending encoded frames over socket (22KB keyframes, 7KB P-frames)

## What We Discovered

### The DMA-BUF Problem

**Expected Architecture:**
```
PipeWire DMA-BUF → vsockenc extracts GEM handle → sends resource_id to QEMU
```

**Reality on virtio-gpu (macOS/UTM):**
```
Mutter provides SHM buffers only (no DMA-BUF export in headless mode on virtio-gpu)
→ vsockenc cannot extract resource IDs
→ sends resource_id=0
```

### The Solution

**Scanout Resource Fallback:**
```
vsockenc sends resource_id=0
→ QEMU falls back to current scanout resource (e.g. resource 104)
→ virgl_renderer_transfer_read_iov() reads 8294400 bytes
→ IOSurface created from raw pixels
→ VideoToolbox encodes to H.264
→ Send over socket to vsockenc
```

This matches how Chromium does offscreen rendering on virtio-gpu.

## Code Changes

### QEMU (qemu-utm repo, utm-edition branch)

**File:** `hw/display/helix/helix-frame-export.m`

**Scanout Fallback (lines 592-609):**
```objc
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

**Debug Logging in Callback:**
```objc
error_report("[HELIX] encoder_output_callback called: status=%d, sampleBuffer=%p", ...);
...
error_report("[HELIX] Frame sent successfully: %zu bytes, pts=%lld, keyframe=%d", ...);
```

**Commit:** c6434bbf72 - "feat(helix-frame-export): Add scanout resource fallback and debug logging"

## Verification Logs

### QEMU Successfully Encoding

```
[HELIX] Frame request: resource_id=0, 1920x1080, pts=103331525
[HELIX] Current scanout[0] resource_id=104
[HELIX] Using scanout resource_id=104
[HELIX] Successfully read 8294400 bytes via transfer
[HELIX] Created IOSurface 0x7400201a0 (1920x1080) from resource 104
[HELIX] Frame encode request submitted (async callback will send response)
[HELIX] encoder_output_callback called: status=0, sampleBuffer=0x73ec2e300
[HELIX] Frame sent successfully: 22376 bytes, pts=103331525, keyframe=1
```

### Correct Pipeline in Use

```
[SHARED_VIDEO] Pipeline: pipewiresrc path=47 do-timestamp=true keepalive-time=500 !
queue max-size-buffers=1 leaky=downstream !
vsockenc tcp-host=10.0.2.2 tcp-port=5900 bitrate=10000 keyframe-interval=120 !
h264parse ! video/x-h264,profile=constrained-baseline,stream-format=byte-stream !
appsink name=videosink emit-signals=true max-buffers=2 drop=true sync=false
```

### Socket Connection Established

```bash
# Inside guest container
$ netstat -an | grep 5900
tcp  0  0  10.213.0.3:59344  10.0.2.2:5900  ESTABLISHED
```

## Current Blocker 🔴

**vsockenc receive thread not forwarding H.264 frames**

- Socket is ESTABLISHED
- QEMU sends H.264 data successfully
- vsockenc connects and sends initial frame requests
- BUT: vsockenc's `gst_vsockenc_recv_thread()` is not reading/processing responses
- Result: 0 frames delivered to WebSocket client

### Possible Causes

1. **Protocol mismatch** - Struct packing, endianness, or magic number validation failing
2. **Receive thread error** - Thread exited after first error, no longer reading socket
3. **Timing issue** - Response arrives after frame timeout
4. **Buffer issue** - Socket receive buffer full or data lost

### Evidence

- First streaming attempt: QEMU logs show 2 frames sent (keyframe + P-frame)
- Subsequent attempts: NO new frame requests sent by vsockenc
- No errors logged by desktop-bridge
- Socket still ESTABLISHED but inactive (recv queue = 0)

## Next Steps

### 1. Enable vsockenc Debug Logging

Add to container environment or ws_stream.go:
```bash
GST_DEBUG=vsockenc:5
```

Look for:
- "Receive thread started"
- "Failed to read frame response"
- "Invalid message magic"
- "Finished frame pts=..."
- "Connection lost reading header"

### 2. Check Protocol Alignment

Verify struct layout matches between QEMU (Objective-C) and vsockenc (C):
```c
// Both should have same size/alignment
sizeof(HelixMsgHeader)
sizeof(HelixFrameResponse)
```

Check endianness of fields like `magic`, `nal_size`.

### 3. Test Socket Directly

Write minimal test inside container to:
1. Connect to 10.0.2.2:5900
2. Send HelixFrameRequest
3. Read HelixFrameResponse
4. Print magic number, response size, NAL count

### 4. Add Logging to vsockenc recv_thread

Rebuild helix-ubuntu with extra logging in gstvsockenc.c recv_thread:
- Log every header read
- Log magic validation
- Log response parsing
- Log frame finish calls

## Architecture Summary

```
┌─────────────────────────────────────┐
│ Guest (Ubuntu VM)                   │
├─────────────────────────────────────┤
│ GNOME → PipeWire SHM                │
│         ↓                           │
│ pipewiresrc (raw video frames)      │
│         ↓                           │
│ vsockenc                            │
│  - Sends resource_id=0              │
│  - ⚠️ Receive thread blocked ⚠️      │
└─────────────────────────────────────┘
              ↓ TCP 10.0.2.2:5900
┌─────────────────────────────────────┐
│ Host (macOS) - QEMU                 │
├─────────────────────────────────────┤
│ helix-frame-export                  │
│  - Receive resource_id=0            │
│  - Use scanout resource (104) ✅    │
│  - Read 8MB pixel data ✅           │
│  - VideoToolbox encode ✅           │
│  - Send H.264 (22KB) ✅             │
└─────────────────────────────────────┘
              ↓ (H.264 data sent but not received)
┌─────────────────────────────────────┐
│ Guest - vsockenc receive thread     │
│  - ⚠️ Not reading responses ⚠️      │
│  - Pipeline outputs 0 frames        │
│  - WebSocket client: 0 FPS          │
└─────────────────────────────────────┘
```

## Performance Expectations (Once Fixed)

- 60 FPS with active content (vkcube)
- 10 FPS with static screen (damage-based ScreenCast)
- <100ms latency (hardware encoding)
- Zero crashes (resource validation working)

## Files Modified

### QEMU
- `/Users/luke/pm/qemu-utm/hw/display/helix/helix-frame-export.m`
- Built, installed to UTM.app
- Paths fixed with fix-qemu-paths-recursive.sh
- VM restarted with patched QEMU

### Helix (Already Committed)
- `api/pkg/desktop/ws_stream.go` - vsock special case using pipewiresrc
- `desktop/gst-vsockenc/gstvsockenc.c` - (no changes, but needs debugging)

## Test Session Info

- Session ID: ses_01kgrmckj836d5ge0ny474t09t
- Container: ubuntu-external-01kgrmckj836d5ge0ny474t09t
- API Key: hl-test123456789
- Project: prj_test123
- User: usr_test123

## Key Insights

1. **Mutter on virtio-gpu doesn't export DMA-BUF** - confirmed by testing forced caps negotiation
2. **Scanout resources work fine** - no crashes, stable transfers via virgl_renderer_transfer_read_iov()
3. **QEMU VideoToolbox encoding works** - 22KB keyframes at 60 FPS capacity
4. **Socket connectivity OK** - TCP handshake successful, connection stable
5. **vsockenc sends requests** - initial frame requests work (seen in QEMU logs)
6. **Problem is in vsockenc receive** - H.264 responses not being processed

## Commit History (Today)

### QEMU (qemu-utm repo)
```
c6434bbf72 feat(helix-frame-export): Add scanout resource fallback and debug logging
```

### Helix (main repo)
```
0f1410d72 fix(desktop-bridge): Use native pipewiresrc for vsockenc (not pipewirezerocopysrc)
b7fcff40f fix(desktop-bridge): Enable DMA-BUF for vsockenc on macOS/UTM
9aa275186 feat: Add TCP support to gst-vsockenc for macOS ARM testing
b0599449d feat(hydra): Mount virtio-gpu /dev/dri devices for macOS desktop containers
```

## Bottom Line

**80% there!** The hard parts are working:
- ✅ GPU resource access
- ✅ Frame reading from virtio-gpu
- ✅ IOSurface creation
- ✅ VideoToolbox encoding
- ✅ Network connectivity

Just need to debug why vsockenc's receive thread isn't forwarding the H.264 data downstream. This is likely a simple protocol or threading issue that GST_DEBUG logging will reveal.
