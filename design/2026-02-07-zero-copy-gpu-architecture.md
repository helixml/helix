# Zero-Copy GPU Video Capture Architecture

**Date:** 2026-02-07
**Status:** Design investigation

## The Question

Why can SPICE capture frames efficiently from virtio-gpu, but we're struggling with 15 FPS sending raw pixels over TCP?

## How SPICE Does It

SPICE reads directly from the **scanout** - the virtual monitor's framebuffer. Here's the chain:

```
Guest Mutter compositor
  → renders frame to GPU texture (virtio-gpu resource)
  → page flip: sets resource as "scanout" for virtual monitor
  → virtio-gpu sends RESOURCE_FLUSH command to QEMU
  → QEMU's virtio-gpu-virgl.c:virgl_cmd_resource_flush()
    → virgl_renderer_transfer_read_iov(resource_id, ...)
    → reads pixels from GPU resource into CPU DisplaySurface
    → dpy_gfx_update() notifies display backends (SPICE, Cocoa, etc.)
    → SPICE encodes and sends to client
```

Key point: **QEMU already gets notified on every frame** via `resource_flush`. It already reads the pixels via `virgl_renderer_transfer_read_iov()`. There's no TCP, no PipeWire, no GStreamer involved. The GPU resource ID is known because it's the scanout resource.

We actually had this working at **55 FPS** (commit `cc418f5f4` - "DisplaySurface approach"). The helix-frame-export.m code hooks into `virgl_cmd_resource_flush()` and calls `helix_update_scanout_displaysurface()` to capture every frame.

## Why We Switched Away From DisplaySurface

The DisplaySurface approach captures the **VM's scanout** - the VM's own display. But our desktop containers run inside Docker inside the VM, each with their own headless Mutter. Those headless Mutters do NOT render to the VM's scanout. They render to offscreen PipeWire ScreenCast buffers.

```
VM display (scanout 0) ← shows VM's login screen / desktop manager
  └── Docker container 1
       └── Mutter --headless --virtual-monitor 1920x1080
            └── renders to offscreen GPU buffers (NOT scanout)
            └── PipeWire ScreenCast provides frames to subscribers
  └── Docker container 2
       └── Mutter --headless --virtual-monitor 1920x1080
            └── same - offscreen, no scanout
```

So we built the PipeWire → vsockenc → TCP pipeline to get frames out of containers. But this copies 3.1MB of NV12 pixels over SLiRP TCP for every frame, limiting us to ~15 FPS.

## Your Idea: Multiple Scanouts

**"Can't we make many of those, one per desktop?"**

This is the right architectural insight. virtio-gpu supports multiple scanouts (virtual displays). If each container's Mutter rendered to its own scanout, QEMU could capture directly from GPU memory - no TCP needed.

### How virtio-gpu scanouts work

```
virtio-gpu device (QEMU)
  ├── scanout 0: 1920x1080 → UTM window (or headless)
  ├── scanout 1: 1920x1080 → headless (no window)
  ├── scanout 2: 1920x1080 → headless (no window)
  └── ...up to max_outputs (default 1, configurable)
```

Each scanout is a virtual monitor. In the guest, it appears as a DRM connector:
- `/dev/dri/card0`: connector-0 (scanout 0), connector-1 (scanout 1), etc.

When the guest sets a framebuffer on a connector and does a page flip, the `resource_flush` command tells QEMU which resource to read from. QEMU can read it via `virgl_renderer_transfer_read_iov()` without any guest cooperation needed.

### The challenge: containers sharing one GPU

The VM has one virtio-gpu. All containers share it via `/dev/dri/card0`. Currently:
- The VM's own display manager uses scanout 0 (connector-0)
- Containers use `--headless` mode (no connector at all)

For the multi-scanout approach:
1. Configure QEMU with `max_outputs=N` (e.g., 8)
2. Each container gets assigned a DRM connector (connector-1, connector-2, etc.)
3. Container's Mutter uses that connector instead of `--headless`
4. QEMU captures from the corresponding scanout
5. No UTM windows needed - just don't attach a display to those scanouts

### Implementation steps

**QEMU side:**
1. Increase `max_outputs` in virtio-gpu config
2. In `helix-frame-export.m`, watch all scanouts (not just scanout 0)
3. Each scanout gets its own encoder session
4. Map session IDs to scanout indices

**Guest side:**
1. Container startup detects available DRM connectors
2. Mutter uses `--headless` with a real DRM connector instead of virtual-monitor
3. Or: use `gnome-shell` without `--headless`, pointed at a specific connector

**Desktop-bridge side:**
1. Tell QEMU which scanout maps to which session via the existing TCP protocol
2. Receive H.264 frames from QEMU keyed by scanout index
3. Forward to WebSocket clients

## Alternative: Fix the Resource ID Path (Simpler)

Instead of multiple scanouts, we could fix the current architecture to use resource IDs:

### Current broken flow

```
PipeWire ScreenCast → pipewiresrc (SHM buffers, NOT DMA-BUF)
  → videoconvert (CPU: BGRx → NV12)
  → vsockenc: resource_id=0, sends 3.1MB pixels over TCP
  → QEMU: receives pixels, creates IOSurface, encodes
```

### Why it's SHM instead of DMA-BUF

1. Mutter ScreenCast CAN export DMA-BUF on virtio-gpu
2. But PipeWire negotiates LINEAR modifier (0x0) for virtio-gpu
3. Our pipewirezerocopysrc plugin rejects LINEAR as "not GPU-tiled"
4. Falls back to SHM (MemFd)

This is a conservative check designed for NVIDIA (where LINEAR means software fallback). On virtio-gpu, LINEAR is the ONLY modifier - it's still GPU memory, just not tiled.

### Fixed flow (if we accept LINEAR DMA-BUF)

```
PipeWire ScreenCast → pipewiresrc (DMA-BUF, LINEAR modifier)
  → vsockenc: extracts DMA-BUF FD
    → DRM_IOCTL_PRIME_FD_TO_HANDLE → GEM handle (per-process local)
    → DRM_IOCTL_VIRTGPU_RESOURCE_INFO → resource_id (virtio-gpu global)
    → sends resource_id (4 bytes) over TCP, NO pixel data
  → QEMU helix-frame-export.m:
    → virgl_renderer_resource_get_info_ext(resource_id) → Metal texture
    → Metal texture → IOSurface (zero-copy, GPU memory)
    → VideoToolbox encodes directly from IOSurface
```

**Key: the QEMU side already has true zero-copy.** It uses `virgl_renderer_resource_get_info_ext()`
to get the Metal texture handle, then extracts the IOSurface backing store. VideoToolbox encodes
directly from GPU memory. No `virgl_renderer_transfer_read_iov()` (CPU copy) needed.

This is confirmed in `qemu-utm/hw/display/helix/helix-frame-export.m` lines 286-320.

### Bug in current code

The vsockenc code at `gstvsockenc.c:421` has:
```c
/* For virtio-gpu, the GEM handle IS the resource ID */
resource_id = prime_handle.handle;  // WRONG!
```

**GEM handles are NOT resource IDs.** They're per-process local identifiers. The correct code needs a second ioctl:

```c
// Step 1: DMA-BUF fd → GEM handle
struct drm_prime_handle prime = { .fd = dmabuf_fd };
ioctl(drm_fd, DRM_IOCTL_PRIME_FD_TO_HANDLE, &prime);

// Step 2: GEM handle → virtio-gpu resource ID (MISSING!)
struct drm_virtgpu_resource_info info = { .bo_handle = prime.handle };
ioctl(drm_fd, DRM_IOCTL_VIRTGPU_RESOURCE_INFO, &info);
resource_id = info.res_handle;  // THIS is the real resource ID
```

The `drm_virtgpu_resource_info` struct is defined in `virtgpu_drm.h`:
```c
struct drm_virtgpu_resource_info {
    __u32 bo_handle;   // Input: GEM handle
    __u32 res_handle;  // Output: virtio-gpu resource ID
    __u32 size;
    __u32 blob_mem;
};
```

### But there's still the DMA-BUF problem

Even with the correct ioctl, pipewiresrc currently delivers **SHM buffers** on virtio-gpu (because LINEAR modifier is rejected). To get DMA-BUF:

Option A: Use native `pipewiresrc` without `always-copy` and request DMA-BUF caps:
```
pipewiresrc ! video/x-raw(memory:DMABuf) ! vsockenc
```

Option B: Fix pipewirezerocopysrc to accept LINEAR modifier on virtio-gpu.

Option C: Skip PipeWire entirely and use the scanout approach.

## Comparison

| Approach | FPS | Complexity | Pixel copies | Notes |
|----------|-----|-----------|-------------|-------|
| Current: SHM → TCP pixels | 15 | Low (working) | 2 (CPU convert, TCP transfer) | 3.1MB NV12 per frame over TCP |
| DMA-BUF resource ID | 60? | Medium | 0 (true zero-copy) | Metal texture → IOSurface → VideoToolbox |
| Multiple scanouts | 55+ | High | 0 (true zero-copy) | Each container gets own DRM connector |
| Scanout 0 capture (old) | 55 | N/A | 0 | **NOT VIABLE** - captures VM display, not container desktop |

## Recommendation

**Phase 1: Fix the DMA-BUF resource ID path** (medium effort)
1. Make pipewiresrc deliver DMA-BUF on virtio-gpu (accept LINEAR modifier)
2. Fix vsockenc to use `DRM_IOCTL_VIRTGPU_RESOURCE_INFO` for correct resource_id
3. Fix QEMU helix-frame-export to handle resource_id != 0 via `virgl_renderer_transfer_read_iov()`
4. This eliminates 3.1MB TCP transfer per frame

**Phase 2: Multiple scanouts** (future, if needed)
- More architecturally clean
- Each container gets dedicated GPU output
- QEMU captures directly on page flip
- No PipeWire involved at all
- But requires QEMU, guest kernel, and Mutter configuration changes

## Validated Claims

1. **DRM_IOCTL_VIRTGPU_RESOURCE_INFO**: Confirmed in Mesa source (`vn_renderer_virtgpu.c:664-678`). `bo_handle` is input GEM handle, `res_handle` is output resource ID.
2. **DRM_IOCTL_GEM_CLOSE**: Required after PRIME import to avoid leaking GEM handles. Mesa always closes them after use.
3. **QEMU zero-copy path**: `helix-frame-export.m` already uses `virgl_renderer_resource_get_info_ext()` → Metal texture → IOSurface. This is true zero-copy, no `virgl_renderer_transfer_read_iov()` needed.
4. **GEM handle != resource_id**: Confirmed by existence of `VIRTGPU_RESOURCE_INFO` ioctl and Mesa's explicit two-step usage pattern.

## Tested and Failed

1. **DMA-BUF with LINEAR modifier via pipewirezerocopysrc**: PipeWire returns "no more input formats". Mutter on virtio-gpu headless does NOT support DMA-BUF export for ScreenCast - only SHM (MemFd). Tested 2026-02-07.

2. **Native pipewiresrc without videoconvert**: Delivers SHM buffers. vsockenc falls back to sending raw 8.3MB BGRA over TCP (~12 FPS).

## Current Approach: Multiple Scanouts (In Progress)

### QEMU Configuration
Current: `-device virtio-gpu-gl-pci` (max_outputs=1, one Virtual-1 connector)
Target: `-device virtio-gpu-gl-pci,max_outputs=16` (16 connectors available)

Guest sees DRM connectors at `/sys/class/drm/card0-Virtual-{1..16}`.
Each container's Mutter runs against a dedicated connector.

### Architecture
```
QEMU (max_outputs=16)
  ├── scanout 0: VM display (GDM/login, or unused)
  ├── scanout 1: Container A's Mutter → resource_flush → helix-frame-export → H.264
  ├── scanout 2: Container B's Mutter → resource_flush → helix-frame-export → H.264
  └── ...
```

Each scanout fires resource_flush independently on page flip (damage-based).
QEMU reads GPU memory via Metal IOSurface and encodes with VideoToolbox.

### Steps
1. [x] Verify QEMU max_outputs parameter (VIRTIO_GPU_MAX_SCANOUTS=16)
2. [x] Add max_outputs=16 to QEMU and UTM
   - Bumped VIRTIO_GPU_MAX_SCANOUTS from 16 to 32 in qemu-utm fork
   - Changed default max_outputs from 1 to 16 in qemu-utm fork
   - Modified UTM Swift source to pass max_outputs=16 for virtio-gpu devices
   - UTM AdditionalArguments plist approach did NOT work (UTM ignores them)
   - Instead: added max_outputs property directly in displayArguments builder
   - Rebuilding UTM to apply changes
3. [ ] Verify guest sees 16 DRM connectors
4. [ ] Test Mutter on a non-primary connector from inside container
5. [ ] Modify helix-frame-export to watch multiple scanouts
6. [ ] Map session IDs to scanout indices
7. [ ] Hydra allocates scanout index per container

## Open Questions

1. Can a Docker container do DRM modesetting on a specific connector? Needs /dev/dri/card0 access and possibly DRM master.
2. Does Mutter support running on a secondary connector without being DRM master? (logind seat mechanism)
3. How does gnome-shell select which connector to use? `--virtual-monitor` is headless-only. For real connectors, it auto-detects via DRM.
4. With Venus (Vulkan) vs virgl (OpenGL), does the resource info path differ?
