# Pixel Format Flow Analysis for Video Streaming

**Date:** 2026-01-13
**Status:** Analysis
**Related:** `desktop/gst-pipewire-zerocopy/`, `api/pkg/desktop/ws_stream.go`

## Overview

This document analyzes the exact pixel formats that flow through the video streaming pipeline for all 4 supported configurations. Understanding this is critical for debugging format negotiation failures and ensuring zero-copy paths work correctly.

## The 4 Cases

| Case | Compositor | GPU | capture-source | buffer-type | Output Mode |
|------|-----------|-----|----------------|-------------|-------------|
| 1 | GNOME | NVIDIA | pipewire | dmabuf | CUDAMemory |
| 2 | GNOME | AMD/Intel | pipewire | shm | System |
| 3 | Sway | NVIDIA | wayland | shm | System |
| 4 | Sway | AMD/Intel | wayland | shm | System |

---

## Case 1: GNOME + NVIDIA (Zero-Copy CUDA)

### Capture Source
- **Protocol:** PipeWire ScreenCast via xdg-desktop-portal-gnome
- **Buffer Type:** DmaBuf with NVIDIA tiled modifiers

### Format Flow

```
GNOME Mutter
    ↓
    Offers: BGRx/BGRA with NVIDIA modifiers (0x300000000e08xxx)
    ↓
PipeWire Stream (pipewire_stream.rs)
    ↓
    Negotiates: BGRx (SPA format 8) → ARGB8888 (DRM fourcc 0x34325241)
    Note: BGRx is remapped to ARGB8888 for CUDA compatibility
    Modifier: 0x300000000e08014 (NVIDIA block-linear tiled)
    ↓
pipewirezerocopysrc (FrameData::DmaBuf)
    ↓
    Creates smithay::Dmabuf with:
    - fourcc: ARGB8888 (0x34325241)
    - modifier: NVIDIA tiled
    - fd: DmaBuf file descriptor
    ↓
CUDA Import (CUDAImage::from)
    ↓
    EGLImage import: dmabuf → EGL → CUDA texture
    Output: video/x-raw(memory:CUDAMemory),format=BGRA
    ↓
nvh264enc
    ↓
    Internal colorspace conversion on GPU
    Output: video/x-h264
```

### Known Facts
- ✅ NVIDIA modifiers (0xe08xxx family) are required for CUDA import
- ✅ BGRx → ARGB8888 remapping works (CUDA rejects xRGB/BGRx with tiled modifiers)
- ✅ nvh264enc handles BGRA→NV12 conversion internally on GPU

### Unknowns / Risks
- ⚠️ **DRM fourcc vs GStreamer format confusion**: Code maps BGRx→ARGB8888 for DRM, then uses `VideoFormat::Bgra` for GStreamer caps. This works but is confusing because the byte order naming differs.
- ⚠️ **Modifier support varies by driver version**: Testing required on different NVIDIA driver versions.

---

## Case 2: GNOME + AMD/Intel (SHM → VA-API)

### Capture Source
- **Protocol:** PipeWire ScreenCast via xdg-desktop-portal-gnome
- **Buffer Type:** MemFd (SHM)

### Format Flow

```
GNOME Mutter
    ↓
    Offers: BGRx/BGRA (32-bit formats)
    Buffer type: MemFd (SHM, mapped memory)
    ↓
PipeWire Stream (pipewire_stream.rs)
    ↓
    Negotiates: BGRx (SPA format 8)
    DRM fourcc: ARGB8888 (0x34325241) - same remapping as NVIDIA
    Modifier: 0x0 (LINEAR) or absent
    ↓
pipewirezerocopysrc (FrameData::Shm)
    ↓
    Output: video/x-raw,format=??? (see below)
    System memory buffer
    ↓
queue
    ↓
vapostproc
    ↓
    GPU upload + colorspace conversion
    Output: video/x-raw,format=NV12
    ↓
vah264enc
    ↓
    Output: video/x-h264
```

### Format Mapping Issue ⚠️

The current code has a potential issue:

```rust
// In imp.rs create_system_buffer():
// format comes from params.format which is DRM fourcc (ARGB8888 = 0x34325241)
// But VideoMeta expects GStreamer VideoFormat enum!

fn drm_fourcc_to_video_format(fourcc: DrmFourcc) -> VideoFormat {
    match fourcc {
        DrmFourcc::Argb8888 => VideoFormat::Bgra,  // This is the mapping
        ...
    }
}
```

**Flow:**
1. PipeWire sends BGRx (SPA format)
2. `spa_video_format_to_drm_fourcc` maps BGRx → ARGB8888 (DRM)
3. `drm_fourcc_to_video_format` maps ARGB8888 → Bgra (GStreamer)
4. GStreamer buffer has `format=Bgra`

**Is this correct?**
- DRM ARGB8888: Memory layout is `[A][R][G][B]` (little-endian: B, G, R, A in byte order)
- GStreamer BGRA: Memory layout is `[B][G][R][A]` in byte order

This appears correct because both refer to the same byte layout. The naming difference is:
- DRM names by the 32-bit word interpretation on little-endian
- GStreamer names by the byte order

### Known Facts
- ✅ SHM path works (screenshot server uses this)
- ✅ vapostproc handles BGRA→NV12 conversion

### Unknowns / Risks
- ⚠️ **AMD headless GNOME may ONLY support DmaBuf**: Previous errors showed "error alloc buffers: Invalid argument" when requesting MemFd. Current code requests both MemFd+DmaBuf and lets PipeWire choose.
- ⚠️ **Need to verify the actual format Mutter sends in SHM mode**: Could be BGRx, BGRA, or something else.

---

## Case 3: Sway + NVIDIA (ext-image-copy-capture → cudaupload)

### Capture Source
- **Protocol:** ext-image-copy-capture-v1 Wayland protocol
- **Buffer Type:** wl_shm (always SHM, no DmaBuf option in this protocol)

### Format Flow

```
Sway Compositor
    ↓
    Offers SHM formats via session.shm_format event
    Typical formats: Xrgb8888, Argb8888, Xbgr8888, Abgr8888
    ↓
ext_image_copy_capture.rs
    ↓
    Selects format (prefers Xrgb8888 or Argb8888)
    Creates wl_shm_pool + wl_buffer
    ↓
    Receives frame into SHM buffer
    Converts wl_shm::Format → DRM fourcc:
      wl_shm::Xrgb8888 → Fourcc::Xrgb8888 (0x34325258)
    ↓
pipewirezerocopysrc (FrameData::Shm)
    ↓
    drm_fourcc_to_video_format: Xrgb8888 → VideoFormat::Bgrx
    Output: video/x-raw,format=Bgrx (32-bit, system memory)
    ↓
queue
    ↓
videoconvert
    ↓
    Output: video/x-raw,format=RGBA
    (Required because cudaupload needs 32-bit RGBA)
    ↓
cudaupload
    ↓
    Output: video/x-raw(memory:CUDAMemory),format=RGBA
    ↓
nvh264enc
    ↓
    Output: video/x-h264
```

### Known Facts
- ✅ ext-image-copy-capture always uses SHM (no DmaBuf option in protocol v1)
- ✅ Sway outputs 32-bit formats (Xrgb8888 is preferred)
- ✅ videoconvert is needed before cudaupload

### Unknowns / Risks
- ⚠️ **24-bit formats possible**: ext-image-copy-capture CAN output RGB888/BGR888 (24-bit). The current code handles this, but ws_stream.go's pipeline may not have the right caps filter.
- ⚠️ **Format preference**: Code prefers Xrgb8888 but Sway might offer different formats on different setups.

**24-bit Format Handling:**
```rust
// In ext_image_copy_capture.rs bytes_per_pixel():
wl_shm::Format::Bgr888 | wl_shm::Format::Rgb888 => 3,  // 24-bit

// In wl_shm_to_drm_fourcc():
wl_shm::Format::Rgb888 => Fourcc::Rgb888 as u32,  // 0x34324752
wl_shm::Format::Bgr888 => Fourcc::Bgr888 as u32,  // 0x34324742

// In pipewire_stream.rs spa_video_format_to_drm_fourcc():
spa::param::video::VideoFormat::RGB => 0x34324752, // RG24 = RGB888
spa::param::video::VideoFormat::BGR => 0x34324742, // BG24 = BGR888

// In imp.rs drm_fourcc_to_video_format():
DrmFourcc::Bgr888 => VideoFormat::Rgb,  // Note: swapped!
DrmFourcc::Rgb888 => VideoFormat::Bgr,
```

The swap for 24-bit formats looks intentional but is confusing. Need to verify.

---

## Case 4: Sway + AMD/Intel (ext-image-copy-capture → VA-API)

### Capture Source
- **Protocol:** ext-image-copy-capture-v1 Wayland protocol
- **Buffer Type:** wl_shm (always SHM)

### Format Flow

```
Sway Compositor
    ↓
    Offers SHM formats via session.shm_format event
    ↓
ext_image_copy_capture.rs
    ↓
    Same as Case 3 up to pipewirezerocopysrc
    ↓
pipewirezerocopysrc (FrameData::Shm)
    ↓
    Output: video/x-raw,format=Bgrx (or Bgra, depending on negotiation)
    ↓
queue
    ↓
vapostproc
    ↓
    GPU upload + colorspace conversion
    Output: video/x-raw,format=NV12
    ↓
vah264enc
    ↓
    Output: video/x-h264
```

### Known Facts
- ✅ Same capture path as Case 3
- ✅ vapostproc handles the GPU upload and conversion

### Unknowns / Risks
- ⚠️ **Same 24-bit format risk as Case 3**

---

## Format Mapping Tables

### DRM Fourcc ↔ GStreamer VideoFormat

| DRM Fourcc | DRM Code | GStreamer Format | Byte Order |
|------------|----------|------------------|------------|
| ARGB8888 | 0x34325241 | Bgra | B, G, R, A |
| ABGR8888 | 0x34324241 | Rgba | R, G, B, A |
| XRGB8888 | 0x34325258 | Bgrx | B, G, R, X |
| XBGR8888 | 0x34324258 | Rgbx | R, G, B, X |
| BGRA8888 | 0x34324142 | Argb | A, R, G, B |
| RGBA8888 | 0x34324152 | Abgr | A, B, G, R |
| RGB888 | 0x34324752 | Bgr | B, G, R |
| BGR888 | 0x34324742 | Rgb | R, G, B |

Note: The naming is confusing because DRM and GStreamer use different conventions. DRM names by the 32-bit register value on little-endian, GStreamer names by byte order.

### SPA Video Format ↔ DRM Fourcc (Current Mapping)

| SPA Format | SPA Raw | DRM Fourcc | Notes |
|------------|---------|------------|-------|
| BGRA | 12 | BGRA8888 (0x34324142) | Direct mapping |
| RGBA | 14 | RGBA8888 (0x34324152) | Direct mapping |
| BGRx | 8 | ARGB8888 (0x34325241) | **Remapped for CUDA!** |
| RGBx | 10 | ABGR8888 (0x34324241) | **Remapped for CUDA!** |
| ARGB | 7 | ARGB8888 (0x34325241) | Direct mapping |
| ABGR | 8 | ABGR8888 (0x34324241) | Direct mapping |
| xRGB | 9 | XRGB8888 (0x34325258) | Direct mapping |
| xBGR | 10 | XBGR8888 (0x34324258) | Direct mapping |

**BGRx → ARGB8888 Remapping Rationale:**
- CUDA's EGL image import rejects XRGB8888 and BGRX8888 with NVIDIA tiled modifiers
- CUDA accepts ARGB8888 with the same modifiers
- Since BGRx and ARGB8888 have the same byte layout (B, G, R, X/A), this works

---

## Open Questions

### 1. AMD Headless GNOME Buffer Types
**Question:** Does AMD headless GNOME require DmaBuf, or can it use MemFd?

**Current code:**
```rust
// pipewire_stream.rs build_negotiation_params()
let buffer_types: i32 = (1 << 2) | (1 << 3); // MemFd | DmaBuf = 4 | 8 = 12
```
We request both and let PipeWire choose. Need to verify which one is actually used.

### 2. 24-bit Format Support in GStreamer Pipeline
**Question:** Does the GStreamer pipeline in ws_stream.go handle 24-bit formats correctly?

**Current pipeline (Case 3/4):**
```
pipewirezerocopysrc ! queue ! videoconvert ! video/x-raw,format=RGBA ! cudaupload
```

If ext-image-copy-capture returns RGB888 (24-bit), videoconvert should handle it, but we should verify.

### 3. EGL Image Format for CUDA Import
**Question:** What EGL format is used for the CUDA import, and does it match?

The CUDA import path is:
```rust
// In cuda.rs (not in this crate, in waylanddisplaycore)
EGLImage::from(&dmabuf, &raw_display)
CUDAImage::from(egl_image, &cuda_ctx)
```

Need to verify the EGL format attributes match what GNOME sends.

### 4. Alpha Channel Handling
**Question:** Are alpha channels handled correctly, or do we need to strip/add them?

- GNOME may send BGRx (no alpha) or BGRA (with alpha)
- Encoders expect NV12 (no alpha)
- Is the alpha channel preserved until vapostproc/cudaconvert strips it?

---

## Recommendations

1. **Add debug logging for actual negotiated formats** - Log the exact format at each stage when `GST_DEBUG=pipewirezerocopysrc:5` is set.

2. **Test 24-bit format handling** - Force Sway to output BGR888 and verify the pipeline handles it.

3. **Verify AMD GNOME buffer type** - Add logging to show which buffer type (MemFd or DmaBuf) was actually selected by PipeWire.

4. **Document the format naming convention** - The DRM vs GStreamer naming is confusing and should be documented in code comments.
