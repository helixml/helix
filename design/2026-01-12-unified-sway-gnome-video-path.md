# Unified Video Capture Path for Sway and GNOME

**Date:** 2026-01-12
**Status:** Implemented (PipeWire SHM for Sway, DMA-BUF for GNOME, both use hardware encoding)
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

## wlr-export-dmabuf Implementation (2026-01-12)

### Implementation Complete

Added native wlr-export-dmabuf support to pipewirezerocopysrc, bypassing xdg-desktop-portal-wlr
entirely for Sway video capture.

**Files Added/Modified:**
- `desktop/gst-pipewire-zerocopy/src/wlr_export_dmabuf.rs` - New Wayland client for wlr-export-dmabuf
- `desktop/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs` - Detect Sway and use wlr-export-dmabuf
- `desktop/gst-pipewire-zerocopy/Cargo.toml` - Added wayland-client, wayland-protocols-wlr

### NVIDIA CUDA Modifier Compatibility Issue

**Problem Discovered:**
wlr-export-dmabuf captured frames successfully, but CUDA import failed:
```
[PIPEWIRESRC_DEBUG] CUDAImage error: CUDA_ERROR_INVALID_VALUE
```

**Root Cause Analysis:**
Sway uses NVIDIA modifier `0x300000000606014` while GNOME uses `0x300000000e08010`.
Decoded using `DRM_FORMAT_MOD_NVIDIA_BLOCK_LINEAR_2D(c, s, g, k, h)`:

| Field | Sway (0x606014) | GNOME (0xe08010) | Meaning |
|-------|-----------------|------------------|---------|
| c (compression) | 0 | 1 | ROP/3D compression enabled |
| s (sector layout) | 1 | 1 | Desktop (vs Tegra) |
| g (kind generation) | 2 | 2 | Same |
| k (page kind) | **6** | **8** | Different memory page kinds |
| h (block height) | **4** (16 GOBs) | **0** (1 GOB) | Very different block heights |

CUDA's `cuGraphicsEGLRegisterImage()` only supports specific page kinds/modifiers.

**Key Insight:**
The wlr-export-dmabuf protocol has **no format negotiation** - clients receive whatever modifier
the compositor uses. Unlike PipeWire ScreenCast which can negotiate formats, wlr-export-dmabuf
just exports the framebuffer as-is.

### Solution: WLR_DRM_NO_MODIFIERS=1

wlroots provides `WLR_DRM_NO_MODIFIERS=1` environment variable that forces all DRM plane allocations
to use LINEAR modifier (0x0). LINEAR is the baseline interchange format that CUDA can import.

**Trade-offs:**
- LINEAR may have slightly lower GPU performance than tiled formats
- For screen capture (read-only), performance impact is negligible
- Enables true zero-copy from Sway to CUDA/NVENC

### Files Modified

1. **Dockerfile.sway-helix:**
   ```dockerfile
   # Force LINEAR DRM modifiers for CUDA zero-copy compatibility.
   # Without this, Sway uses tiled modifiers (0x606xxx) that CUDA's
   # cuGraphicsEGLRegisterImage() rejects with CUDA_ERROR_INVALID_VALUE.
   ENV WLR_DRM_NO_MODIFIERS=1
   ```

2. **pipewiresrc/imp.rs:**
   - Removed Sway→DmaBuf mode switch (no longer needed with LINEAR)
   - Both GNOME and Sway use unified CUDA path

3. **ws_stream.go:**
   - Removed Sway-specific cudaupload handling
   - Unified zero-copy path: `pipewirezerocopysrc → nvh264enc`

### Final Architecture

```
GNOME (zero-copy via PipeWire):
  Mutter ScreenCast → PipeWire → pipewirezerocopysrc → EGL → CUDA → NVENC

Sway (zero-copy via wlr-export-dmabuf, LINEAR modifiers):
  Sway (WLR_DRM_NO_MODIFIERS=1) → wlr-export-dmabuf → pipewirezerocopysrc → EGL → CUDA → NVENC
```

Both paths use identical GStreamer pipeline from pipewirezerocopysrc onwards.

### References

- [wlroots env_vars.md](https://github.com/swaywm/wlroots/blob/master/docs/env_vars.md) - WLR_DRM_NO_MODIFIERS
- [wlr-export-dmabuf protocol](https://wayland.app/protocols/wlr-export-dmabuf-unstable-v1)
- [GStreamer DMA buffers](https://gstreamer.freedesktop.org/documentation/additional/design/dmabuf.html) - LINEAR modifier
- [NVIDIA DRM modifiers](https://docs.nvidia.com/drive/drive-os-6.0.4/linux/sdk/common/topics/graphics_content/NvKMS-BLOCK_LINEAR_2D-Modifier.html)

## WLR_DRM_NO_MODIFIERS Causes Zed Crash (2026-01-12)

### Problem

While `WLR_DRM_NO_MODIFIERS=1` successfully forces LINEAR modifier for CUDA compatibility,
it causes Zed to crash on startup with:

```
panicked at crates/gpui/src/platform/linux/platform.rs:64:64:
Unable to init GPU context: PortalNotFound(OwnedInterfaceName("org.freedesktop.portal.Notification"))
```

### Analysis

- Both old and new containers have identical portal logs with same errors
- Both show `glfw error: No such interface "org.freedesktop.portal.Settings"`
- Old container: Zed starts successfully despite portal errors (graceful degradation)
- New container: Zed crashes immediately

The only difference is `WLR_DRM_NO_MODIFIERS=1` in the environment. While this env var
should only affect DRM buffer allocation, it somehow triggers a crash path in Zed's
GPUI during GPU context initialization.

### Decision: Use PipeWire SHM Path for Sway

Given the Zed crash issue with `WLR_DRM_NO_MODIFIERS=1`, we chose the simpler PipeWire SHM approach:

1. **Remove `WLR_DRM_NO_MODIFIERS=1`** from Dockerfile.sway-helix
2. **Use PipeWire SHM** instead of wlr-export-dmabuf (simpler, unified PipeWire path)
3. **Pass `dmabuf_caps = None`** for Sway to force SHM-only format negotiation
4. **cudaupload** transfers SHM frames to GPU for hardware encoding

The wlr-export-dmabuf code remains in the codebase (committed in `a01d1640a`) but is no longer
used for Sway. PipeWire SHM is simpler and works reliably with xdg-desktop-portal-wlr.

### Final Architecture

```
GNOME (zero-copy via PipeWire DMA-BUF):
  Mutter ScreenCast → PipeWire → pipewirezerocopysrc → EGL → CUDA → NVENC

Sway (hardware encoding via PipeWire SHM):
  Sway → xdg-desktop-portal-wlr → PipeWire SHM → pipewirezerocopysrc → cudaupload → NVENC
```

**Trade-offs:**
- Sway has one CPU copy (SHM → cudaupload → CUDA memory)
- Still uses hardware NVENC encoding (not software x264)
- Much better than wf-recorder fallback (which may use software encoding)
- Avoids the Zed crash caused by WLR_DRM_NO_MODIFIERS
- Unified PipeWire path for both GNOME and Sway (just different format negotiation)

### Why Not wlr-export-dmabuf?

We implemented and tested wlr-export-dmabuf (commit `a01d1640a`) but hit blockers:
1. Sway uses NVIDIA modifiers (0x606xxx) that CUDA cannot import
2. `WLR_DRM_NO_MODIFIERS=1` forces LINEAR (CUDA-compatible) but crashes Zed
3. PipeWire SHM achieves the same goal (hardware encoding) with simpler code

## wlr-screencopy Implementation (2026-01-12)

### Problem with PipeWire Path

PipeWire + xdg-desktop-portal-wlr had format negotiation issues:
- Modifier negotiation complexity between our plugin and the portal
- Multiple format pods required (with/without modifiers)
- Race conditions in buffer allocation

### Solution: Native wlr-screencopy Protocol

Bypass PipeWire entirely for Sway using the native `wlr-screencopy-unstable-v1` Wayland protocol.
This is simpler and more reliable than xdg-desktop-portal-wlr + PipeWire.

**Files Added:**
- `desktop/gst-pipewire-zerocopy/src/wlr_screencopy.rs` - Wayland client for wlr-screencopy

**Protocol Flow:**
1. Connect to Wayland display via WAYLAND_DISPLAY
2. Bind `zwlr_screencopy_manager_v1` and `wl_shm` globals
3. Call `capture_output()` to request a frame
4. Receive `buffer` event with format, width, height, stride
5. Create memfd + wl_shm_pool + wl_buffer
6. Call `frame.copy(buffer)` to request the screenshot
7. Receive `ready` event when frame is captured
8. Copy SHM data to GStreamer buffer

**Detection:**
- Check `XDG_CURRENT_DESKTOP` for "sway" or "wlroots"
- If detected, use wlr-screencopy instead of PipeWire

**Architecture:**
```
Sway (via wlr-screencopy):
  Sway → wlr-screencopy (SHM) → pipewirezerocopysrc → cudaupload → NVENC
```

### Trade-offs

- Uses SHM (shared memory) not DMA-BUF - one CPU copy per frame
- Still uses hardware encoding (NVENC/VAAPI)
- Simpler than PipeWire + portal
- Works reliably on all wlroots compositors

## Future: ext-image-copy-capture-v1

The `ext-image-copy-capture-v1` protocol is a more modern, standardized replacement
for `wlr-screencopy-unstable-v1`. It's supported by Sway 1.10+ and KWin 6.2+.

**Key Benefits:**
1. **Damage tracking** - Only sends changed regions, reducing bandwidth for static screens
2. **Cross-compositor** - Works on Sway, KDE Plasma, and other compositors
3. **Standardized** - Part of wayland-protocols-extra (not wlroots-specific)
4. **Session-based** - Persistent capture sessions with better state management

**Damage Tracking Benefits:**
- Static desktops produce fewer frames
- Slow network connections can catch up during idle periods
- Reduced CPU/GPU usage when screen is static
- Better for metered connections

**Protocol Comparison:**

| Feature | wlr-screencopy | ext-image-copy-capture |
|---------|---------------|----------------------|
| Compositor | wlroots only | Sway 1.10+, KDE 6.2+ |
| Damage | No | Yes |
| Session | Per-frame | Persistent |
| Stability | Unstable | Stable |
| Memory | SHM or DMA-BUF | SHM or DMA-BUF |

**Implementation Priority:**
For now, wlr-screencopy works and is widely supported. ext-image-copy-capture-v1
can be added as an enhancement when we need damage-based streaming.

## Related Issues

- wf-recorder may silently fall back to libx264 if h264_nvenc unavailable
- No verification logging of which encoder wf-recorder actually uses
- Different bitrate/quality settings between wf-recorder and GStreamer paths
