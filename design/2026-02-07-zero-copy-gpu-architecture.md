# Zero-Copy GPU Video Capture Architecture

**Date:** 2026-02-07
**Status:** GPU blit path working - zero CPU copies per frame

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ macOS Host                                                   │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ QEMU (virtio-gpu-gl-pci, max_outputs=16)            │    │
│  │                                                      │    │
│  │  helix-frame-export.m (v7-gpu-blit)                  │    │
│  │    TCP:15937 (127.0.0.1, forwarded via SLiRP)        │    │
│  │    ├─ Accepts multiple TCP clients concurrently      │    │
│  │    ├─ SUBSCRIBE(scanout_id) → client gets H.264 push│    │
│  │    ├─ ENABLE_SCANOUT(id,w,h) → hotplug connector    │    │
│  │    ├─ Per-scanout VideoToolbox encoder sessions      │    │
│  │    └─ On resource_flush for scanout N:               │    │
│  │         1. Read GPU resource → IOSurface             │    │
│  │         2. VideoToolbox H.264 encode (zero-copy)     │    │
│  │         3. Push H.264 to all clients subscribed to N │    │
│  └─────────┬───────────────────────────────────────────┘    │
│             │ SLiRP: guest 10.0.2.2:15937 → host 127.0.0.1 │
│             │                                                │
│  Browser ←──WebSocket──← Helix API ←──RevDial──← Container  │
└─────────────┼───────────────────────────────────────────────┘
              │
┌─────────────┼───────────────────────────────────────────────┐
│ Linux VM    │ (Ubuntu 25.10, aarch64)                        │
│             │                                                │
│  /dev/dri/card0 (virtio-gpu, 16 connectors: Virtual-1..16)  │
│  Virtual-1: linux console (no GDM - disabled)               │
│  Virtual-2..16: available for container desktops             │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ helix-drm-manager daemon (/usr/local/bin/)           │    │
│  │   /run/helix-drm.sock (Unix socket)                  │    │
│  │   Sole DRM master on /dev/dri/card0                  │    │
│  │                                                      │    │
│  │   On lease request from container:                   │    │
│  │     1. Allocate free scanout index (1-15)            │    │
│  │     2. TCP→QEMU: ENABLE_SCANOUT(idx, 1920, 1080)    │    │
│  │     3. echo 1 > /sys/class/drm/card0-Virtual-N/status│   │
│  │     4. DRM_IOCTL_MODE_CREATE_LEASE(connector, crtc)  │    │
│  │     5. Send lease FD to container via SCM_RIGHTS     │    │
│  │     → Returns: {scanout_id, connector_name, lease_fd}│    │
│  └──────────┬──────────────────────────────────────────┘    │
│             │                                                │
│  ┌──────────┼──────────────────────────────────────────┐    │
│  │ Container A (Docker)                                 │    │
│  │                                                      │    │
│  │  startup-app.sh (modified for scanout mode):         │    │
│  │    1. Connect to /run/helix-drm.sock                 │    │
│  │    2. Request lease → get scanout_id + lease FD      │    │
│  │    3. Start D-Bus system bus inside container        │    │
│  │    4. Start logind-stub --lease-fd=N                 │    │
│  │    5. Start gnome-shell --display-server --wayland   │    │
│  │       → Mutter calls TakeDevice(226, minor)          │    │
│  │       → logind-stub returns lease FD                 │    │
│  │       → Mutter uses lease FD as DRM device           │    │
│  │       → Mutter renders to Virtual-N connector        │    │
│  │       → Page flip → virtio-gpu resource_flush        │    │
│  │                                                      │    │
│  │  desktop-bridge (scanout mode):                      │    │
│  │    1. TCP connect to 10.0.2.2:15937 (QEMU)          │    │
│  │    2. Send SUBSCRIBE(scanout_id)                     │    │
│  │    3. Receive pre-encoded H.264 from QEMU            │    │
│  │    4. Forward to WebSocket clients                   │    │
│  │    → No GStreamer, no PipeWire for video              │    │
│  │    → PipeWire still used for AUDIO only              │    │
│  │                                                      │    │
│  │  Zed IDE, Qwen Code, dev tools (unchanged)          │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### Connection Inventory

| From | To | Protocol | Purpose |
|------|-----|----------|---------|
| helix-drm-manager | QEMU | TCP 10.0.2.2:15937 | ENABLE/DISABLE_SCANOUT |
| container startup | helix-drm-manager | Unix /run/helix-drm.sock | Request DRM lease (SCM_RIGHTS) |
| desktop-bridge | QEMU | TCP 10.0.2.2:15937 | SUBSCRIBE + receive H.264 |
| Mutter | logind-stub | D-Bus system bus | TakeDevice → lease FD |
| logind-stub | (inherited) | FD inheritance | Lease FD from startup script |
| desktop-bridge | Helix API | RevDial WebSocket | H.264 → browser |
| desktop-bridge | Mutter | D-Bus session bus | Screenshot, window mgmt |

### What Runs Where

| Component | Runs In | Binary | Built By |
|-----------|---------|--------|----------|
| helix-frame-export | QEMU (macOS host) | qemu-aarch64-softmmu | build-qemu-standalone.sh |
| helix-drm-manager | VM (systemd service) | /usr/local/bin/helix-drm-manager | go build |
| logind-stub | Container | /usr/local/bin/logind-stub | go build (in desktop image) |
| gnome-shell | Container | /usr/bin/gnome-shell | apt (GNOME 49) |
| desktop-bridge | Container | /usr/local/bin/desktop-bridge | go build (in desktop image) |

### Backward Compatibility (NVIDIA/AMD)

The scanout mode is ONLY used on macOS ARM (virtio-gpu). Detection:

```go
// In ws_stream.go
isMacOSVirtioGpu := !isSway && checkGstElement("vsockenc")
if isMacOSVirtioGpu && os.Getenv("HELIX_VIDEO_MODE") != "pipewire" {
    // Scanout mode: subscribe to QEMU for H.264
    return startScanoutStream(...)
} else {
    // PipeWire mode: existing GStreamer pipeline (NVIDIA/AMD)
    return startPipeWireStream(...)
}
```

On NVIDIA/AMD: No helix-drm-manager, no logind-stub, no scanout. PipeWire ScreenCast
pipeline unchanged. The container Dockerfile conditionally includes these components.

### Verification Status

| Connection | Status | Notes |
|-----------|--------|-------|
| helix-drm-manager → QEMU TCP | ✅ VERIFIED | ENABLE_SCANOUT works, connector goes connected |
| Container → helix-drm-manager | ✅ VERIFIED | Lease FD received via SCM_RIGHTS |
| desktop-bridge → QEMU TCP | ✅ VERIFIED | SUBSCRIBE works, gets SUBSCRIBE_RESP |
| Mutter → logind-stub D-Bus | ✅ VERIFIED | TakeDevice(226,0) returns lease FD, Mutter creates GBM renderer |
| Mutter sets mode on lease | ✅ VERIFIED | Virtual-2 goes enabled=enabled, mode 1920x1080@75 |
| DRM lease with planes | ✅ VERIFIED | Need UNIVERSAL_PLANES cap set before lease creation |
| QEMU auto-encode on page flip | ✅ VERIFIED | **124.6 FPS** with drm-flipper continuous page flips |
| H.264 → TCP subscriber | ✅ VERIFIED | 1256 frames in 10s, 282B P-frames, 6.2KB keyframes |
| Mutter modeset on lease FD | ✅ VERIFIED | logind-stub session OK, GUdev tag index fix, Mutter renders via atomic modesetting |
| Container scanout integration | ✅ VERIFIED | gnome-shell --display-server works in container with DRM lease |
| GNOME → QEMU H.264 in container | ✅ VERIFIED | 88KB keyframe received via SUBSCRIBE(scanout_id) from container's GNOME desktop |
| GNOME → H.264 sustained | ✅ VERIFIED | 41 frames in 12.2s (3.4 FPS) during overview toggle, 74KB keyframe + 0.5-17KB P-frames |
| desktop-bridge scanout mode | ✅ VERIFIED | ScreenCast skipped, RemoteDesktop for input, video from QEMU TCP |
| H.264 → WebSocket end-to-end | ✅ VERIFIED | 84 video frames in 25.3s (3.3 FPS). 74KB IDR keyframe + 420B P-frames on static desktop |

### RESOLVED: QEMU Double-Init Bug

**Problem**: `helix_scanout_frame_ready()` was called but always saw `active_clients=0`.

**Root cause**: QEMU calls `helix_frame_export_init()` twice during boot (once for
initial display, once after display resize). The second call created a new
`HelixFrameExport` struct and replaced `g_helix_export`, but the TCP server thread
kept running with the old struct. Subscribers connected to the old TCP server
were invisible to the new `g_helix_export`.

**Fix**: If `g_helix_export` is already valid on second init, just update the
`virtio_gpu` pointer and return. The existing TCP server and client list are preserved.
(QEMU commit `3c65cca992`)

### RESOLVED: Kernel SET_SCANOUT Theory Was Wrong

Investigation confirmed that DRM leases do NOT renumber the scanout_id.
The kernel virtio-gpu driver sends SET_SCANOUT with the correct scanout_id
(uses `output->index` which is set during initialization and never changes).
The `resource_flush → scanout matching` works correctly when SET_SCANOUT is called.

### RESOLVED: Mutter Needs Pre-activated CRTC

**Problem**: Mutter can't do the first modeset on an inactive CRTC through a DRM lease.
The `drmModeGetPlane()` through the lease FD returns 0 formats for the IN_FORMATS
property (expected - lease doesn't expose it). Mutter falls back to legacy format
enumeration, but still can't complete the modeset on a cold CRTC.

**Fix**: `helix-drm-manager` now does an initial `drmModeSetCrtc` with a dumb buffer
BEFORE creating the DRM lease. This puts the CRTC in an active state (mode set,
connector enabled, framebuffer attached). When Mutter gets the lease, it sees
`CRTC active: 1, mode: 1920x1080` and successfully inherits the display.

### RESOLVED: Mutter "No GPUs found" in Container

**Problem**: gnome-shell in `--display-server` mode uses GUdev to enumerate DRM card
devices. It calls `g_udev_enumerator_add_match_tag("seat")` which uses libudev's
`udev_enumerate_add_match_tag()`. This function uses a **reverse tag index** at
`/run/udev/tags/<tag_name>/` rather than checking each device's tags.

We were creating the udev database entry at `/run/udev/data/c226:0` with `G:seat`
and `Q:seat` tags, but NOT the tag index directories. `udevadm info` could read
the device properties correctly, but enumeration returned 0 devices.

**Fix**: Create tag index directories in addition to the database entry:
```bash
# In detect-render-node.sh
mkdir -p /run/udev/tags/seat /run/udev/tags/mutter-device-preferred-primary
touch /run/udev/tags/seat/c226:0
touch /run/udev/tags/mutter-device-preferred-primary/c226:0
```

After this fix, Mutter successfully finds the GPU:
```
Added device '/dev/dri/card0' (virtio_gpu) using atomic mode setting.
Created gbm renderer for '/dev/dri/card0'
GPU /dev/dri/card0 selected primary given udev rule
Using Wayland display name 'wayland-0'
```

### RESOLVED: Scanout ID Off-by-One (was a red herring)

**Observation**: With stale scanout allocations (from leaked test containers), the
kernel's SET_SCANOUT scanout_id appeared to be pool_index + 1. This was caused by
corrupted state from 15 leaked scanout allocations, NOT an actual off-by-one.

**With a clean restart**: pool index 1 → ENABLE_SCANOUT(1) → kernel SET_SCANOUT
scanout_id=1. **No offset needed.** The ScanoutID returned by helix-drm-manager
is the same as the pool index (scanoutIdx), and matches the QEMU scanout ID.

**Lesson**: Always test with clean state. Leaked DRM leases corrupt the mapping.

### Desktop-Bridge Scanout Mode

Added `VideoModeScanout` to desktop-bridge (`api/pkg/desktop/scanout_source.go`).
When `HELIX_VIDEO_MODE=scanout` or `?videoMode=scanout` is set:

1. Bypasses PipeWire and GStreamer entirely
2. Requests DRM lease from helix-drm-manager (gets scanout ID)
3. Connects to QEMU TCP 10.0.2.2:15937
4. Sends SUBSCRIBE(scanout_id)
5. Receives pre-encoded H.264 frames from QEMU's VideoToolbox encoder
6. Forwards to WebSocket clients using the existing binary protocol

This is backward compatible - existing NVIDIA/AMD PipeWire paths are unchanged.

### Performance Results

| Test | FPS | Frame Size | Notes |
|------|-----|-----------|-------|
| drm-flipper (60 FPS page flips) | **124.6** | 282B P / 6.2KB I | Full pipeline verified |
| Static VM console (scanout 0) | 5.0 | 236-248B P / 6.8KB I | Damage-based, no active rendering |
| modetest test pattern | 9.8 | 224-254B P / 6.9KB I | Static after initial pattern |
| **Container GNOME → WebSocket** | **3.3** | 420B P / 74KB I | Full E2E: 84 frames in 25.3s. Static desktop with cursor blink |
| Container GNOME overview animation | ~7-8 | 11-17KB P | During GNOME overview toggle burst |

| **3x parallel containers** | **10.1 total** | 2.5-5.5KB avg | 3 GNOME desktops via WebSocket, ghostty terminal damage, 292 kb/s total |

Expected with active desktop (typing, window movement): **15-30 FPS** based on damage frequency.
Expected with vkcube/games: **55-60 FPS** at 1920x1080 (constant GPU damage).

### Multi-Desktop Stability & Performance (2026-02-08)

**Test: 3 concurrent GNOME desktop sessions at 1080p, streaming H.264 via WebSocket**

| Metric | Value | Notes |
|--------|-------|-------|
| VM stability | ✅ Stable | Load ~14.7, 9.6GB of 51GB RAM used |
| QEMU CPU usage | 460% | virglrenderer command translation is the bottleneck |
| QEMU RSS | 32GB | Shared across all scanouts + virglrenderer |
| GPU utilization | 47% active, 495mW | NOT the bottleneck |
| Metal shader failures | 222 in 3 min | Not fatal at 1080p, causes artifacts |
| Guest iowait | 37.6% | Disk contention between containers |

**5K resolution (5120x2880): UNSTABLE with multiple sessions**

Two crash modes discovered:
1. **VTEncoderXPCServ memory limit**: macOS limits VideoToolbox's XPC service to 1GB RSS.
   At 5K, encoding buffers exceed this → `EXC_RESOURCE` kill → VM hangs.
   Log: `VTEncoderXPCServ [77678] crossed memory high watermark (1000 MB)`
2. **Metal shader compilation failures**: 3 concurrent GNOME sessions overwhelm the
   Metal shader compiler. ~401 failures/min at 5K. Eventually deadlocks QEMU.

**Resolution**: Default agent resolution to 1080p (via app's `external_agent_config.resolution`).
QEMU EDID still advertises 5K max — individual sessions can request higher if needed.

**Remaining bottleneck**: virglrenderer CPU-side command translation. Each guest Vulkan/GL
call goes through Venus → virglrenderer → Metal (MoltenVK). This is purely CPU-bound.
The GPU is only 47% utilized — the translation layer can't feed it fast enough.

### Zero-Copy Frame Path: WORKING via GPU Blit (2026-02-08)

**Status:** GPU blit path is working. Zero CPU copies per frame.

**Solution (v7-gpu-blit):** Instead of trying to extract Metal textures from virglrenderer
(which requires `console_has_gl()` and SPICE display listener registration — complex and
fragile), we use a direct GPU blit approach via EGL/ANGLE:

1. At init: create a shared EGL context (shares textures with virglrenderer's context)
2. Per-scanout: create IOSurface → EGL pbuffer (via `EGL_IOSURFACE_ANGLE`) → GL texture + FBOs
3. Per-frame: `glBlitFramebuffer(virgl_tex_id → IOSurface)`, `glFinish()`, pass IOSurface
   to VideoToolbox for H.264 encoding

This approach mirrors how SPICE's IOSurface blit works (`spice-display.c:spice_iosurface_blit_egl`)
but targets VideoToolbox encoding instead of display. The key insight is that virglrenderer's
`tex_id` (from `virgl_renderer_resource_get_info().tex_id`) is a valid GL texture ID that's
shared across all EGL contexts in the same share group.

**Verified working at 1920x1080:**
```
[GPU_BLIT] GPU blit initialized: shared EGL context=0x4 (share=0x3)
[GPU_BLIT] ANGLE native device=0x9f3350000
[GPU_BLIT] Setup scanout 0: 1920x1080 IOSurface=0x9f18281b0 tex=4 dst_fbo=1 src_fbo=2 target=0xde1
[FRAME_READY] GPU blit #1: scanout=0 tex_id=3 1920x1080
[ENCODE] scanout=0 frame=1 status=0 1920x1080
```

**Performance:** GPU blit via `glBlitFramebuffer` is sub-millisecond on Apple Silicon even at 5K,
compared to the old CPU readback via `virgl_renderer_transfer_read_iov()` which copied 59MB/frame
at 5K (5120×2880×4 bytes).

**Three encoding paths in priority order:**
1. **GPU blit** (new, primary): `tex_id` → `glBlitFramebuffer` → IOSurface → VideoToolbox
2. **Metal IOSurface snapshot** (legacy): if `metal_iosurface` is set at SET_SCANOUT time
3. **CPU readback** (last resort): `virgl_renderer_transfer_read_iov()` → IOSurface

The GPU blit path is always taken when EGL is available (which it always is on macOS/ANGLE).

#### Previous approach (abandoned): Register as GL Display Listener

The original fix plan was to register helix as a GL display listener (like SPICE does) to
receive Metal textures via `dpy_gl_scanout_texture()`. This required:
- Stub `DisplayGLCtxOps` and `DisplayChangeListenerOps`
- Registration on each scanout console via `qemu_console_set_display_gl_ctx()`
- Potential conflicts with SPICE on console 0

The GPU blit approach is simpler and more robust — it works at the virglrenderer resource level
rather than the display listener level, so it doesn't interfere with SPICE or require display
console registration.

#### Git history
- `3c3343508d` — Eliminated triple-copy (3× 59MB → 1× 59MB per frame at 5K)
- `814e59d757` — Added zero-copy via Metal texture IOSurface capture code (never reached)
- `cb9b329a8e` — **GPU blit path** via EGL/ANGLE shared context + IOSurface pbuffer
- `985cea2890` — Fix missing ANGLE IOSurface hint constants

### Encoding Artifacts Investigation

**Symptom**: Visible encoding artifacts and flickering on GNOME desktop at 5K.

**Likely cause**: Race condition on shared IOSurface. The Metal texture's IOSurface is
written to by virglrenderer (guest rendering) and simultaneously read by VideoToolbox
(encoding). There's no GPU synchronization fence between "guest finishes rendering" and
"encoder reads the surface." This produces tearing/corruption in encoded frames.

**Possible fixes**:
1. Insert a Metal fence (`MTLFence` or `MTLEvent`) between rendering and encoding
2. Double-buffer: alternate between two IOSurfaces per scanout
3. Use `IOSurfaceLock()` to serialize access (but adds latency)

### Vulkan Driver: KosmicKrisp (RECOMMENDED over MoltenVK)

UTM supports two Vulkan drivers for the Venus path:
- **MoltenVK**: Mature, widely tested, translates Vulkan → Metal. Suffers from massive
  shader compilation failures under concurrent GNOME sessions (401+ failures/min).
- **KosmicKrisp** ✅: Mesa-based Vulkan driver, requires macOS 15+ (Sequoia).
  **Dramatically better rendering quality** — eliminates most MoltenVK shader artifacts.

**Test result (2026-02-08)**: Switching from MoltenVK to KosmicKrisp at 1080p eliminated
the severe rendering artifacts and flickering. Only slight encoder artifacts remain
(attributed to the IOSurface race condition, not the Vulkan driver).

This is a **global UTM setting**, not per-VM. To switch:
```bash
# Switch to KosmicKrisp (RECOMMENDED)
defaults write com.utm.app QEMUVulkanDriver -int 3

# Switch back to MoltenVK
defaults write com.utm.app QEMUVulkanDriver -int 2
```

Both are pre-bundled in UTM. The setting controls which ICD JSON file is pointed to by
`VK_DRIVER_FILES` environment variable when QEMU launches. VM restart required after change.

### Issues Discovered & Fixed

1. **DRM_FB_helper CPU hog on reboot**: Enabled scanouts persist across guest reboot.
   Fixed by resetting `enabled_output_bitmask=1` in `virtio_gpu_base_reset()`.

2. **DRM lease ENOSPC with planes**: `DRM_CLIENT_CAP_UNIVERSAL_PLANES` must be set
   on the master FD BEFORE creating leases with plane objects. Without it, plane IDs
   don't exist and `MODE_CREATE_LEASE` returns ENOSPC.

3. **Mutter "Plane has no advertised formats"**: Even with planes in the lease,
   Mutter reports no formats. This doesn't prevent mode setting but may affect
   rendering quality.

4. **FD inheritance across exec**: Go's `exec.Command` sets CLOEXEC on all FDs.
   Must use `ExtraFiles` to pass the lease FD to child processes.

5. **logind D-Bus Seat property type**: Must use `(so)` struct type, not `[2]interface{}`.

## Key Benefits

- **Zero CPU copies (PLANNED, not yet working)**: Mutter renders to GPU -> QEMU reads Metal IOSurface -> VideoToolbox encodes
- **Damage-based**: Only encodes on page flip. Static screen = 0 frames captured, ~2 FPS broadcast keepalive
- **No PipeWire**: Video capture bypasses PipeWire entirely. PipeWire still used for audio
- **No TCP pixel transfer**: Only resource IDs over TCP, not 3.1MB pixel data per frame
- **Expected FPS**: 55+ (based on earlier DisplaySurface approach at 55 FPS)
- **Resolution**: Full native 1920x1080 (or whatever the container's Mutter is configured for)

## Verified Steps

### 1. QEMU max_outputs=16

- Bumped `VIRTIO_GPU_MAX_SCANOUTS` from 16 to 32 in qemu-utm fork
- Changed default `max_outputs` from 1 to 16
- Fixed static assertion (`sizeof(virtio_gpu_resp_display_info)`: 408 -> 792)
- Built via `./for-mac/qemu-helix/build-qemu-standalone.sh`
- Guest sees 16 DRM connectors (`[drm] number of scanouts: 16`)
- **DO NOT modify UTM source** - just patch QEMU binary into UTM.app

### 2. On-demand scanout hotplug

- Added `HELIX_MSG_ENABLE_SCANOUT` (0x20) and `HELIX_MSG_DISABLE_SCANOUT` (0x21) to protocol
- Protocol defined in both `gstvsockenc.h` (helix repo) and `helix-frame-export.h` (qemu-utm repo)
- QEMU handler: `helix_enable_scanout()` in `virtio-gpu-base.c`
  - Sets `req_state[scanout_id].width/height`
  - Sets `enabled_output_bitmask`
  - Triggers `VIRTIO_GPU_EVENT_DISPLAY` config interrupt
- Made `virtio_gpu_notify_event()` non-static for helix access
- Guest reprobe required: `echo 1 > /sys/class/drm/card0-Virtual-N/status`
- Tested: Virtual-2 goes from disconnected to connected with 26 modes

### 3. DRM lease

- GDM disabled (`systemctl disable gdm`) - VM console on Virtual-1 is sufficient
- After GDM gone, `DRM_IOCTL_SET_MASTER` succeeds
- `DRM_IOCTL_MODE_CREATE_LEASE` works: leased connector 45 + CRTC 51, got lessee_id=1
- Lease FD is an independent DRM master controlling only leased resources
- Multiple containers can each have their own lease (no DRM master conflicts)
- Mutter does NOT need forking - it accepts DRM devices via logind/FD

### 4. DMA-BUF path (FAILED - not needed with scanout approach)

- PipeWire ScreenCast on Mutter with virtio-gpu headless does NOT export DMA-BUF
- Tested: `pipewirezerocopysrc buffer-type=dmabuf-passthrough` -> "no more input formats"
- Root cause: Mutter on virtio-gpu only offers SHM (MemFd) for ScreenCast, not DMA-BUF
- The vsockenc VIRTGPU_RESOURCE_INFO code is correct but can't be exercised without DMA-BUF
- **This approach is abandoned in favor of multiple scanouts**

## Remaining Implementation

### helix-drm-manager daemon (Go, runs in VM)

Location: `api/cmd/helix-drm-manager/main.go`

**Responsibilities:**
1. Open `/dev/dri/card0`, call `DRM_IOCTL_SET_MASTER`
2. Listen on `/run/helix-drm.sock` (Unix socket)
3. Maintain pool of available scanout indices (1-15)
4. On lease request from container:
   - Allocate next free scanout index
   - Connect to QEMU TCP:15937, send `HELIX_MSG_ENABLE_SCANOUT`
   - Wait for `HELIX_MSG_SCANOUT_RESP`
   - Trigger connector reprobe: `echo 1 > /sys/class/drm/card0-Virtual-{N+1}/status`
   - Create DRM lease: `DRM_IOCTL_MODE_CREATE_LEASE(connector_id, crtc_id)`
   - Send lease FD to container via `SCM_RIGHTS` on Unix socket
5. On container disconnect:
   - Send `HELIX_MSG_DISABLE_SCANOUT` to QEMU
   - Release scanout index

**DRM ioctl constants (arm64 Linux):**
```
DRM_IOCTL_SET_MASTER = _IO('d', 0x1e) = 0x641e
DRM_IOCTL_DROP_MASTER = _IO('d', 0x1f) = 0x641f
DRM_IOCTL_MODE_CREATE_LEASE = _IOWR('d', 0xc6, 24)
```

**Connector ID mapping:**
- Virtual-1: connector_id=38 (scanout 0, VM console)
- Virtual-2: connector_id=45 (scanout 1)
- Virtual-3: connector_id=52 (scanout 2)
- Pattern: connector_id = 38 + (scanout_index * 7)
- CRTC IDs: 37, 44, 51, 58, 65, 72, ... (pattern: 37 + scanout_index * 7)

### Container startup changes

In `desktop/ubuntu-config/startup-app.sh` (or a new script):
1. Connect to `/run/helix-drm.sock`
2. Request a DRM lease (send scanout request, receive lease FD via SCM_RIGHTS)
3. Configure Mutter to use the lease FD instead of `--headless --virtual-monitor`
4. Mutter env vars: `GDK_BACKEND=drm`, `MUTTER_DEBUG_FORCE_KMS=1`, or pass lease FD via logind

### QEMU helix-frame-export changes

Currently: only captures scanout 0 (VM display) via `helix_update_scanout_displaysurface()`
Needed: capture ALL active scanouts independently

The `virgl_cmd_resource_flush()` callback already iterates `for (i = 0; i < max_outputs; i++)` and
calls `helix_update_scanout_displaysurface(g, i, rf.resource_id)` per scanout. Each scanout can have
its own VideoToolbox encoder session. The H.264 output needs to be tagged with the scanout index
so desktop-bridge knows which container's stream it belongs to.

### Desktop-bridge changes

**Video mode selection** (backward compatible with NVIDIA/AMD):

```
VideoMode detection in ws_stream.go:
  if HELIX_VIDEO_MODE=scanout → scanout mode (macOS virtio-gpu)
  else if isMacOSVirtioGpu   → scanout mode (auto-detected)
  else if isNvidiaGnome       → zerocopy mode (CUDA DMA-BUF)
  else if isSway              → shm mode (wlroots)
  else if isAmdGnome          → zerocopy mode (EGL DMA-BUF)
```

**Scanout mode flow** (new, macOS ARM only):
1. desktop-bridge detects macOS/virtio-gpu environment
2. Connects to `/run/helix-drm.sock`, requests DRM lease
3. Gets scanout ID and lease FD
4. Connects to QEMU `10.0.2.2:15937`, sends `SUBSCRIBE(scanout_id)`
5. Receives pre-encoded H.264 frames from QEMU (zero-copy on host)
6. Forwards H.264 directly to WebSocket clients
7. No GStreamer, no PipeWire, no in-container encoding

**PipeWire mode flow** (existing, NVIDIA/AMD):
- Unchanged. PipeWire ScreenCast → pipewiresrc/pipewirezerocopysrc → nvenc/vaapi → WebSocket
- This path is NOT affected by the scanout changes

**Integration points (all implemented):**
- `ws_stream.go`: `VideoModeScanout` auto-detected via `HELIX_VIDEO_MODE` env var
- `scanout_source.go`: TCP reader for QEMU H.264 frames, implements `VideoFrame` channel
- `detect-render-node.sh`: Exports `HELIX_SCANOUT_MODE=1` and `HELIX_VIDEO_MODE=scanout` for virtio-gpu
- `startup-app.sh`: Branches gnome-shell launch: `--display-server` (scanout) vs `--headless` (standard)
- `start-desktop-bridge.sh`: Passes `HELIX_VIDEO_MODE` to desktop-bridge
- `devcontainer.go`: Bind-mounts `/run/helix-drm.sock` into virtio-gpu containers
- `Dockerfile.ubuntu-helix`: Builds `logind-stub` and `mutter-lease-launcher` for ARM64
- PipeWire still started for audio even in scanout mode

## QEMU Commits (helixml/qemu-utm, branch utm-edition-venus-helix)

- `f18edbc` - feat: Increase max scanouts to 32, default to 16
- `b91ef275f3` - fix: Update static assertion for 32 scanouts (408 -> 792 bytes)
- `3b2de2f062` - feat: Enable all scanouts at startup (REVERTED - kills GDM)
- `08f4b9bcd7` - Revert enable all scanouts at startup
- `bd644b1e35` - feat: HELIX_MSG_ENABLE_SCANOUT handler
- `4d51b7f989` - fix: Move scanout helpers to virtio-gpu-base.c
- `9cfd078e36` - fix: Add scanout message type constants to header
- `8af26aae47` - fix: Handle ENABLE_SCANOUT in server thread dispatch
- `ea91ab699c` - feat: Multi-client TCP server with per-scanout auto-encoding
- `7116968c86` - fix: Reset enabled_output_bitmask on guest reboot
- `7eec31cbbc` - debug: SET_SCANOUT and resource_flush logging to helix-debug.log
- `3c65cca992` - fix: Prevent double-init from orphaning TCP server clients
- `4f9d92a605` - debug: Add fe pointer comparison logging to subscribe handler

## Helix Commits (feature/macos-arm-desktop-port)

Key commits:
- `4b1da106f` - feat: Zero-copy GPU path - extract virtio-gpu resource IDs from DMA-BUF
- `a169355f6` - feat: Build pipewirezerocopysrc for ARM64
- `74ad04545` - fix: Install pipewirezerocopysrc plugin on ARM64
- `94e0990dc` - fix: DmaBuf passthrough caps
- `1e62082fd` - fix: Fall back to NV12 over TCP (Mutter rejects DMA-BUF on virtio-gpu)
- `4df3a2f44` - feat: Add HELIX_MSG_ENABLE_SCANOUT protocol
- `ce98ed774` - milestone: DRM lease VERIFIED
- `346bcbdc4` - feat: helix-drm-manager daemon - DRM lease manager for container desktops
- `3b2fa865f` - feat: Add SUBSCRIBE protocol for scanout-keyed H.264 streaming
- `8ece70cae` - feat: scanout-stream-test tool for end-to-end validation
- `b6cf15441` - feat: CRTC pre-activation for DRM lease compatibility with Mutter
- `ad297fe80` - feat: Scanout video mode for macOS ARM desktop streaming (scanout_source.go)
- `fb82665e4` - fix: Use dbus-run-session for gnome-shell in mutter-lease-launcher
- `b7d8ec6e3` - feat: Integrate scanout mode into Helix container stack
- `3b2e56bfd` - feat: drm-flipper test tool, 124.6 FPS H.264 pipeline verified
- `426a59937` - fix: Coordinate scanout ID between mutter-lease-launcher and desktop-bridge
- `99036e00a` - fix: Match virtio-pci driver name for virtio-gpu in containers
- `9873537ad` - fix: Add system D-Bus setup for logind-stub in containers
- `07784f8de` - fix: Add all required session properties to logind-stub
- `e5f24fb33` - fix: Add udev card device entry with seat tag for Mutter display-server
- `cb35e0a4b` - fix: Add Q: current tags to udev card entry for GUdev compatibility

## VM Setup Notes

**SSH access** (port forwarded from guest:22 to host:2222):
```bash
ssh -p 2222 luke@localhost
# Or with ~/.ssh/config entry "helix-vm":
ssh helix-vm
```

**Helix repo in VM**: `~/helix` (branch `feature/macos-arm-desktop-port`)
```bash
ssh helix-vm "cd ~/helix && git pull"  # Update from GitHub
```

**Docker** installed at `/usr/bin/docker` (Docker 29.2.1).
Build the helix-ubuntu image from the VM:
```bash
ssh helix-vm "cd ~/helix && docker build -f Dockerfile.ubuntu-helix -t helix-ubuntu:latest ."
```

**Go 1.25** installed at `/usr/local/go/bin/go` (matches helix go.mod).

**GDM disabled** for the DRM lease approach to work:
```bash
sudo systemctl disable gdm
sudo systemctl stop gdm
```

This should be done in the VM image build (not just the dev VM). The VM display
(Virtual-1) shows the Linux console, which can be accessed via SPICE in UTM if
needed. In production, we can expose SPICE access via the Helix app settings.

## Roadmap: Production VM Image

### Current approach: Build everything from source in VM
The provisioning script (`for-mac/scripts/provision-vm.sh`) builds all dependencies
from source inside the VM, including the helix-ubuntu Docker image. This works but
is slow and requires cloning Zed, Qwen Code, and building large Rust/Go projects.

### Target approach: Official Helix install + ARM64 registry images
1. **ARM64 Drone CI agent**: Set up an ARM64 build agent (Mac Mini or Ampere cloud)
   to produce `linux/arm64` Docker images alongside the existing `linux/amd64` builds.
   Currently only the CLI binary is cross-compiled for arm64 — Docker images are amd64 only.

2. **Multi-arch Docker images**: Publish `helix-ubuntu`, `helix-sway`, `helix-sandbox`
   as multi-arch manifests (`linux/amd64` + `linux/arm64`) to the registry.

3. **Official install script**: Use the standard Helix install script
   (`curl | bash` or docker-compose) to set up the control plane + sandbox in the VM.
   The desktop images would be pulled from the registry, not built locally.

4. **macOS ARM overlay**: Layer only the VM-specific components on top:
   - `helix-drm-manager` (Go binary, systemd service) — manages DRM leases for scanout
   - ZFS 2.4.0 with dedup on a dedicated virtual disk — workspace storage efficiency
   - GDM disabled — required for DRM lease approach
   - Custom QEMU on macOS host side (not in the VM)

This would reduce VM provisioning from ~30 minutes (building everything) to ~5 minutes
(pull pre-built images + install helix-drm-manager).

## Performance History (TCP pixel approach - being replaced)

| Resolution | Format | FPS | Notes |
|-----------|--------|-----|-------|
| 1920x1080 | BGRA | 8.7 | 8MB/frame over TCP |
| 960x540 | NV12 | 43 | 777KB/frame, best TCP config |
| 1920x1080 | NV12 | 15 | 3.1MB/frame, TCP-limited |
| 640x360 | NV12 | 39 | Resolution doesn't help below 1MB |

Expected with scanout approach: **55+ FPS at full 1920x1080** (no TCP pixel transfer)
