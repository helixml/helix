# Windows Desktop Port: GPU Acceleration & Video Encoding Architecture

**Date:** 2026-02-11
**Branch:** `feature/windows-desktop-port`
**Status:** Code complete (cross-platform refactor + WSL2 VM manager), not yet tested on Windows hardware

## Overview

The Windows port replaces QEMU with WSL2. GPU acceleration and hardware video encoding use Microsoft's GPU-PV (GPU Paravirtualization) and a D3D12-backed VAAPI driver, respectively.

## Architecture: WSL2 with GPU-PV

### Why WSL2 Instead of QEMU

QEMU on Windows lacks a good hardware GPU passthrough story — there's no equivalent to macOS Virtualization.framework's virtio-gpu/Venus integration. WSL2 provides:

- Hardware GPU acceleration (OpenGL/Vulkan) via GPU-PV, working with NVIDIA, AMD, and Intel GPUs using their native Windows drivers
- Hardware H.264 encoding via VAAPI over D3D12
- No GPU passthrough configuration needed — the host keeps full GPU access simultaneously
- Simpler distribution (rootfs tarball instead of qcow2 disk image)

### GPU-PV (GPU Paravirtualization)

Microsoft's GPU-PV shares the host GPU with WSL2 at the driver level:

```
WSL2 Linux kernel
    │
    ├── /dev/dxg  (GPU-PV device, exposed by dxgkrnl kernel module)
    │
    ├── Mesa D3D12 backend (libgallium_dri.so)
    │       │
    │       ├── OpenGL  →  translated to D3D12 calls  →  GPU-PV  →  Host GPU
    │       └── Vulkan (dzn driver)  →  D3D12  →  GPU-PV  →  Host GPU
    │
    └── /usr/lib/wsl/lib/  (GPU driver libraries injected by Microsoft)
```

Key points:
- The host's GPU driver (NVIDIA/AMD/Intel Windows driver) does the actual rendering
- WSL2's `dxgkrnl` module translates GPU operations across the VM boundary
- Mesa's D3D12 backend provides OpenGL/Vulkan on top of D3D12
- No special GPU configuration needed — works out of the box on Windows 11 and Windows 10 21H2+

### Hardware H.264 Encoding: VAAPI over D3D12

Microsoft provides a `libva` driver that translates VAAPI calls to D3D12 video encode operations:

```
GStreamer vaapih264enc
    │
    ▼
libva (VAAPI)
    │  LIBVA_DRIVER_NAME=d3d12
    ▼
libva-d3d12-driver
    │
    ▼
D3D12 ID3D12VideoEncodeCommandList
    │
    ▼
GPU-PV  →  Host GPU hardware encoder
              ├── NVENC (NVIDIA)
              ├── VCE/VCN (AMD)
              └── QSV (Intel)
```

This uses the same `vaapih264enc` GStreamer element as a native Linux setup, but the backend is D3D12 instead of a native VA driver.

## Full Pipeline

```
┌─────────────────────── WSL2 (Helix distro) ───────────────────────┐
│                                                                    │
│  Docker Container (helix-ubuntu)                                   │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  GNOME Desktop (headless Wayland compositor)                  │  │
│  │       │ renders via OpenGL/Vulkan                             │  │
│  │       ▼                                                      │  │
│  │  Mesa D3D12 backend ──► /dev/dxg (GPU-PV) ──► Host GPU      │  │
│  │       │                                                      │  │
│  │  PipeWire ScreenCast (captures frames via D-Bus portal)      │  │
│  │       │                                                      │  │
│  │  GStreamer: pipewiresrc → vaapih264enc                        │  │
│  │       │         (LIBVA_DRIVER_NAME=d3d12)                    │  │
│  │       │              │                                       │  │
│  │       │         D3D12 Video Encode ──► GPU-PV ──► Host GPU   │  │
│  │       ▼                    (hardware H.264 encoder)          │  │
│  │  desktop-bridge (WebSocket H.264 stream)                     │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
         │ WebSocket
         ▼
  Wails WebView2 (frontend) ── decodes H.264 via MediaSource API
```

## Docker Compose Configuration

The compose file deployed inside the WSL2 distro must expose GPU-PV to the container:

```yaml
services:
  helix-ubuntu:
    devices:
      - /dev/dxg:/dev/dxg
    environment:
      - LIBVA_DRIVER_NAME=d3d12
    volumes:
      - /usr/lib/wsl:/usr/lib/wsl  # WSL2 GPU driver libraries
```

The `/usr/lib/wsl/lib` directory contains GPU driver libraries that Microsoft injects into WSL2 — these must be bind-mounted into Docker containers for GPU-PV to work.

## Platform Comparison

| Aspect | macOS (QEMU) | Windows (WSL2) | Linux (native) |
|--------|-------------|----------------|----------------|
| Virtualization | QEMU + Virtualization.framework | WSL2 (Microsoft lightweight VM) | None (Docker native) |
| GPU access | Virtio-GPU / Venus (Vulkan) | GPU-PV (D3D12 paravirtualization) | Direct GPU access |
| Video encode | Custom zerocopy GStreamer plugin | `vaapih264enc` with d3d12 driver | `vaapih264enc` (native) |
| GPU vendors | Apple Silicon only | NVIDIA, AMD, Intel | NVIDIA, AMD, Intel |
| Disk image | qcow2 (~8GB) | rootfs tarball (~1GB) | N/A |
| `runInVM()` | SSH to QEMU VM | `wsl.exe -d Helix -- bash -c` | `bash -c` directly |

## Code Structure (Implemented)

Cross-platform split using Go build tags:

```
for-mac/
├── vm.go                  # Shared: types, struct, cross-platform methods
├── vm_darwin.go           # macOS: QEMU, SSH, QMP, ZFS, SPICE
├── vm_windows.go          # Windows: WSL2 import/manage, no QEMU
├── vm_linux.go            # Linux: native Docker, no VM
├── platform_darwin.go     # macOS: data dir, memory, machine ID, NIC
├── platform_windows.go    # Windows: data dir, memory, machine ID
├── platform_linux.go      # Linux: data dir, memory, machine ID
├── tray_darwin.go         # macOS: CGo GCD dispatch for systray
├── tray_windows.go        # Windows: direct call (no-op)
├── tray_linux.go          # Linux: direct call (no-op)
├── scripts/
│   ├── build-wsl-rootfs.sh    # Builds Ubuntu 24.04 rootfs for WSL2
│   └── build-windows.md       # Windows build instructions
└── vm-manifest-windows.json   # Download manifest for rootfs tarball
```

Key abstraction: `runInVM(script string) *exec.Cmd` — each platform implements this differently but shared methods (startHelixStack, diagnoseAPIFailure, etc.) call it uniformly.

## Requirements

- Windows 11 or Windows 10 21H2+ (for GPU-PV in WSL2)
- WSL2 enabled with a compatible GPU driver
- `mesa-va-drivers` package inside the rootfs (Ubuntu 24.04 ships this)

## Open Items

1. **Build and test rootfs tarball** — run `scripts/build-wsl-rootfs.sh`, verify Docker + GPU-PV work inside the imported distro
2. **Upload rootfs to CDN** — update `vm-manifest-windows.json` with size/SHA256
3. **Build .exe on Windows** — Wails requires Windows-native GCC for CGo/WebView2 bindings; can't cross-compile from macOS
4. **Test full pipeline** — verify GPU-PV OpenGL rendering, VAAPI H.264 encoding, and WebSocket streaming work end-to-end
5. **Docker-in-WSL2 GPU access** — confirm `/dev/dxg` is accessible inside Docker containers within WSL2 (may need `--privileged` or specific device cgroup rules)
6. **Headless Wayland in WSL2** — verify that a headless Wayland compositor (weston or mutter) works inside Docker-in-WSL2 with GPU-PV for PipeWire ScreenCast
