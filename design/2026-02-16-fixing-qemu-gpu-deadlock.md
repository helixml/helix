# Fixing a GPU Deadlock in QEMU: When Interrupts Get Silently Eaten

**Date**: 2026-02-16
**Author**: Luke Marsden + Claude
**Status**: In progress

## The Problem

We're running 4+ GPU-accelerated Linux desktops (GNOME Shell, Zed IDE, Chromium) inside a single QEMU virtual machine on Apple Silicon. Each desktop gets its own virtual GPU output via virtio-gpu with Vulkan passthrough through virglrenderer/Venus. One or two desktops work fine. Four desktops deadlock the entire VM — htop hangs, video streaming stops, kernel reports tasks blocked for 120+ seconds.

## The Deadlock Chain

The chain has three links, each discovered by peeling back a layer of the previous one.

### Link 1: The Kernel Mutex

When we start 4 desktops, the kernel's serial console reports:

```
helix-drm-manag:30254 blocked on mutex likely owned by gnome-shell:36330
```

The DRM manager (which hands out virtual displays to containers) and gnome-shell both acquire `mode_config.mutex` — a kernel mutex that serializes DRM modesetting operations. gnome-shell holds it during atomic page flips while waiting for GPU fences. The DRM manager grabs it when probing connector status or setting CRTCs. With 4 desktops, one gnome-shell holds the mutex indefinitely (because its GPU fence never completes), blocking the DRM manager, which blocks all other gnome-shells' page flips.

**Fix**: Removed all DRM manager operations that acquire `mode_config.mutex` — the connector probing (`reprobeConnector`) and CRTC initialization (`activateCrtc`) were both unnecessary because QEMU's `enableScanout` already triggers guest hotplug events.

### Link 2: The Global Command Block

With the mutex deadlock removed, we hit the next layer: `renderer_blocked`, a global counter in QEMU's virtio-gpu device model. When `renderer_blocked > 0`, `process_cmdq()` refuses to process **any** command from **any** GPU context.

`renderer_blocked` was designed for SPICE's single-display GL path — pause GPU work while the display client catches up. But Venus (the Vulkan-on-virtio protocol) increments it during blob resource unmaps, which involve an async RCU (read-copy-update) cleanup phase. With 4 GPU contexts doing overlapping unmaps, the counter stays >0 perpetually.

The command queue also used FIFO head-of-line blocking: if the first command in the queue was suspended (waiting for RCU), every command behind it — including commands from completely different GPU contexts — was blocked.

**Fix**: Removed `renderer_blocked` from the blob unmap path entirely (the `cmd_suspended` mechanism already prevents the specific unmap from re-executing). Changed the command queue loop from `QTAILQ_FIRST + break` to `QTAILQ_FOREACH_SAFE + continue` so suspended commands don't block other contexts.

### Link 3: The Silent Interrupt Drop

With the mutex and command-blocking fixes applied, all 4 desktops initialize and GPU interrupts start flowing. Then after 10-30 seconds, everything freezes again. GPU interrupt count stops advancing. But this time, QEMU's fence polling is running, the command queue is empty, and `renderer_blocked` is 0. There's nothing to process because no commands are reaching QEMU.

The clue is a single QEMU warning:

```
warning: Blocked re-entrant IO on MemoryRegion: virtio-pci-notify-virtio-gpu at addr: 0x0
```

This is QEMU's memory region re-entrancy guard. Here's what it means.

## How Virtio-GPU Commands Flow

```
Guest vCPU thread              QEMU main thread
==================              ================
Write cmd to virtio ring
Write to PCI notify BAR  ─────>  MMIO handler fires
  (virtqueue "kick")              engaged_in_io = true
                                  handle_ctrl():
                                    virtqueue_pop() all pending cmds
                                    process_cmdq()
                                      for each cmd:
                                        dispatch to virglrenderer
                                        virtio_gpu_ctrl_response()
                                          virtqueue_push()  ← put response in ring
                                          virtio_notify()   ← send MSI-X interrupt
                                            stl_le_phys()   ← write to GIC ITS
                                  engaged_in_io = false
```

The guest kernel's virtio-gpu driver communicates with QEMU through a virtio virtqueue — a shared-memory ring buffer. The guest writes command descriptors and "kicks" the queue by writing to a PCI BAR (MMIO register). QEMU receives the kick, pops commands from the ring, processes them, and sends responses back via `virtio_notify()`, which injects an MSI-X interrupt.

**Every command gets exactly one response.** The guest thread that submitted a command blocks in the kernel until the response arrives as an interrupt. If QEMU never sends the interrupt, the guest thread blocks forever.

## The Re-Entrancy Guard

QEMU's memory subsystem has a safety mechanism: when an MMIO handler is executing (`engaged_in_io = true`), any attempt to write to the same device's memory region is silently blocked. This prevents infinite loops where a handler's side effects trigger another dispatch to the same handler.

The problem: `virtio_notify()` — called from inside the MMIO handler to send responses — does a `stl_le_phys()` write that goes through the memory dispatch system. If this write path touches the same device's memory region, the re-entrancy guard fires. The notification is silently dropped — `MEMTX_ACCESS_ERROR` is returned, but nobody checks it. The guest never receives the interrupt.

With 1-2 desktops, this rarely triggers. With 4+ desktops and 14 vCPUs contending on QEMU's Big QEMU Lock (BQL), the timing window widens and it happens on almost every run.

## Why the Timer Didn't Save Us

QEMU has a `fence_poll` timer designed to break exactly this kind of circular dependency. It's supposed to fire every 10ms and call `virgl_renderer_poll()` (check for completed GPU work) and `process_cmdq()` (process pending commands). This way, even if a kick notification is lost, the timer picks up the slack.

The timer doesn't fire.

We confirmed this with 1-second process sampling (782 samples at 1ms intervals): `gui_update` (another REALTIME timer) shows 3 hits. `fence_poll` shows **zero**. Both are created with `timer_new_ms(QEMU_CLOCK_REALTIME, ...)` so they should be on the same timerlist. We don't know why one fires and the other doesn't — our best hypothesis is a race in QEMU's GLib timer integration when timers are created after the main loop is already running (gui_update is created during display init, fence_poll during the first virtqueue kick).

We also fixed an earlier issue where the timer used `QEMU_CLOCK_VIRTUAL`, which stops advancing when all vCPUs are halted (executing WFI). When every guest thread is blocked on GPU fences, all vCPUs enter WFI, the virtual clock stops, and the timer never fires — a different flavor of the same deadlock.

## The Fix: Bottom Halves and Ring Draining

Instead of fighting the timer system, we bypass it entirely.

### What's a Bottom Half?

The term comes from the Linux kernel's interrupt handling model:

- **Top half**: the interrupt handler itself — runs immediately in interrupt context, does the minimum necessary work (acknowledge hardware, copy urgent data), can't sleep or do anything complex.
- **Bottom half**: deferred work that runs later in process context — handles the bulk of the processing where sleeping, locking, and complex operations are safe.

QEMU borrows this concept. A QEMU BH (bottom half) is a callback registered with `aio_bh_new()` that runs on the main loop thread during `aio_ctx_dispatch`. You schedule it from any context (including other threads) with `qemu_bh_schedule()`, which writes to an eventfd to wake the main loop. The main loop then dispatches all pending BHs on its next iteration.

The critical property: **BHs run outside of MMIO handler context.** When a BH callback executes, `engaged_in_io` is false. Any `virtio_notify()` calls from inside a BH will go through the memory dispatch system normally, without hitting the re-entrancy guard.

### The Implementation

```c
/* Dedicated thread — sleeps 10ms, schedules BH, repeat */
static void *fence_poll_thread_fn(void *opaque)
{
    VirtIOGPU *g = opaque;
    VirtIOGPUGL *gl = VIRTIO_GPU_GL(g);

    while (gl->fence_poll_thread_running) {
        g_usleep(10000); /* 10ms = 100 Hz */
        qemu_bh_schedule(gl->fence_poll_bh);
    }
    return NULL;
}

/* BH callback — runs on main loop thread, BQL held, engaged_in_io=false */
static void fence_poll_bh_cb(void *opaque)
{
    VirtIOGPU *g = opaque;
    VirtIOGPUGL *gl = VIRTIO_GPU_GL(g);
    struct virtio_gpu_ctrl_command *cmd;

    /* Drain the virtqueue ring — recover commands from dropped kicks */
    if (gl->renderer_state == RS_INITED) {
        VirtQueue *vq = virtio_get_queue(VIRTIO_DEVICE(g), 0);
        if (virtio_queue_ready(vq)) {
            cmd = virtqueue_pop(vq, sizeof(struct virtio_gpu_ctrl_command));
            while (cmd) {
                cmd->vq = vq;
                cmd->error = 0;
                cmd->finished = false;
                QTAILQ_INSERT_TAIL(&g->cmdq, cmd, next);
                cmd = virtqueue_pop(vq, sizeof(struct virtio_gpu_ctrl_command));
            }
        }
    }

    virgl_renderer_poll();
    virtio_gpu_process_cmdq(g);
}
```

This fixes both failure modes:

1. **Dropped kick notifications**: The BH drains the virtqueue ring directly via `virtqueue_pop`, just like `handle_ctrl` does. Even when the re-entrancy guard drops a kick, the commands are picked up within 10ms.

2. **Dropped response notifications**: When `process_cmdq` runs from the BH, `engaged_in_io` is false. `virtio_notify()` goes through the memory dispatch normally — no re-entrancy guard, no silent drops. The guest receives its interrupt.

3. **Timer system unreliability**: The thread uses `g_usleep` (always works) and `qemu_bh_schedule` (thread-safe, writes to eventfd). No dependency on QEMU's timer subsystem at all.

The thread does nothing except sleep and schedule — all actual GPU work happens on the main loop thread with BQL held, which is the only correct context for `virgl_renderer_poll()` and `process_cmdq()`. `qemu_bh_schedule()` is documented as thread-safe and is the standard QEMU mechanism for cross-thread work scheduling.

## All Fixes

| # | Fix | Layer | Root Cause |
|---|-----|-------|-----------|
| 1 | Remove `activateCrtc` | Guest kernel | `mode_config.mutex` deadlock with running gnome-shells |
| 2 | Remove `reprobeConnector` | Guest kernel | Same mutex, different acquisition path (sysfs write) |
| 3 | Remove `renderer_blocked` from blob unmap | QEMU device model | Global counter blocks all contexts; overlapping unmaps keep it >0 |
| 4 | `QTAILQ_FOREACH_SAFE` in `process_cmdq` | QEMU device model | Suspended command causes head-of-line blocking across contexts |
| 5 | Increase virtqueue to 1024 | QEMU device model | 256-entry ring saturates with 4 GPU contexts |
| 6 | REALTIME clock for fence_poll | QEMU timer | VIRTUAL clock stops when vCPUs halt (WFI) |
| 7 | Thread + BH fence polling with ring drain | QEMU device model | Timer doesn't fire; re-entrant MMIO drops kicks and notifications |

Every bug is a variation of the same theme: mechanisms designed for a single GPU context becoming global bottlenecks or failure points when shared across many.
