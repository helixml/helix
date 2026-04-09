# Multi-Desktop CFRelease Deadlock Investigation

**Date**: 2026-02-28
**Status**: Fix applied, pending build and test
**Incident**: VM hung after demoing several parallel desktop sessions (v2.7.10 Mac app)

## Symptom

User started multiple desktop sessions in parallel. The VM became unresponsive:
- Serial console: unresponsive
- SSH (port 41222): connection timeout
- HTTP (port 41080): connection timeout
- QEMU process: still running at 199% CPU
- QEMU logs: still printing keepalive frame encoding and frame drop messages

### Console errors (guest kernel)
```
[drm:virtio_gpu_dequeue_ctrl_func [virtio_gpu]] *ERROR* response 0x1200 (command 0x10c)
[drm:virtio_gpu_dequeue_ctrl_func [virtio_gpu]] *ERROR* response 0x1203 (command 0x102)
```

Decoded:
- `0x1200` = `VIRTIO_GPU_RESP_ERR_UNSPEC` on `0x10c` = `VIRTIO_GPU_CMD_RESOURCE_CREATE_BLOB`
- `0x1203` = `VIRTIO_GPU_RESP_ERR_INVALID_RESOURCE_ID` on `0x102` = `VIRTIO_GPU_CMD_RESOURCE_UNREF`

### QEMU frame export logs
```
[HELIX] Client 1: dropping frame (need 4625, have 106/1048576 bytes free, total drops=364100)
[KEEPALIVE] Re-encoded scanout 1 from IOSurface (slot=2, total keepalive frames=364800)
[HELIX] Client 0: dropping frame (need 2039, have 791/1048576 bytes free, total drops=364500)
```

Two clients connected, both with nearly full TCP send buffers (106 and 791 bytes free out of 1MB). 364k+ frames dropped.

## Investigation

### Process sample (1 second at 1ms intervals, PID 2415)

Taken with `sample 2415 1` at 23:01 UTC. 712 samples captured.

### Root cause: CFRelease blocking in resource_unref with BQL held

**Thread_8161975 (QEMU main loop / glib dispatch)** — 712/712 samples (100%):
```
qemu_main_loop → main_loop_wait → g_main_context_dispatch →
  aio_ctx_dispatch → aio_dispatch → aio_bh_poll → aio_bh_call →
    virtio_gpu_gl_handle_ctrl (virtio-gpu-gl.c:100) →
      virtio_gpu_process_cmdq (virtio-gpu.c:1062) →
        virtio_gpu_virgl_process_cmd (virtio-gpu-virgl.c:1275) →
          virgl_renderer_resource_unref →
            util_hash_table_remove →
              virgl_resource_destroy_func →
                CFRelease →
                  CF_IS_OBJC  ← STUCK HERE 100% of samples
```

This thread holds the Big QEMU Lock (BQL). It's processing a `VIRTIO_GPU_CMD_RESOURCE_UNREF` command, which triggers virglrenderer to destroy the resource. The resource destruction calls `CFRelease` on a CoreFoundation/Metal object, and this call **never returns**.

### All 14 vCPU threads blocked on BQL

Every vCPU thread is stuck at:
```
hvf_cpu_thread_fn → hvf_vcpu_exec → bql_lock_impl → qemu_mutex_lock_impl → _pthread_mutex_firstfit_lock_wait
```

All 14 vCPU threads (100% of samples for each) are waiting to acquire the BQL. Since the main loop thread holds BQL and is stuck in `CFRelease`, no guest code can execute.

### Other thread states

| Thread | State | Notes |
|--------|-------|-------|
| call_rcu_thread | blocked on BQL (100%) | Can't do RCU callbacks |
| fence_poll_thread | sleeping in g_usleep (100%) | Schedules BHs, but they can't run (main loop stuck) |
| keepalive_thread | sleeping in usleep (100%) | Not currently encoding |
| SPICE Worker | idle in g_poll (100%) | Waiting for events |
| multi_accept_thread | blocked in accept() (100%) | Waiting for new TCP clients |
| 28 ANGLE-Worker threads | idle (100%) | No GPU work queued |
| virgl render server | idle in poll() (100%) | No commands arriving |
| 5 virgl GPU renderer threads (ctx 5,13,15,17,27) | idle | No work — 5 desktop sessions were active |
| vkr-ringmon, vkr-ring, vkr-queue threads | idle on cond_wait | Per-context Vulkan threads, no work |

### Deadlock chain

```
QEMU main loop thread
  ├── holds BQL
  ├── stuck in: virgl_resource_destroy_func → CFRelease → CF_IS_OBJC
  │   (releasing Metal/CoreFoundation object, never returns)
  │
  ├── blocks: all 14 vCPU threads (can't execute guest code)
  ├── blocks: fence_poll BH (can't dispatch — main loop stuck)
  ├── blocks: call_rcu_thread (can't run RCU callbacks)
  └── blocks: any new virtio command processing
```

The guest is completely frozen because no vCPU can run. The fence_poll thread is alive and scheduling BHs every 10ms, but those BHs never fire because the main loop is stuck in the same `virtio_gpu_gl_handle_ctrl` BH call.

## Open questions / Assumptions to validate

### Q1: Why does CFRelease never return?

**Hypothesis A**: Metal texture deallocation is waiting for GPU operations to complete. If the GPU has pending work referencing this texture, `CFRelease` (which triggers `[MTLTexture dealloc]`) might synchronously wait for GPU completion. But GPU completion needs the fence_poll BH to fire, which needs the main loop, which holds BQL — circular deadlock.

**Hypothesis B**: Objective-C runtime lock contention. Another thread (VideoToolbox encoder callback? keepalive thread?) holds an ObjC/CF lock that `CF_IS_OBJC` needs. But the keepalive thread appears to be sleeping, not in ObjC code.

**Hypothesis C**: Metal heap corruption from the earlier blob creation failures. The `RESP_ERR_UNSPEC` errors on `RESOURCE_CREATE_BLOB` indicate resource exhaustion. If this corrupted Metal's internal heap state, subsequent deallocations could spin/deadlock.

**Validated**: `ps -M` and `top` show 0.0% CPU for the QEMU process. The 199% from initial `ps aux` was accumulated CPU time. **`CF_IS_OBJC` is blocked (sleeping), not spinning.** This is consistent with a lock deadlock inside Metal/CoreFoundation, not a busy-wait. The thread is in `_pthread_mutex_firstfit_lock_wait` or a similar sleep primitive inside the ObjC/CF runtime.

Disassembly of `virgl_resource_destroy_func` confirms the stuck instruction is the `bl CFRelease` at offset 0x1467c, called for resource type 4 (CoreFoundation/Metal handle). The native handle at `res->native_handle` (offset 0x20) is being released.

### Q2: What caused the frame buffer saturation?

The "dropping frame" logs show QEMU's frame export TCP send buffers are nearly full. Two possibilities:

**Hypothesis A**: Guest not draining — the guest-side scanout_source/desktop-bridge processes are hung because the guest kernel is frozen, so they can't read from the TCP connections.

**Hypothesis B**: Browser disconnected — if the browser viewing the desktop stream disconnected, backpressure propagates: browser → WebSocket → Go API server → desktop-bridge → TCP → QEMU send buffer fills.

**Important**: The frame dropping is almost certainly a **symptom, not the cause**. The root cause is the CFRelease deadlock. But the frame drops may have started before the deadlock (during the resource exhaustion phase) or after (once the guest froze).

**To validate**: Check if there are active WebSocket connections in the Go API server logs. Check client connection timestamps in QEMU frame export logs.

### Q3: Did the blob creation failures trigger the deadlock?

The `RESP_ERR_UNSPEC` on `RESOURCE_CREATE_BLOB` happened first (logged on the guest serial console). This indicates Metal heap exhaustion or virglrenderer proxy failures. After these failures, the guest tried to `RESOURCE_UNREF` resources that were never created (getting `RESP_ERR_INVALID_RESOURCE_ID`).

But the deadlock is in a DIFFERENT `RESOURCE_UNREF` — one for a resource that DID exist. The blob creation failures may have:
1. Left Metal's heap in a fragmented/stressed state
2. Caused a buildup of resources needing cleanup
3. Triggered a cascade of unrefs that eventually hit the one that deadlocks

**To validate**: Check if `virgl_resource_destroy_func` can be called with the BQL dropped (similar to how `virgl_renderer_resource_create_blob` drops BQL).

## Proposed fix

### Fix 8: Drop BQL around virgl_renderer_resource_unref

Similar to the existing fix for `virgl_renderer_resource_create_blob` (which drops BQL because the proxy path can block on Metal):

```c
// In virgl_cmd_resource_unref():
// Drop BQL around resource destruction — virgl_resource_destroy_func
// calls CFRelease on Metal objects which can block on GPU heap operations.
// Holding BQL during this blocks all vCPU threads, freezing the VM.
bql_unlock();
virgl_renderer_resource_unref(unref.resource_id);
bql_lock();
```

**Risk**: Need to verify that `virgl_renderer_resource_unref` only accesses virglrenderer internal state and doesn't touch QEMU device model state. Looking at the code, it:
1. Removes from virglrenderer's hash table (`util_hash_table_remove`)
2. Calls `virgl_resource_destroy_func` which releases the native handle (Metal texture)
3. Does NOT access QEMU's virtio-gpu state, memory regions, or scanout data

The QEMU-side cleanup (memory region removal) happens separately via `virtio_gpu_cleanup_mapping` which runs BEFORE `virgl_renderer_resource_unref`. So dropping BQL should be safe.

**Alternative fix**: Make resource destruction asynchronous — queue the `CFRelease` to a separate cleanup thread that doesn't hold BQL. This is more complex but avoids the BQL unlock/relock overhead and any subtle races.

## Fix applied

Two call sites in `hw/display/virtio-gpu-virgl.c`:

1. **`virgl_cmd_resource_unref`** (line ~416): Moved `QTAILQ_REMOVE` before the BQL drop so no other code path can find the resource while it's being destroyed. Then `bql_unlock/lock` around `virgl_renderer_resource_unref`.

2. **`virgl_cmd_resource_create_blob` error path** (line ~1025): Added `bql_unlock/lock` around the cleanup `virgl_renderer_resource_unref`. Resource was never added to reslist so no QTAILQ_REMOVE needed.

The pattern matches the existing precedent at line ~985 where `virgl_renderer_resource_create_blob` drops BQL because the proxy path can block on Metal heap allocation.

## Timeline

- VM started: 2026-02-27 20:31 UTC (from QEMU launch time)
- Multiple desktop sessions started in parallel (demo scenario)
- Blob creation failures appeared on serial console
- VM became unresponsive (serial, SSH, HTTP dead)
- QEMU still running at 199% CPU, encoding keepalive frames
- Sample taken: 2026-02-28 23:01 UTC
- 5 virgl GPU contexts active (5 desktop sessions)

## Files

| File | Role |
|------|------|
| `hw/display/virtio-gpu-virgl.c:1275` | `virgl_cmd_resource_unref` — where BQL is held during CFRelease |
| `hw/display/virtio-gpu-virgl.c:976-987` | Existing BQL drop for `resource_create_blob` (precedent) |
| `hw/display/virtio-gpu-gl.c:100` | `virtio_gpu_gl_handle_ctrl` — BH that processes commands |
| `hw/display/helix/helix-frame-export.m:304` | Frame drop logging |
| `/tmp/qemu-sample.txt` | Process sample from incident |
