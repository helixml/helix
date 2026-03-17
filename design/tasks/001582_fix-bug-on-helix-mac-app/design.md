# Design: Fix VM Boot Hang on Helix Mac App

## Root Cause Analysis

### Why realize is called 3 times (normal, not the bug)

The three `[HELIX] realize: START` entries in `qemu-helix-realize.log` are expected. QEMU's virtio device lifecycle calls `realize` during:
1. Initial device creation (QEMU startup)
2. UEFI/firmware driver reset and re-initialization
3. Linux kernel virtio-gpu driver reset and re-initialization (writes 0 to status register, triggering unrealize → realize)

All three complete with `*errp=0x0` (no error). The hang happens after #3 completes.

### The actual bug: MMIO re-entrancy guard silently drops virtio notifications

During Linux kernel GPU driver initialization, the following race occurs:

1. Guest writes a virtio-gpu command to the virtqueue (MMIO write → sets `engaged_in_io = true` in QEMU)
2. QEMU processes the command inside the MMIO handler context
3. QEMU calls `virtio_notify()` to interrupt the guest with the response
4. `virtio_notify()` internally calls `stl_le_phys()` which writes to the same device's memory region
5. QEMU's re-entrancy guard sees `engaged_in_io = true` and **silently drops the write** (returns `MEMTX_ACCESS_ERROR`, unchecked)
6. Guest never receives the interrupt → GPU driver thread blocks indefinitely waiting for response
7. VM appears stuck on "Booting VM..." — SSH never comes up because the kernel GPU init deadlocks

**Why it's intermittent**: The race window depends on vCPU scheduling and BQL (Big QEMU Lock) contention. With 14 vCPUs competing, timing varies per boot.

This is documented in `/home/retro/work/helix-4/design/2026-02-16-fixing-qemu-gpu-deadlock.md` and `/home/retro/work/helix-4/design/2026-02-16-gpu-virtualization-architecture.md`.

### Secondary issue: Timer-based fence polling doesn't fire

The existing `fence_poll` timer uses `QEMU_CLOCK_VIRTUAL` which stops when all vCPUs halt (WFI — common during boot). Even after switching to `QEMU_CLOCK_REALTIME`, the timer still doesn't fire reliably due to a race in GLib timer registration for timers created after the main loop starts.

## Fix: Thread-Based BH Fence Polling

Replace the broken timer approach with a dedicated thread that:

1. Sleeps 10ms in a loop
2. Calls `qemu_bh_schedule()` (thread-safe — writes to eventfd) to enqueue a bottom-half callback
3. The BH executes **outside** the MMIO handler context (`engaged_in_io = false`)
4. Inside the BH: drain the virtqueue ring via `virtqueue_pop()` to recover any dropped kick commands
5. Call `process_cmdq()` from BH context — `virtio_notify()` now goes through normal memory dispatch without hitting re-entrancy guard

This is the fix described in the design document, item #7 of the fix list.

## Additional Fixes Required

The design document identifies 7 fixes needed. All should be applied together as QEMU patches:

| # | Fix | Why |
|---|-----|-----|
| 1 | Remove `activateCrtc` from guest DRM manager | Avoids `mode_config.mutex` deadlock with running gnome-shells |
| 2 | Remove `reprobeConnector` from guest DRM manager | Same mutex, different path |
| 3 | Remove `renderer_blocked` from blob unmap path | Global counter blocks ALL GPU contexts; overlapping unmaps keep it >0 |
| 4 | `QTAILQ_FOREACH_SAFE` in `process_cmdq` | Suspended command at head blocks all other contexts (head-of-line blocking) |
| 5 | Increase virtqueue to 1024 entries | 256-entry ring saturates with multiple GPU contexts |
| 6 | Use `QEMU_CLOCK_REALTIME` for fence_poll | VIRTUAL clock stops when vCPUs halt |
| 7 | **Thread + BH fence polling with ring drain** | Core fix for the boot hang (timer unreliable, MMIO re-entrancy drops notifications) |

Fixes #3, #4, #5, #7 directly address the boot hang. #1 and #2 affect multi-desktop scenarios.

## helix_frame_export_init Idempotency

`helix_frame_export_init()` in `for-mac/qemu-helix/helix-frame-export.c` is called from `virtio_gpu_virgl_init()` each time realize runs. Currently contains `TODO` stubs and allocates a new `HelixFrameExport` struct each call without freeing the old one. This leaks memory across the 3 realize calls and may leave the encoder in an undefined state.

Fix: add a static guard or check if already initialized, and clean up previous state before reinitializing.

## Key Files

- `for-mac/qemu-helix/helix-frame-export.c` — frame export init (needs idempotency)
- `qemu-patches/` — QEMU patches (add new patches for the 7 fixes)
- `design/2026-02-16-fixing-qemu-gpu-deadlock.md` — detailed root cause analysis (reference)
- `design/2026-02-16-gpu-virtualization-architecture.md` — GPU architecture context
