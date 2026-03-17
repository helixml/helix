# Implementation Tasks

## QEMU Patches (core fix)

- [ ] Create QEMU patch: add dedicated fence-poll thread (sleeps 10ms, schedules BH) to replace broken timer — this fixes the MMIO re-entrancy drop that silently kills virtio notifications during boot
- [ ] Create QEMU patch: drain virtqueue ring (`virtqueue_pop()` loop) inside the BH callback to recover dropped kick commands
- [ ] Create QEMU patch: remove `renderer_blocked` counter check from blob unmap path in `process_cmdq` — prevents global block on all GPU contexts during concurrent unmaps
- [ ] Create QEMU patch: change `process_cmdq` loop from `QTAILQ_FIRST + break` to `QTAILQ_FOREACH_SAFE + continue` — eliminates head-of-line blocking across GPU contexts
- [ ] Create QEMU patch: increase virtqueue size from 256 to 1024 entries — prevents saturation during boot initialization

## Guest Kernel / DRM Manager

- [ ] Remove `activateCrtc` call from `helix-drm-manager` (QEMU's `enableScanout` already triggers hotplug events, making this redundant and deadlock-prone)
- [ ] Remove `reprobeConnector` sysfs write from `helix-drm-manager` (same `mode_config.mutex` deadlock path)

## Frame Export Fix

- [ ] Add idempotency guard to `helix_frame_export_init()` in `for-mac/qemu-helix/helix-frame-export.c`: check if already initialized, clean up previous `HelixFrameExport` state before reinitializing (called 3× during normal boot due to virtio device reset cycles)

## Verification

- [ ] Reproduce boot hang consistently in test (run 10+ cold boots, observe failure rate before fix)
- [ ] Apply patches and verify 10+ cold boots complete without hang
- [ ] Confirm no regression: boot time, GPU frame rate, and multi-desktop behavior unchanged
