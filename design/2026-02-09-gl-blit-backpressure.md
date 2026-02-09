# GL Blit Sync for H.264 Encoding

**Date:** 2026-02-09
**Status:** IOSurfaceLock fence only (backpressure removed after crash)

## Problem

The GL blit path for H.264 frame capture has corruption because
`glFlush()` submits ANGLE's Metal commands but doesn't wait for
completion. We immediately pass the IOSurface to VideoToolbox, which
reads it from a *different* Metal command queue. Metal does NOT
synchronize across command queues automatically.

## What SPICE Actually Does

SPICE's `qemu_spice_gl_update()` in `ui/spice-display.c` (CONFIG_IOSURFACE path):

```
1. spice_iosurface_blit()     — GL blit virgl tex → IOSurface
2. gl_block(true)             — freeze virtio-gpu command queue
3. glFlush()                  — submit GL commands (NOT glFinish)
4. spice_qxl_gl_draw_async()  — hand off to SPICE client
5. [later] gl_draw_done()     — client done → BH → gl_block(false)
```

SPICE does NOT use IOSurfaceLock or glFinish. It relies on implicit
latency between glFlush and the SPICE client reading the IOSurface
(at least one display refresh interval ~16ms).

SPICE does NOT use both mechanisms — only gl_block + glFlush.

## Action Log

### Commit 9e19517a12 — Zero-copy GL blit with triple-buffered ring

Replaced the dead Metal IOSurface snapshot path (Venus/KosmicKrisp returns
NULL Metal handles) with GL blit matching SPICE's approach:

```
virgl tex_id → [glBlitFramebuffer] → IOSurface[ring_slot]
→ [CVPixelBufferCreateWithIOSurface] → VTCompressionSession → H.264
```

- EGL context shares with `spice_gl_ctx` (correct share group)
- Triple-buffered ring (3 IOSurface+FBO pairs) prevents VT async race
- `CVPixelBufferCreateWithIOSurface` wraps directly (no CPU memcpy)
- Removed old Metal IOSurface snapshot code + CPU memcpy

### Commit 535af85418 — Added backpressure + IOSurfaceLock fence

Added two sync mechanisms:
1. `helix_gl_block(true/false)` around the blit
2. `IOSurfaceLock(kIOSurfaceLockReadOnly)` + unlock after glFlush as GPU fence

**Result: QEMU crashed** on frame 2. "QEMU exited from an error."

### Commit da57c68e1e — Removed backpressure, kept IOSurfaceLock only

Root cause of crash: `helix_scanout_frame_ready` is called from inside
`virtio_gpu_process_cmdq` (via `virgl_cmd_set_scanout`). Calling
`helix_gl_block(false)` triggers `virtio_gpu_gl_flushed` →
`virtio_gpu_handle_gl_flushed` → tries to re-enter `process_cmdq`.
The re-entrancy guard (`g->processing_cmdq`) prevents actual re-entry
but the interaction caused QEMU to crash.

**Current approach: IOSurfaceLock fence only.**

```
1. glBlitFramebuffer         — read virgl tex → IOSurface[ring_slot]
2. glFlush()                 — submit Metal commands
3. IOSurfaceLock(ReadOnly)   — wait for ALL GPU writes (cross-queue fence)
4. IOSurfaceUnlock()
5. CVPixelBufferCreateWithIOSurface — zero-copy wrap
6. VTCompressionSessionEncodeFrame  — async (ring buffer protects)
```

### Testing needed

- Does IOSurfaceLock alone fix the corruption?
- If not, need to find another way to add backpressure (can't use
  gl_block from inside process_cmdq)

## Key Files

- `hw/display/helix/helix-frame-export.m` — GL blit + IOSurfaceLock fence
- `hw/display/helix/helix-frame-export.h` — ring buffer struct fields
- `hw/display/virtio-gpu-base.c` — `helix_gl_block()` wrapper (unused now)
- `hw/display/virtio-gpu-virgl.c` — calls `helix_scanout_frame_ready`

## Key Insight: Why Backpressure Can't Work Here

SPICE's `gl_block` works because `qemu_spice_gl_update` is called from the
display update path (`dpy_gl_update`), NOT from inside `process_cmdq`.
Our `helix_scanout_frame_ready` is called from `virgl_cmd_set_scanout` which
IS inside `process_cmdq`. To use backpressure, we'd need to move the frame
capture to the display update path instead.
