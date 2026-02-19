# Multi-Desktop DRM Deadlock Analysis

**Date**: 2026-02-16
**Status**: In progress — GPU interrupt freeze not yet resolved

## Symptom

Starting 4 containers with mutter/gnome-shell rendering causes the VM to become unusable. htop/ps hang, video streaming fails (0 frames), kernel reports hung tasks blocked 120+ seconds.

## Fixes Applied (Committed)

### Fix 1: Remove `activateCrtc` (helix, `fbd22b346`)
`DRM_IOCTL_MODE_SETCRTC` on master FD acquires `mode_config.mutex`, deadlocking with running gnome-shells doing atomic page flips. Removed — mutter does its own modeset via the lease FD.

### Fix 2: Remove `renderer_blocked` from blob unmap (qemu, `0cfa4993f6`)
`renderer_blocked` is GLOBAL — stops ALL command processing when >0. Blob unmaps (Venus uses heavily) were incrementing it. With 4 contexts, unmaps overlap and counter stays >0 perpetually. Removed increment/decrement, added `detached` flag to guard against double memory region removal.

### Fix 3: `QTAILQ_FOREACH_SAFE` in `process_cmdq` (qemu, `0cfa4993f6`)
Original loop did `QTAILQ_FIRST` + `break` on suspended commands, blocking all subsequent commands. Changed to `FOREACH_SAFE` + `continue` so suspended blob unmaps don't block other contexts.

### Fix 4: Increase virtqueue to 1024 entries (qemu, `0cfa4993f6`)
Control virtqueue was 256 entries. With 4 GPU contexts, fills up causing all guests to block in `virtio_gpu_queue_ctrl_sgs`.

### Fix 5: Skip `reprobeConnector` (helix, `5a3b82b32`)
Writing to `/sys/class/drm/card0-Virtual-N/status` calls `drm_helper_probe_single_connector_modes` which acquires `mode_config.mutex`. Same deadlock as `activateCrtc`. QEMU's `enableScanout` already triggers guest hotplug.

### Fix 6: `fence_poll` — REALTIME clock + unconditional re-arm (qemu, `b1f65e89bd`)
`QEMU_CLOCK_VIRTUAL` stops when vCPUs halt (WFI). Changed to `QEMU_CLOCK_REALTIME`. Also unconditionally re-arm (100 Hz polling).

## Current Problem: fence_poll Timer Not Firing

**Despite all fixes, GPU interrupts still freeze** (interrupt count stays constant for 5+ seconds). QEMU `sample` shows:

- Main loop (qemu_main thread): 780/790 samples idle in `g_poll` → `__select`
- Timer system fires `gui_update` (REALTIME) ~4 times/second — proves REALTIME timers work
- **Zero `fence_poll` hits in 1-second sample** — the timer exists but never fires
- All vCPU threads: heavy BQL contention (25-40% of samples in `bql_lock_impl`)
- virglrenderer threads: idle (waiting for requests on socket)

### Deadlock Chain (Current)

1. **gnome-shell** holds `mode_config.mutex` during `drm_atomic_commit` → `wait_for_fences` → `dma_fence_default_wait`
2. GPU fence never completes because `fence_poll` isn't running → `virgl_renderer_poll()` never called → virglrenderer never reports fence completion
3. **Other gnome-shells** block on `drm_modeset_lock` (same mutex)
4. **gst-plugin-scan** stuck in `virtio_gpu_vram_mmap` (synchronous wait for MAP_BLOB response)
5. htop/ps block on `mmap_lock` of D-state processes

### Hypotheses for fence_poll Not Firing

**H1: Timer deleted or never created**
Unlikely — `gui_update` timer (also REALTIME) works fine. But `fence_poll` is created in `virtio_gpu_virgl_init` which runs lazily on first `handle_ctrl`. Init definitely ran (QEMU logs show version string and frame export).

**H2: `fence_poll` fires but crashes/hangs inside, preventing re-arm**
`virgl_renderer_poll()` or `process_cmdq()` could hang. The `Blocked re-entrant IO on MemoryRegion: virtio-pci-notify-virtio-gpu` warning suggests re-entrant MMIO. If `process_cmdq` calls `virtio_gpu_ctrl_response` which does `virtio_notify` which triggers a re-entrant MMIO write, QEMU may abort or skip the notification.

**H3: Timer fires initially but gets deleted during reset**
`virtio_gpu_gl_reset` calls `timer_free(gl->fence_poll)`. If a guest reboot or reset happens, the timer is freed and never recreated (init only runs on first `handle_ctrl`).

**H4: Main loop not polling the REALTIME timer fd**
The `g_poll` timeout might not be set correctly for the REALTIME timer, so the main loop sleeps past the timer deadline.

### Next Steps

1. Add debug logging to `fence_poll` to verify it fires at all after init
2. Check if `virtio_gpu_gl_reset` is being called (would delete the timer)
3. Investigate the re-entrant IO warning — could be caused by `process_cmdq` → `ctrl_response` → `virtio_notify` re-entering

## Files

| File | Role |
|------|------|
| `api/pkg/drm/manager.go` | DRM lease management (removed activateCrtc + reprobeConnector) |
| `api/pkg/drm/ioctl_linux.go` | DRM ioctl implementations |
| `hw/display/virtio-gpu-virgl.c` | fence_poll, blob unmap, process_cmd |
| `hw/display/virtio-gpu.c` | process_cmdq (FOREACH_SAFE) |
| `hw/display/virtio-gpu-base.c` | virtqueue size, renderer_blocked, gl_block |
| `hw/display/virtio-gpu-gl.c` | handle_ctrl, reset, init state machine |
| `hw/display/helix/helix-frame-export.m` | Frame capture pipeline |
