# Desktop capture: GL-import + persistent-buffer architecture (2026-06-11)

Branch: `feat/desktop-gl-import-persistent-buffer`

## Problem (proven by instrumentation, this session)

NVIDIA zero-copy capture stutters: every few seconds the client sees a 60–140ms
freeze + a catch-up burst. Root cause, localised with the `[METRIC]` probes:

- We import Mutter's PipeWire dma-buf to CUDA **per frame** in the PipeWire
  callback: `EGLImage::from` (eglCreateImageKHR) + `CUDAImage::from`
  (cuGraphicsEGLRegisterImage), synchronously, while holding Mutter's buffer.
- `cuda.egl` and `cuda.reg` spike to ~70ms under GPU contention; `cuda.copy`
  **never** spikes. So the cost is specifically EGL-import + CUDA-register
  (driver resource-management lock, contends with our own nvh264enc), not the
  copy and not a GPU-wide stall.
- The stall is inline on the single capture thread → blocks Mutter delivery →
  `A.arrival`/`hold_buf`/`cuda_total` maxes are all equal (≈141ms) → gap → burst.

## What we ruled out (with evidence)

- Network / WAN: ethernet, mtr clean; producer-side gap == client gap.
- CPU saturation / Go scheduler: 48 cores, canary 0/0/0.
- Thermal: 53°C, full clocks.
- The bounded(8) channel: `chan depth_max=0, drops=0` always.
- Mutter "renders on demand": `A.arrival avg=16ms` = steady 60fps.
- **Registration caching (TWO attempts, both reordered):** caching the CUDA
  registration of Mutter's rotating pool dma-bufs serves stale/wrong frames —
  even with the slot fd kept alive. A cached CUDA-EGL registration of an
  *external* buffer does not track the producer's later writes. Dead end.

## What wolf and OBS actually do (the references)

- **wolf / gst-wayland-display** is a *compositor*, not a capturer. It renders
  the desktop into ONE persistent `output_buffer` it owns, registers that buffer
  to CUDA **once** (`GsBufferType::CUDA.cuda_image: Arc<Mutex<CUDAImage>>`), and
  reuses the registration every frame (`to_gs_buffer` = map+copy). Caching works
  for it because it owns the buffer and renders→maps sequentially on one thread.
- **OBS linux-pipewire** *does* capture from PipeWire (like us) AND uses NVENC.
  Its path: dma-buf → **cheap per-frame GL import** (`gs_texture_create_from_dmabuf`,
  eglCreateImage + GL texture; NO `cuGraphicsEGLRegisterImage`) → composite into
  OBS's **own persistent framebuffer** → NVENC reads THAT (registered once). OBS
  also uses explicit sync (`SPA_META_SyncTimeline` acquire/release). It NEVER
  CUDA-registers the external capture buffer.

**Convergence:** both register a buffer *they own* once, and get the capture
content into it cheaply. We are the only one shoving the external rotating
capture buffer straight into NVENC via per-frame CUDA registration.

## Why not stock GStreamer (`pipewiresrc ! glupload ! cudaupload ! nvh264enc`)

Confirmed caps exist (GStreamer 1.26.6: `glupload` takes DMABuf→GLMemory,
`cudaupload` takes GLMemory→CUDAMemory). BUT: stock GStreamer does not reliably
negotiate NVIDIA's tiled dma-buf modifiers (the `0x300000000e08xxx` family) —
this is the original reason the custom wolf-based plugin exists. So we keep the
custom plugin for negotiation and do the GL import ourselves.

## Proposed architecture (reuse vendored wolf machinery)

Make `pipewirezerocopysrc` a mini-blitter that feeds wolf's persistent-buffer
path, instead of per-frame manual CUDA register of the external buffer:

1. Stand up a `GlesRenderer` on the NVIDIA render node (`setup_renderer` exists).
2. Create ONE persistent `GsBufferType::CUDA` output buffer, registered to CUDA
   once (already implemented in `allocator/mod.rs` buffer init).
3. Per frame, on the capture thread:
   a. `renderer.import_dmabuf(mutter_dmabuf)` → `GlesTexture` — cheap GL import
      (OBS-style; no CUDA register of the external buffer).
   b. `bind(output_buffer)` + draw the texture as a `TextureRenderElement` into
      it (mirrors `create_frame`).
   c. `output_buffer.to_gs_buffer()` → reuses the **cached** CUDA registration of
      our own buffer (map+copy) → CUDAMemory GstBuffer → nvh264enc.
   d. requeue Mutter's buffer (we copied out in step b; race-safe).

This gives: cheap per-frame external import (GL, no register spike) + cached
NVENC registration on a buffer we own (wolf's smooth path). All pieces are
already vendored in `wayland-display-core`.

### Open items / risks
- GL render adds a GPU pass per frame (texture blit). Cheap, but measure with
  the existing `[METRIC]` probes (cuda.* should drop; add a `gl_blit` stage).
- Synchronization: smithay's GL dmabuf import + render should respect implicit
  fences; if we see tearing, adopt OBS's explicit `SPA_META_SyncTimeline`.
- Threading: keep it on the capture thread first (simplest, matches wolf). The
  GL blit replaces the expensive register, so the thread should no longer stall.
- Format/colour: ensure the blit preserves BGRx/BGRA without an extra convert.

## Validation
Existing instrumentation makes this a clean A/B: deploy to a THROWAWAY session
(not the user's live one this time), watch `[METRIC]` (cuda.egl/reg → gone,
new gl_blit stage small, A.arrival/B.create p99 flat, burst=0) AND confirm
visually smooth + correctly ordered.

## Status
- [x] Branch created, instrumentation kept.
- [x] Confirmed vendored `GsBufferType::CUDA` + `GlesRenderer` + import path.
- [ ] Revert the failed registration-caching from the capture plugin (keep instrumentation).
- [ ] Implement GL-import → persistent-buffer blit → cached NVENC registration.
- [ ] Build, test on throwaway session, A/B with metrics.
