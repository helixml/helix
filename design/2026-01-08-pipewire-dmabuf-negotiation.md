# PipeWire DMA-BUF Format Negotiation

**Date:** 2026-01-08
**Status:** In Progress
**Goal:** Fix ~40fps cap on GNOME Ubuntu desktop streaming (should be 60fps)

## Problem Statement

GNOME ScreenCast via PipeWire delivers frames at ~40fps instead of 60fps. OBS achieves 60fps from the same source, proving the issue is in the consumer code (`pipewirezerocopysrc` GStreamer element).

Investigation revealed that despite CUDA mode initializing correctly, PipeWire was sending **SHM frames instead of DmaBuf frames**, causing the pipeline to use a slow fallback path instead of zero-copy GPU rendering.

## Architecture

```
GNOME 49 Mutter ScreenCast
    ↓ (PipeWire)
pipewirezerocopysrc (Rust/GStreamer)
    ↓ (DmaBuf or SHM)
EGLImage → CUDAImage → CUDA Buffer
    ↓
Wolf streaming pipeline
```

## Key Files

- `/prod/home/luke/pm/wolf/gst-pipewire-zerocopy/src/pipewire_stream.rs` - PipeWire stream handling
- `/prod/home/luke/pm/wolf/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs` - GStreamer source element
- `/tmp/obs-studio-src/plugins/linux-pipewire/pipewire.c` - OBS reference implementation

## Experiment Log

### Attempt 1: Increase PipeWire buffer count (Prior session)
- Changed `mpsc::sync_channel(2)` to `mpsc::sync_channel(8)`
- **Result:** Did not fix the framerate issue

### Attempt 2: Add VIDEO_RATE property (Prior session)
- Added `VIDEO_RATE => "60/1"` to stream properties
- **Result:** Did not fix the framerate issue

### Attempt 3: Add MUTTER_DEBUG_KMS_THREAD_TYPE=user workaround (Prior session)
- Added environment variable to fix Mutter frame scheduling
- Reference: https://gitlab.gnome.org/GNOME/mutter/-/issues/3788
- **Result:** Helped with frame drops but didn't fix underlying DmaBuf vs SHM issue

### Attempt 4: Add buffer params for DMA-BUF (Today)
- Added `build_buffer_params()` function to request `SPA_DATA_DmaBuf | SPA_DATA_MemPtr` buffer types
- Added `update_params()` call in `param_changed` callback (like OBS does)
- **Result:** Still receiving SHM frames

### Attempt 5: Add format/modifier constraints (Today)
- Studied OBS's `build_format()` function which creates one pod per format
- OBS includes modifier enum with `MANDATORY | DONT_FIXATE` flags
- Implemented single format pod with BGRx + DRM_FORMAT_MOD_INVALID

**Format pod structure:**
```
- mediaType: Video
- mediaSubtype: Raw
- videoFormat: BGRx (5)
- modifier: DRM_FORMAT_MOD_INVALID with flags 0x18
- framerate: Range 0-360 fps
```

**Result:** Format negotiation fails with "no more input formats"

PipeWire debug shows:
- Consumer offers: BGRx with modifier (flags 0x18)
- Producer offers: BGRx with modifier, BGRA with modifier, BGRx (no mod), BGRA (no mod)
- Negotiation fails at intersection

### Current Theory

The modifier values don't intersect:
- We request `DRM_FORMAT_MOD_INVALID` (0x00ffffffffffffff) as implicit modifier
- GNOME/Mutter likely offers specific NVIDIA tiled modifiers
- Since MANDATORY flag is set, negotiation fails if modifiers don't match

### Next Steps

1. **Option A:** Query available modifiers from EGL/DRM instead of hardcoding
   - OBS does this via `gs_query_dmabuf_modifiers_for_format()`
   - Would need to probe GPU for supported modifiers

2. **Option B:** Remove modifier constraint, let PipeWire/GNOME choose
   - Accept whatever format GNOME sends
   - Handle both DmaBuf and SHM in the process callback

3. **Option C:** Match OBS exactly - multiple format pods
   - Build one pod per format
   - First pass: formats with modifiers (DmaBuf)
   - Second pass: formats without modifiers (SHM fallback)
   - PipeWire tries in order

## OBS Reference Code

Key insight from OBS `build_format_params()`:

```c
// First, build pods WITH modifiers (DmaBuf capable)
for (size_t i = 0; i < format_info.num; i++) {
    if (format_info[i].modifiers.num == 0) continue;
    params[count++] = build_format(..., modifiers, modifier_count);
}

// Then, build pods WITHOUT modifiers (SHM fallback)
for (size_t i = 0; i < format_info.num; i++) {
    params[count++] = build_format(..., NULL, 0);
}
```

OBS also adds `DRM_FORMAT_MOD_INVALID` as implicit modifier if GPU supports it:
```c
if (dmabuf_flags & GS_DMABUF_FLAG_IMPLICIT_MODIFIERS_SUPPORTED) {
    uint64_t modifier_implicit = DRM_FORMAT_MOD_INVALID;
    da_push_back(info->modifiers, &modifier_implicit);
}
```

## Commits

- `[pending]` Initial format/modifier implementation
