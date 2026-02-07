# Zero-Copy GPU Video Capture Architecture

**Date:** 2026-02-07
**Status:** In progress - core components verified, wiring up end-to-end

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ macOS Host                                                   │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ QEMU (virtio-gpu-gl-pci, max_outputs=16)            │    │
│  │                                                      │    │
│  │  helix-frame-export.m (v6-multi-scanout)             │    │
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
| Mutter → logind-stub D-Bus | ⬜ NOT TESTED | logind-stub registers OK, but TakeDevice not tested with Mutter |
| QEMU auto-encode on page flip | ⬜ NOT TESTED | Code written, needs Mutter rendering to a connector |
| H.264 → WebSocket end-to-end | ⬜ NOT TESTED | Needs all above working |

### Open Concerns

1. **Container D-Bus system bus**: Each container needs its own D-Bus system bus for
   logind-stub. Start `dbus-daemon --system --config-file=...` inside the container.
   This is separate from the session bus (already working for Mutter D-Bus services).

2. **Container /dev/dri access**: The container needs /dev/dri/card0 bind-mounted for
   Mutter to discover DRM devices via udev. The lease FD controls access permissions.

3. **FD inheritance**: The startup script gets the lease FD from helix-drm-manager and
   must pass it to logind-stub. The FD must stay open (not be garbage-collected).
   Solution: startup script gets FD, execs logind-stub which inherits it.

4. **Mutter connector selection**: When Mutter gets the lease FD from logind-stub via
   TakeDevice, it sees only the leased connector+CRTC. It should automatically use
   that connector. Need to verify Mutter doesn't reject single-connector leases.

5. **/run/helix-drm.sock mount**: The Unix socket must be bind-mounted into each
   container. Add to Docker run: `-v /run/helix-drm.sock:/run/helix-drm.sock`.

## Key Benefits

- **Zero CPU copies**: Mutter renders to GPU -> QEMU reads Metal IOSurface -> VideoToolbox encodes
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

**Integration points:**
- `ws_stream.go`: Add `VideoModeScanout` alongside existing modes
- `scanout_stream.go` (new): Implements scanout TCP receiver
- Container startup: When scanout mode, skip PipeWire ScreenCast setup
- PipeWire still needed for audio even in scanout mode

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

## VM Setup Notes

**SSH access** (port forwarded from guest:22 to host:2222):
```bash
ssh -p 2222 luke@localhost
# Or with ~/.ssh/config entry "helix-vm":
ssh helix-vm
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

## Performance History (TCP pixel approach - being replaced)

| Resolution | Format | FPS | Notes |
|-----------|--------|-----|-------|
| 1920x1080 | BGRA | 8.7 | 8MB/frame over TCP |
| 960x540 | NV12 | 43 | 777KB/frame, best TCP config |
| 1920x1080 | NV12 | 15 | 3.1MB/frame, TCP-limited |
| 640x360 | NV12 | 39 | Resolution doesn't help below 1MB |

Expected with scanout approach: **55+ FPS at full 1920x1080** (no TCP pixel transfer)
