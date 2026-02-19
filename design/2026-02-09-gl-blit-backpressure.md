# GL Blit Sync for H.264 Encoding

**Date:** 2026-02-09
**Status:** Corruption fixed — testing whether all 3 sync layers are needed or if all-keyframe was masking it

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

### Commit 67d79c6580 — Replaced glFinish with glFlush

`glFinish()` blocks the QEMU main thread waiting for the Metal command
buffer. This stalls `process_cmdq` and deadlocks the guest GPU driver
after a few seconds. Replaced with `glFlush()` (non-blocking), matching
what SPICE does. The triple-buffered ring provides latency margin.

**Still saw corruption** with EGL context save/restore + glFlush alone
(without backpressure). The guest can still modify the virgl texture
between glFlush and VT reading the IOSurface.

### Commit 098b5532cc — Deferred backpressure via BH

Implemented the deferred BH approach. All QEMU code, no UTM changes.

- `virtio-gpu-base.c`: Added `helix_gl_unblock_bh` callback,
  `helix_create_gl_unblock_bh()`, `helix_schedule_gl_unblock()`
- `helix-frame-export.h`: Added `void *gl_unblock_bh` to `HelixFrameExport`
- `helix-frame-export.m`:
  - `helix_frame_export_init`: creates BH via `helix_create_gl_unblock_bh`
  - `helix_scanout_frame_ready`: calls `helix_gl_block(true)` before blit
  - `scanout_encoder_callback`: calls `helix_schedule_gl_unblock` (thread-safe)
  - Error paths (blit fail, CVPixelBuffer fail, encode fail): also schedule BH

### Commit a9825e4b79 — glFinish on virgl context + all-keyframe diagnostic

Still saw intermittent corruption with backpressure alone. Two changes:

1. **`glFinish()` on virglrenderer's context** before switching to helix
   context. Backpressure prevents the guest from submitting MORE commands,
   but doesn't wait for ALREADY-SUBMITTED GPU rendering to complete. The
   GPU may still be executing rendering commands when we read the texture.
   `glFinish()` on virglrenderer's context (currently active when we're
   called) waits for the guest's rendering to complete.

2. **All-keyframe encoding** (diagnostic). Every frame is a self-contained
   keyframe — if one frame has a corrupt blit, it doesn't propagate to
   subsequent frames via P-frame prediction.

**Result: Corruption gone.** Now testing which fix actually solved it
by reverting to P-frame encoding (keyframe on first frame only).

### Key finding: Three-layer sync was needed

The corruption required ALL THREE mechanisms to fix:
1. **EGL context save/restore** — prevents corrupting virglrenderer's GL state
2. **Backpressure via BH** — prevents guest from modifying texture while we read
3. **`glFinish()` on virgl context** — waits for GPU to finish rendering texture

Without #3, the GPU could still be executing rendering commands for the
texture even though the guest has submitted SET_SCANOUT. `glFlush()` only
submits commands, doesn't wait. `glFinish()` blocks until the Metal
command buffer completes. With backpressure limiting to one frame in-flight,
this doesn't accumulate or cause hangs.

### Testing: P-frame encoding

Reverted to keyframe-on-first-frame-only to confirm the blit sync fixes
are sufficient without all-keyframe encoding.

## Key Files

- `hw/display/helix/helix-frame-export.m` — GL blit + backpressure + EGL save/restore
- `hw/display/helix/helix-frame-export.h` — ring buffer + BH fields
- `hw/display/virtio-gpu-base.c` — `helix_gl_block()` + BH wrappers
- `hw/display/virtio-gpu-virgl.c` — calls `helix_scanout_frame_ready`

## Correction: Backpressure CAN Work From Inside process_cmdq

Earlier analysis said backpressure can't work because we're called from
inside `process_cmdq`. **This was wrong.** SPICE also calls `gl_block(true)`
from inside `process_cmdq`:

```
process_cmdq → RESOURCE_FLUSH → dpy_gl_update → qemu_spice_gl_update
  → gl_block(true)    ← increments renderer_blocked
  → glFlush()         ← submit (non-blocking)
  → gl_draw_async()   ← hand off to SPICE client
  → return            ← back to process_cmdq, which STOPS (blocked)
  ...
  [later, on SPICE client thread]
  → gl_draw_done()    ← schedules a BH
  ...
  [main loop, next iteration]
  → BH fires          ← gl_block(false), renderer_blocked=0
  → gl_flushed()      ← process_cmdq resumes
```

The key: SPICE calls `gl_block(true)` and **returns**. The
`gl_block(false)` happens later via a bottom-half (BH) scheduled from
the SPICE client thread. The BH fires on the next main loop iteration,
safely outside `process_cmdq`.

Our crash (commit 535af85418) happened because we called BOTH
`gl_block(true)` AND `gl_block(false)` **synchronously in the same
function call**. The unblock immediately triggered `gl_flushed` →
re-entered `process_cmdq` → crash. The concept was fine; the execution
was wrong.

## Implementation: Deferred Backpressure via BH

Matches SPICE's exact pattern. ~30 lines of QEMU code, no UTM changes.

```
helix_scanout_frame_ready (inside process_cmdq):
  1. Save EGL context
  2. gl_block(true)              — stop further command processing
  3. eglMakeCurrent(helix_ctx)
  4. glBlitFramebuffer           — read virgl tex → IOSurface[ring_slot]
  5. glFlush()                   — submit (non-blocking)
  6. Restore EGL context
  7. CVPixelBufferCreateWithIOSurface
  8. VTCompressionSessionEncodeFrame(async)
  9. return                      — process_cmdq stops (renderer_blocked > 0)

VT encode completion callback (on VT thread):
  10. qemu_bh_schedule(encode_done_bh)  — thread-safe, bounces to main thread

BH handler (on main thread, next main loop iteration):
  11. gl_block(false)            — renderer_blocked=0 → process_cmdq resumes
```

Implementation:
- Add `QEMUBH *encode_done_bh` to `HelixFrameExport`
- Create BH in init: `qemu_bh_new(helix_encode_done_bh, fe)`
- VT callback: `qemu_bh_schedule(fe->encode_done_bh)`
- BH handler: `helix_gl_block(fe->virtio_gpu, false)`
- `helix_scanout_frame_ready`: add `helix_gl_block(true)` before blit

This gives us proper backpressure (at most one frame in-flight), matches
SPICE's proven pattern exactly, and the VT encode completion replaces
SPICE's `gl_draw_done` signal. The triple-buffered ring becomes a safety
margin rather than the primary sync mechanism.
