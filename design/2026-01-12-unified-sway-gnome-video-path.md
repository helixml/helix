# Unified Video Capture Path for Sway and GNOME

**Date:** 2026-01-12
**Status:** Implemented
**Author:** Luke (with Claude)

## Problem

Sway uses a different video capture path than GNOME, resulting in:
1. **Likely software encoding** - wf-recorder may fall back to libx264
2. **Multiple CPU copies** - wlr-screencopy → wf-recorder → FIFO → GStreamer
3. **No DMA-BUF zero-copy** - frames copied through CPU memory
4. **Dual maintenance** - two encoder paths to configure and debug
5. **Inconsistent latency** - different pipeline characteristics

### Current Architecture

```
GNOME (zero-copy):
  Mutter ScreenCast → PipeWire → pipewirezerocopysrc → DMA-BUF → CUDA → NVENC

Sway (CPU copies):
  Sway → wlr-screencopy → wf-recorder (ffmpeg) → FIFO → filesrc → h264parse → appsink
```

### Why Sway Uses wf-recorder

The stock `pipewiresrc` GStreamer element hangs during format negotiation with
xdg-desktop-portal-wlr's PipeWire streams. This was documented as a compatibility
issue and led to the wf-recorder fallback.

## Proposed Solution

Try using `pipewirezerocopysrc` (our custom Rust plugin) with Sway's portal PipeWire
node. Our plugin handles format negotiation differently and may work where stock
pipewiresrc fails.

### Benefits

1. **Unified path** - One set of encoder configs for both compositors
2. **Zero-copy** - DMA-BUF directly from compositor to GPU encoder
3. **Hardware encoding** - Same NVENC/VAAPI path for both
4. **Consistent latency** - Same pipeline characteristics
5. **Less code** - Remove wf-recorder fallback path

### Implementation Options

#### Option 1: Test pipewirezerocopysrc with Sway Portal (Lowest Risk)

The xdg-desktop-portal-wlr already creates a PipeWire stream. We already call
the portal APIs and get a node ID. Currently we don't use it - we start
wf-recorder instead.

**Test:** Modify `desktop.go` to try pipewirezerocopysrc first, fall back to
wf-recorder only if it fails.

```go
// In desktop.go setupSwaySession()
// Instead of immediately using wf-recorder, try zero-copy first:
if tryZeroCopyWithPortal(s.pipeWireNodeID, s.pipeWireFd) {
    // Success! Use normal GStreamer pipeline
    s.nodeID = s.pipeWireNodeID
    s.pipeWireFd = s.portalFd
    // Don't set up video forwarder
} else {
    // Fall back to wf-recorder
    s.videoForwarder = NewVideoForwarderForSway(shmSocketPath, s.logger)
}
```

#### Option 2: Use wlr-export-dmabuf Protocol Directly

wlroots has `wlr-export-dmabuf-unstable-v1` protocol that exports DMA-BUF frames
directly without going through PipeWire. This bypasses the portal entirely.

**Implementation:** Create a new GStreamer element `wlrootssrc` that:
1. Connects to Wayland compositor
2. Binds wlr-export-dmabuf-manager-v1
3. Requests frame export
4. Receives DMA-BUF fds
5. Outputs to GStreamer as video/x-raw(memory:DMABuf)

This is more work but gives direct control over the capture.

#### Option 3: Use wlr-screencopy with DMA-BUF

The `wlr-screencopy-unstable-v1` protocol (what wf-recorder uses) can also export
DMA-BUF frames. A custom GStreamer element could use this.

**Implementation:** Similar to Option 2, but uses wlr-screencopy protocol.

### Recommendation

**Start with Option 1** - minimal code change to test if pipewirezerocopysrc works
with Sway's portal stream. If format negotiation still hangs, investigate why.

If Option 1 fails, **consider Option 2** for true zero-copy without PipeWire
dependency on Sway.

## Test Results (2026-01-12)

### Initial Test: Portal PipeWire Path FAILED

The initial attempt to use pipewirezerocopysrc with xdg-desktop-portal-wlr's
PipeWire stream **failed**. Format negotiation never completed:

```
[PIPEWIRE_DEBUG] PipeWire state: Unconnected -> Connecting
[PIPEWIRE_DEBUG] PipeWire state: Connecting -> Paused
[PIPEWIRE_DEBUG] DMA-BUF available with 7 modifiers, offering DmaBuf ONLY (no SHM fallback)
  offer[0] = 0x300000000e08010 (NVIDIA tiled)
  offer[1] = 0x300000000e08011
  ...

Video frames: 0 (param_changed callback NEVER called!)
```

**Root Cause:** xdg-desktop-portal-wlr does NOT support NVIDIA tiled modifiers.
It only supports modifiers that wlr-screencopy can produce. The PipeWire stream
from xdg-desktop-portal-wlr is a passthrough of wlr-screencopy frames.

### Key Insight: wlroots DMA-BUF Protocols

For true zero-copy with wlroots compositors (Sway), we need to use the native
Wayland protocols rather than going through xdg-desktop-portal-wlr's PipeWire:

1. **`wlr-export-dmabuf-unstable-v1`** - Primary protocol for DMA-BUF frame export
   - Allows a client to receive DMA-BUF file descriptors directly from the compositor
   - Provides full frames with GPU-accessible buffers
   - No PipeWire intermediary - direct compositor → client path

2. **`wlr-screencopy-unstable-v1`** with `linux-dmabuf`** - DMA-BUF via screencopy
   - Recent versions support linux-dmabuf parameters for zero-copy capture
   - Can request DMA-BUF instead of SHM buffers
   - Same protocol wf-recorder uses, but with DMA-BUF mode

### Revised Architecture

```
GNOME (zero-copy via PipeWire):
  Mutter ScreenCast → PipeWire → pipewirezerocopysrc → DMA-BUF → CUDA → NVENC

Sway (zero-copy via Wayland protocols):
  Option A: Sway → wlr-export-dmabuf → wlrexportdmabufsrc → DMA-BUF → CUDA → NVENC
  Option B: Sway → wlr-screencopy(dmabuf) → wlrscreencopysrc → DMA-BUF → CUDA → NVENC
```

### Next Steps

1. **Implement `wlrexportdmabufsrc` GStreamer element** using wlr-export-dmabuf-unstable-v1
   - Connect to Wayland compositor via WAYLAND_DISPLAY
   - Bind zwlr_export_dmabuf_manager_v1
   - Request frame capture, receive DMA-BUF fds
   - Output to GStreamer as video/x-raw(memory:DMABuf)

2. **Alternative: Implement `wlrscreencopysrc`** with DMA-BUF mode
   - Use wlr-screencopy-unstable-v1 with linux-dmabuf
   - May be simpler as wlr-screencopy is more widely supported

3. **Both approaches bypass xdg-desktop-portal-wlr entirely** for video capture

## Testing Plan

1. **Verify current Sway encoding:**
   ```bash
   # In Sway container, check if NVENC is actually being used
   ps aux | grep wf-recorder
   strace -e write -p <pid>  # See if writing to NVENC or CPU encoding
   ```

2. **Test pipewirezerocopysrc with Sway:**
   ```bash
   # Manually test if our plugin works with portal node
   gst-launch-1.0 pipewirezerocopysrc pipewire-node-id=<NODE> \
     ! video/x-raw ! videoconvert ! autovideosink
   ```

3. **Compare latency:**
   ```bash
   helix spectask latency <sway-session> --tests 20
   # Compare with GNOME session
   ```

## Files to Modify

- `api/pkg/desktop/desktop.go` - Try zero-copy before wf-recorder fallback
- `api/pkg/desktop/ws_stream.go` - No changes if portal stream works
- `api/pkg/desktop/video_forwarder.go` - Keep as fallback, may remove later

## AMD/Intel Zero-Copy VA-API Path (2026-01-12)

### Problem

The initial AMD/Intel VA-API implementation used `format=DMA_DRM` caps with `glupload/gldownload`
before `vapostproc`. This was an unnecessary intermediate step.

### Solution: Match Wolf's Approach

Wolf's waylanddisplaysrc outputs DMABuf with **regular video format** (not DMA_DRM):

```
waylanddisplaysrc ! video/x-raw(memory:DMABuf) ! vapostproc ! video/x-raw(memory:VAMemory),format=NV12 ! vah265enc
```

vapostproc accepts `video/x-raw(memory:DMABuf)` with regular formats (BGRx, BGRA) directly.
It does NOT accept `format=DMA_DRM`.

### Implementation

1. **pipewirezerocopysrc caps** - Output regular format with DMABuf feature:
   ```
   video/x-raw(memory:DMABuf),format=BGRx,width=1920,height=1080
   ```
   NOT:
   ```
   video/x-raw(memory:DMABuf),format=DMA_DRM,drm-format=XR24:0x...
   ```

2. **GStreamer pipeline** - Direct DMABuf to vapostproc:
   ```
   pipewirezerocopysrc ! vapostproc ! video/x-raw(memory:VAMemory),format=NV12 ! vah264enc
   ```
   No glupload/gldownload needed.

### Files Modified

- `desktop/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs`:
  - pad_templates(): DMABuf caps now use `format_list(rgba_formats)` not `format(DmaDrm)`
  - caps(): Same change for DmaBuf output mode
  - create(): DmaBuf caps update uses regular format, not VideoInfoDmaDrm

- `api/pkg/desktop/ws_stream.go`:
  - Removed glupload/gldownload from VA-API pipeline paths
  - vapostproc connects directly to pipewirezerocopysrc output

### Pipeline Comparison

```
NVIDIA (CUDA path):
  pipewirezerocopysrc ! video/x-raw(memory:CUDAMemory) ! nvh264enc

AMD/Intel (VA-API path):
  pipewirezerocopysrc ! video/x-raw(memory:DMABuf) ! vapostproc ! vah264enc
```

Both are true zero-copy - no CPU involvement in frame transfer.

## Related Issues

- wf-recorder may silently fall back to libx264 if h264_nvenc unavailable
- No verification logging of which encoder wf-recorder actually uses
- Different bitrate/quality settings between wf-recorder and GStreamer paths
