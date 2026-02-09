# GL Blit Sync for H.264 Encoding

**Date:** 2026-02-09
**Status:** glFinish fence + EGL context save/restore (testing)

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

### IOSurfaceLock alone — Still corrupt

User reported staircase artifacts, shifted duplicate content, pink bands.
Two root causes identified:

1. **EGL context not restored after blit.** `helix_gl_blit_frame` called
   `eglMakeCurrent(helix_ctx)` but never restored virglrenderer's context.
   Since we're called from inside `virgl_cmd_set_scanout`, subsequent
   virglrenderer GL calls went to the wrong context → corruption.
   (The old deleted Metal snapshot code DID save/restore the context.)

2. **IOSurfaceLock may not fence GPU writes.** It's documented for CPU
   access synchronization. May not actually wait for Metal command queue
   writes from a different context.

### Fix: EGL context save/restore + glFinish

Replaced IOSurfaceLock with `glFinish()` and added EGL context save/restore:

```
1. Save EGL context (virglrenderer's)
2. eglMakeCurrent(helix_ctx)
3. glBlitFramebuffer           — read virgl tex → IOSurface[ring_slot]
4. glFinish()                  — wait for ALL GL commands on our context
5. Restore EGL context (virglrenderer's)
6. CVPixelBufferCreateWithIOSurface — zero-copy wrap
7. VTCompressionSessionEncodeFrame  — async (ring buffer protects)
```

`glFinish()` blocks until all commands submitted on our ANGLE context are
complete. Since ANGLE translates GL → Metal, this waits for the Metal
command buffer to finish. This is stronger than `glFlush()` (non-blocking)
and more appropriate than `IOSurfaceLock` (CPU-oriented).

### Testing needed

- Does EGL context save/restore + glFinish fix the corruption?
- If not, remaining suspects: pixel format mismatch, Y-flip, or
  encoder configuration issue

## Key Files

- `hw/display/helix/helix-frame-export.m` — GL blit + glFinish fence + EGL save/restore
- `hw/display/helix/helix-frame-export.h` — ring buffer struct fields
- `hw/display/virtio-gpu-base.c` — `helix_gl_block()` wrapper (unused now)
- `hw/display/virtio-gpu-virgl.c` — calls `helix_scanout_frame_ready`

## Key Insight: Why Backpressure Can't Work Here

SPICE's `gl_block` works because `qemu_spice_gl_update` is called from the
display update path (`dpy_gl_update`), NOT from inside `process_cmdq`.
Our `helix_scanout_frame_ready` is called from `virgl_cmd_set_scanout` which
IS inside `process_cmdq`. To use backpressure, we'd need to move the frame
capture to the display update path instead.

## Key Insight: Display Update Path Not Feasible for Secondary Scanouts

Moving frame capture to the display update path would fix the backpressure
problem, but secondary scanouts use `virtio_gpu_secondary_ops` with
`GRAPHIC_FLAGS_NONE` — no GL, no DMABUF. This was deliberately done
(`virtio-gpu-base.c:240-253`) to prevent UTM's broken `gl_draw_done`
handling from permanently blocking `renderer_blocked`. Without GL flags,
secondary consoles get 2D SPICE display listeners, and `dpy_gl_update` is
never called for them. The display update path simply doesn't exist for the
scanouts Helix uses.

To make the display update path work, you'd need to:
1. Re-enable GL flags for secondary scanouts
2. Fix UTM's `gl_draw_done` handling (or bypass SPICE entirely for Helix scanouts)
This is a larger architectural change.
