# macOS ARM Desktop Port - Architecture Design

**Date**: 2026-02-02
**Status**: In Progress

## Overview

Port Helix desktop streaming to macOS ARM64 (Apple Silicon), replacing the current Linux-based Wolf/NVENC pipeline with a native macOS stack using VideoToolbox for hardware H.264 encoding.

## Goals

1. Run Helix desktop sessions on macOS ARM hosts
2. GPU-accelerated rendering inside VMs via virtio-gpu
3. Zero-copy video encoding path using Apple VideoToolbox
4. Maintain compatibility with existing dev container images (helix-ubuntu, helix-sway)

## Architecture

### Current Linux Architecture
```
Host (Linux + NVIDIA GPU)
└── Sandbox Container (DinD)
    └── Dev Container (helix-ubuntu)
        ├── GNOME/Sway Desktop
        ├── PipeWire ScreenCast capture
        └── pipewirezerocopysrc → NVENC H.264 → WebSocket
```

### Target macOS Architecture
```
Host (macOS ARM + Apple Silicon)
└── UTM/QEMU VM (virtio-gpu → host GPU via virglrenderer)
    └── Docker (DinD)
        └── Dev Container (helix-ubuntu)
            ├── GNOME/Sway Desktop (rendered via virtio-gpu)
            ├── PipeWire ScreenCast capture
            └── GPU frames → host → VideoToolbox H.264 → WebSocket
```

## CRITICAL: UTM Version Requirement

**We REQUIRE UTM 5.0+ (currently using 5.0.1 beta)**

UTM 5.0 ships an updated `virglrenderer` that exports the critical API we need:

```c
// This function returns native Metal texture for a virtio-gpu resource
int virgl_renderer_resource_get_info_ext(int res_handle,
                                          struct virgl_renderer_resource_info_ext *info);

// info.native_type == VIRGL_NATIVE_HANDLE_METAL_TEXTURE
// info.native_handle == MTLTexture*
```

**UTM 4.x (e.g., 4.7.5) does NOT export this function** - the code exists in the headers but wasn't compiled into the binary.

With UTM 5.0's virglrenderer:
- 54 exported symbols (vs 41 in 4.x)
- Includes `virgl_renderer_resource_get_info_ext` ✓
- Includes `virgl_metal_create_texture` ✓
- Includes `virgl_renderer_create_handle_for_scanout` ✓

## Key Decision: Use UTM

**Why UTM instead of vanilla QEMU:**
- UTM has already solved IOSurface sharing on macOS
- Uses forked QEMU with full GPU acceleration stack
- IOSurfaceID passed between processes for zero-copy rendering
- CocoaSpice handles Metal texture rendering
- Well-maintained, active development

### UTM 5.0 Graphics Stack (for Linux VMs)

UTM 5.0.1's virglrenderer supports **both OpenGL (virgl) AND Vulkan (Venus)** paths:

#### OpenGL Path (virgl)
```
Guest (Linux VM):
  OpenGL app → Mesa virgl driver → virtio-gpu (VirGL protocol)

Host (macOS):
  virglrenderer (decodes VirGL → OpenGL) → ANGLE (OpenGL → Metal) → IOSurface
```

#### Vulkan Path (Venus) - Used by Modern GNOME/mutter
```
Guest (Linux VM):
  Vulkan app (incl. mutter) → vulkan_kosmickrisp ICD → virtio-gpu (Venus protocol)

Host (macOS):
  virglrenderer/Venus (render server) → MoltenVK (Vulkan → Metal) → IOSurface
```

**IMPORTANT: Modern mutter (GNOME 42+) uses Vulkan for rendering**, not OpenGL. This means guest compositors use the Venus path.

The key insight is that **virglrenderer manages resources for BOTH paths**. Regardless of whether a surface was created via virgl (OpenGL) or Venus (Vulkan), `virgl_renderer_resource_get_info_ext()` can return the native Metal texture.

**UTM 5.0.1 includes:**
- MoltenVK.framework - Vulkan → Metal translation
- vulkan.1.framework - Vulkan loader
- vulkan_kosmickrisp.framework - Venus ICD (guest Vulkan driver)

Strings in virglrenderer confirm Venus support:
```
"VK_MESA_venus_protocol"
"failed to initialize venus renderer"
```

**Key components:**
- **VirGL**: Protocol for serializing OpenGL commands over virtio-gpu (NOT legacy, actively maintained)
- **virglrenderer**: Host library that handles BOTH virgl (OpenGL) AND Venus (Vulkan)
- **ANGLE**: Google's library that translates OpenGL → Metal (needed because macOS deprecated OpenGL)
- **Venus**: Vulkan path in virglrenderer - guest Vulkan → MoltenVK → Metal
- **MoltenVK**: Translates Vulkan API calls to Metal on macOS

UTM defaults to `virtio-gpu-gl-pci` for Linux VMs with ANGLE Metal backend + Venus for Vulkan.

### UTM's Graphics Evolution

| Version | Graphics Backend | Notes |
|---------|-----------------|-------|
| UTM 4.x | VirGL + ANGLE | `virgl_renderer_resource_get_info_ext` NOT exported |
| UTM 5.0 | VirGL + ANGLE + **Venus** | **Exports the API we need** ✓, Vulkan 1.3 via Venus |

**UTM 5.0.1 "What's New"**: *"Improved graphics acceleration for Linux: Vulkan 1.3 is now supported on Linux guests with VirtIO Venus drivers in Mesa."*

**Reference**: [UTM Graphics Architecture](https://github.com/utmapp/UTM/blob/main/Documentation/Graphics.md), [Venus + MoltenVK Issue](https://github.com/utmapp/UTM/issues/4551)

## GPU Frame Path Analysis

### Two Frame Sources

There are two different frame paths to understand:

#### 1. VM Main Display (virtio-gpu)
```
Guest rendering commands → virtio-gpu → virglrenderer → host GPU (IOSurface)
```
- With virtio-gpu-gl-pci, guest sends rendering commands
- virglrenderer on host translates to OpenGL/Metal
- Result is in host GPU memory as IOSurface
- UTM passes IOSurfaceID between QEMULauncher and CocoaSpice

#### 2. Dev Container PipeWire Captures
```
Dev container desktop → PipeWire ScreenCast → DMA-BUF → ???
```
- Desktop containers capture frames via PipeWire
- pipewirezerocopysrc gets DMA-BUF file descriptors
- These DMA-BUFs reference virtio-gpu resources
- virtio-gpu resources ARE on host GPU (via virglrenderer)

**Key Insight**: The dev container frames ARE on the host GPU - we just need a mechanism to reference them for VideoToolbox encoding.

**IMPORTANT CLARIFICATION**: We must NOT capture the VM's main display (which UTM's SPICE/CocoaSpice renders). We need to capture the frames from the **headless mutter instance** running inside each dev container. These are different surfaces:

- **VM main display**: SPICE protocol → CocoaSpice → IOSurface (what UTM displays)
- **Dev container display**: mutter → virgl → virtio-gpu resource → (also on host GPU)

The dev container's headless mutter renders via virgl driver, which goes through virtio-gpu to the host GPU. PipeWire captures mutter's output as a DMA-BUF, which references a virtio-gpu resource.

## GPU Frame Forwarding Options

### Option A: VirGL Video Encoding (VA-API → VideoToolbox)

**How it works:**
- Guest uses VA-API for video encoding
- virtio_gpu VA-API driver (Mesa) wraps frames as virtio-gpu resources
- virglrenderer receives resources, encodes with host hardware encoder

**Status:**
- VirGL video encoding exists for H.264/H.265 (added by Kylinsoft)
- Currently uses VA-API backend (Linux only)
- **No VideoToolbox backend exists** - would need to be written

**IMPORTANT**: UTM v5.0 is moving to **gfxstream**, which does NOT have video encoding support.
gfxstream is focused on graphics (OpenGL/Vulkan), not video codecs. If we use UTM v5.0+,
this path is not available unless we also run virglrenderer alongside gfxstream.

**Reference**: [Virgl Adds Accelerated Video Encoding](https://www.phoronix.com/news/Virgl-Encode-H264-H265)

### Option B: virtio-gpu UUID Resource Sharing

**How it works:**
- Guest exports virtio-gpu resource with UUID
- Pass UUID to host via vsock
- Host looks up resource, maps to IOSurface

**Status:**
- UUID support exists in virtio-gpu
- Would need custom implementation for IOSurface mapping
- Less established than Option A

**Reference**: [virtio-gpu and qemu graphics in 2021](https://www.kraxel.org/blog/2021/05/virtio-gpu-qemu-graphics-update/)

### Option C: vhost-user-gpu Protocol

**How it works:**
- DMABUF fds shared via UNIX socket between processes
- Protocol defined in QEMU documentation

**Status:**
- Designed for Linux with DMABUF
- Would need macOS adaptation for IOSurface

**Reference**: [Vhost-user-gpu Protocol](https://www.qemu.org/docs/master/interop/vhost-user-gpu.html)

### Option D: Keep Container-Side Encoding (Fallback)

**How it works:**
- Dev containers encode with software x264 inside VM
- Host just proxies already-encoded H.264 stream

**Pros:**
- Works immediately, no kernel/driver changes needed
- Compatible with existing pipewirezerocopysrc

**Cons:**
- Uses VM CPU for encoding instead of host VideoToolbox
- Slightly higher latency

## Recommended Approach

### Phase 1: UTM Integration + Software Encoding (Quick Win)

1. Embed UTM framework in for-mac Wails app
2. Use UTM's QEMU fork with virtio-gpu for GPU acceleration (rendering)
3. Keep software H.264 encoding inside containers (x264)
4. Host proxies H.264 via WebSocket (current architecture)

**Deliverable**: Working macOS desktop app with GPU-accelerated VM rendering, software video encode

### Phase 2: Zero-Copy VideoToolbox Encoding

Given that UTM v5.0 uses gfxstream (no video encoding), our options are:

**Option 2A**: Custom frame export via gfxstream hooks
- Tap into gfxstream's IOSurface output for the guest framebuffer
- When PipeWire captures, the captured buffer IS the gfxstream surface
- Export IOSurfaceID to our encoder process
- Encode with VideoToolbox

**Option 2B**: Parallel virglrenderer for video only
- Use gfxstream for graphics (rendering)
- Add virglrenderer video context for encoding only
- Guest VA-API → virglrenderer → VideoToolbox backend (needs writing)

**Option 2C**: Direct vsock frame transfer
- Guest captures with pipewirezerocopysrc
- Instead of encoding, memcpy to vsock shared memory region
- Host receives raw NV12/RGBA frames
- Host encodes with VideoToolbox
- **Con**: Not true zero-copy, but simpler than kernel modifications

### VideoToolbox Zero-Copy Path

Apple's recommended zero-copy encoding path for Apple Silicon:

```
1. CVPixelBufferPool backed by IOSurface
2. Get CVPixelBuffer from pool
3. Create Metal texture from CVPixelBuffer (same IOSurface backing)
4. Render to Metal texture
5. Send CVPixelBuffer to VideoToolbox (zero-copy via Unified Memory)
```

**Key APIs:**
- `CVPixelBufferCreateWithIOSurface()` - Create CVPixelBuffer from IOSurface
- `CVPixelBufferGetIOSurface()` - Get IOSurface from CVPixelBuffer
- `MTLDevice.makeTexture(descriptor:iosurface:plane:)` - Metal texture from IOSurface
- `VTCompressionSessionEncodeFrame()` - VideoToolbox H.264 encode

**Reference**: [WWDC21: Create image processing apps powered by Apple silicon](https://developer.apple.com/videos/play/wwdc2021/10153/)

## Implementation Files

### for-mac (Wails app)

| File | Purpose |
|------|---------|
| `app.go` | Main app struct, VM lifecycle |
| `vm.go` | VMManager - QEMU process control |
| `video.go` | VideoEncoder - encoding stats/state |
| `video_server.go` | WebSocket server for H.264 streaming |

### Dependencies

- **UTM Framework**: QEMU fork + CocoaSpice + virglrenderer
- **GStreamer**: For vtenc_h264 if using GStreamer pipeline
- **VideoToolbox**: Apple's hardware encoder framework

## Frame Export Mechanism (Guest → Host)

### Existing QEMU/SPICE Pattern

QEMU already supports exporting guest framebuffers for external encoding:

1. **virglrenderer API**: `virgl_renderer_get_fd_for_texture()` exports textures to DMA-BUF FD
2. **vhost-user-gpu protocol**: QEMU shares scanout DMABUF via FD passing (UNIX socket)
3. **SPICE uses this**: "the dmabuf is shared with Spice for encode via GStreamer"

Quote from Gerd Hoffmann: *"A simple standalone app can connect to qemu, get access to the dma-bufs via file descriptor passing and blit the dma-buf to your screen."*

### Adapting for macOS/UTM

On Linux:
```
virglrenderer → DMA-BUF fd → GStreamer → VA-API encode
```

On macOS (what we need):
```
virglrenderer (UTM) → IOSurface → our encoder → VideoToolbox
```

**UTM already does the DMA-BUF → IOSurface translation** internally (that's how CocoaSpice gets IOSurface for Metal rendering). We need to:

1. Tap into UTM's IOSurface for our video encoder process
2. Or expose vhost-user-gpu style protocol with IOSurface instead of DMA-BUF

### Concrete Implementation Path

```
Guest (dev container):
  1. PipeWire ScreenCast → captures compositor framebuffer
  2. This IS a virtio-gpu resource (already on host GPU via virglrenderer)
  3. Export resource UUID via vsock to host

Host (our VideoEncoder component):
  4. Receive UUID from guest
  5. Look up resource in virglrenderer → get IOSurface
     (virgl_renderer_get_fd_for_texture equivalent, but IOSurface on macOS)
  6. IOSurface → CVPixelBuffer (zero-copy)
  7. VideoToolbox H.264 encode
  8. Send NAL units back to guest via vsock

Guest (dev container):
  9. Receive H.264 NALs
  10. Forward to normal Helix WebSocket streaming
```

### Key UTM Integration Points

1. **Resource lookup API**: Need UTM to expose `resource_id/UUID → IOSurface` mapping
2. **IOSurfaceID sharing**: UTM passes IOSurfaceID between QEMULauncher and CocoaSpice - we tap into this
3. **vsock for control**: Guest sends resource UUIDs, host sends back encoded NALs

### Why This Works

With virtio-gpu + virglrenderer:
- Guest compositor (GNOME/mutter) renders to virtio-gpu
- virglrenderer translates to host OpenGL/Metal
- Result IS on host GPU as IOSurface (in UTM's implementation)
- PipeWire ScreenCast captures the compositor buffer = same virtio-gpu resource
- We just need the mapping from that resource to the host IOSurface

### DMA-BUF to virtio-gpu Resource ID

When PipeWire captures headless mutter's output, it returns a DMA-BUF fd. To get the virtio-gpu resource ID:

```c
// Guest-side: Get GEM handle from DMA-BUF fd
int gem_handle;
struct drm_prime_handle prime = {
    .fd = dmabuf_fd,
    .flags = 0,
};
ioctl(drm_fd, DRM_IOCTL_PRIME_FD_TO_HANDLE, &prime);
gem_handle = prime.handle;

// With virtio-gpu, the GEM handle maps to a resource ID
// The kernel driver maintains this mapping internally
```

The virtio-gpu driver in Linux kernel maintains the mapping between GEM handles and virtio-gpu resource IDs. We can expose this via a custom ioctl or by reading from the virtio-gpu driver.

**Reference**: [RFC: Export virtio-gpu resource handles via DMA-buf API](https://lore.kernel.org/lkml/20190912094121.228435-1-tfiga@chromium.org/)

### virglrenderer Native Metal Texture Support

**KEY DISCOVERY**: virglrenderer already supports returning native Metal textures!

```c
// virglrenderer.h
enum virgl_renderer_native_handle_type {
   VIRGL_NATIVE_HANDLE_NONE,
   VIRGL_NATIVE_HANDLE_D3D_TEX2D,      // D3D11 on Windows
   VIRGL_NATIVE_HANDLE_METAL_TEXTURE,  // MTLTexture on macOS!
};

struct virgl_renderer_resource_info_ext {
   struct virgl_renderer_resource_info base;
   virgl_renderer_native_handle native_handle;  // Can be MTLTexture*
   enum virgl_renderer_native_handle_type native_type;
};

// Get Metal texture for a resource
int virgl_renderer_resource_get_info_ext(int res_handle,
                                         struct virgl_renderer_resource_info_ext *info);
```

**Complete Zero-Copy Encoding Path:**
```
Guest (dev container):
  1. PipeWire captures mutter → DMA-BUF fd
  2. DRM_IOCTL_PRIME_FD_TO_HANDLE → GEM handle
  3. Get virtio-gpu resource ID from GEM handle
  4. Send resource ID to host via vsock

Host (our encoder):
  5. virgl_renderer_resource_get_info_ext(resource_id, &info)
     → info.native_type == VIRGL_NATIVE_HANDLE_METAL_TEXTURE
     → info.native_handle == MTLTexture*
  6. MTLTexture → texture.iosurface → IOSurfaceRef
  7. CVPixelBufferCreateWithIOSurface() → CVPixelBufferRef (zero-copy)
  8. VTCompressionSessionEncodeFrame() → H.264 NAL units
  9. Send NAL units back to guest via vsock

Guest:
  10. Forward H.264 to normal Helix WebSocket streaming
```

This is the optimal path - we get a native Metal texture directly from virglrenderer and encode it with VideoToolbox, all staying on the GPU with zero copies to system memory.

## pipewirezerocopysrc Integration

### Current Linux Flow (NVENC)
```
PipeWire ScreenCast → DMA-BUF fd → pipewirezerocopysrc
    → CUDA import (cuGraphicsEGLRegisterImage)
    → NVENC encode (zero-copy, same GPU memory)
    → H.264 NAL units → WebSocket
```

### Target macOS Flow (VideoToolbox)

The key insight: with virtio-gpu, the DMA-BUF inside the guest references memory that IS on the host GPU.

```
PipeWire ScreenCast → DMA-BUF fd (virtio-gpu resource)
    → Export resource UUID/handle to host
    → Host: virtio-gpu resource → IOSurface
    → IOSurface → CVPixelBuffer (zero-copy)
    → VideoToolbox encode (zero-copy via Unified Memory)
    → H.264 NAL units → WebSocket
```

### GStreamer Pipeline Architecture (RESOLVED)

**Key Decision**: ONE GStreamer pipeline in the guest, NO GStreamer on the host.

The macOS host uses direct VideoToolbox API calls. From the guest's perspective, encoding is delegated via vsock to the host, but it looks like a normal GStreamer encoder element.

```
┌─────────────────────────────────────────────────────────────────┐
│ GUEST (Linux VM)                                                │
│                                                                 │
│  GStreamer Pipeline:                                            │
│  ┌──────────────────┐   ┌──────────────┐   ┌─────────────────┐ │
│  │ pipewiresrc      │──▶│ vsockenc     │──▶│ appsink/websink │ │
│  │ (ScreenCast)     │   │ (new element)│   │                 │ │
│  └──────────────────┘   └──────┬───────┘   └─────────────────┘ │
│                                │ ▲                              │
│                     resource ID│ │H.264 NALs                    │
│                                ▼ │                              │
│                         ┌──────────────┐                        │
│                         │    vsock     │                        │
│                         └──────┬───────┘                        │
└────────────────────────────────┼────────────────────────────────┘
                                 │
                          virtio-vsock
                                 │
┌────────────────────────────────┼────────────────────────────────┐
│ HOST (macOS)                   │                                │
│                         ┌──────┴───────┐                        │
│                         │ vsock server │                        │
│                         └──────┬───────┘                        │
│                                │                                │
│                    resource ID │                                │
│                                ▼                                │
│                   ┌────────────────────────┐                    │
│                   │ virglrenderer lookup   │                    │
│                   │ resource → MTLTexture  │                    │
│                   └────────────┬───────────┘                    │
│                                │                                │
│                      MTLTexture.iosurface                       │
│                                ▼                                │
│                   ┌────────────────────────┐                    │
│                   │ VideoToolbox API       │                    │
│                   │ (direct, no GStreamer) │                    │
│                   └────────────┬───────────┘                    │
│                                │                                │
│                          H.264 NALs                             │
│                                │                                │
│                         back via vsock                          │
└─────────────────────────────────────────────────────────────────┘
```

**vsockenc GStreamer Element**:
- Looks like a normal encoder element to GStreamer
- Accepts video frames (DMA-BUF backed)
- Extracts virtio-gpu resource ID from DMA-BUF
- Sends resource ID to host via vsock
- Receives H.264 NAL units back from host
- Outputs encoded data to next element

**Host VideoToolbox Server**:
- NOT a GStreamer pipeline - direct API calls
- Receives resource IDs via vsock
- Looks up MTLTexture via virglrenderer
- Encodes with VTCompressionSession
- Sends NAL units back to guest

This architecture:
- Minimizes changes to desktop-bridge (just swap encoder element)
- Keeps WebSocket streaming in guest (existing infrastructure)
- Host is simple - just VideoToolbox API, no GStreamer dependency
- Zero-copy path maintained (GPU memory never leaves GPU)

### virtio-gpu Resource → IOSurface Mapping

With UTM's virglrenderer + ANGLE stack:

```
Guest virtio-gpu resource → virglrenderer → ANGLE → MTLTexture (backed by IOSurface)
```

**virglrenderer provides the API we need:**
```c
// Get native handle for a resource
int virgl_renderer_resource_get_info_ext(int res_handle,
                                         struct virgl_renderer_resource_info_ext *info);

// info.native_type == VIRGL_NATIVE_HANDLE_METAL_TEXTURE
// info.native_handle == MTLTexture*
// MTLTexture.iosurface → IOSurfaceRef
```

**Encoding path on host:**
1. Receive resource ID from guest via vsock
2. `virgl_renderer_resource_get_info_ext(resource_id)` → MTLTexture
3. `MTLTexture.iosurface` → IOSurfaceRef
4. `CVPixelBufferCreateWithIOSurface()` → CVPixelBufferRef (zero-copy)
5. `VTCompressionSessionEncodeFrame()` → H.264 NAL units
6. Send NAL units back to guest via vsock

## Implementation Plan

### Phase 1: Proof of Concept

1. **Build UTM from source** on macOS ARM
   - Clone UTM with all submodules
   - Build QEMU, virglrenderer, ANGLE, CocoaSpice
   - Verify Linux VM runs with virtio-gpu acceleration

2. **Test virglrenderer Metal texture export**
   - Create test program that calls `virgl_renderer_resource_get_info_ext()`
   - Verify we can get `VIRGL_NATIVE_HANDLE_METAL_TEXTURE`
   - Verify `MTLTexture.iosurface` is accessible

3. **Test VideoToolbox encoding from IOSurface**
   - Create test encoder that takes IOSurface → CVPixelBuffer → VTCompressionSession
   - Verify H.264 output is valid

### Phase 2: Guest-Host Communication

4. **Implement vsock communication**
   - Guest-side: send resource IDs when PipeWire captures frames
   - Host-side: receive resource IDs, look up Metal textures

5. **Implement resource ID extraction on guest**
   - Modify pipewirezerocopysrc or create new component
   - DMA-BUF fd → GEM handle → virtio-gpu resource ID

### Phase 3: Integration

6. **Build for-mac with UTM**
   - Embed UTM components or use as subprocess
   - Integrate VideoToolbox encoder
   - Connect vsock communication

7. **Test end-to-end**
   - Start VM with helix-ubuntu container
   - Verify frames flow: mutter → PipeWire → host → VideoToolbox → H.264 → WebSocket

### Key Files to Create/Modify

**Host (for-mac):**

| Component | Location | Purpose |
|-----------|----------|---------|
| VideoToolbox encoder | `for-mac/encoder.go` | IOSurface → H.264 via cgo ✅ |
| vsock server | `for-mac/vsock.go` | Receive resource IDs, send NALs back ✅ |
| UTM manager | `for-mac/utm.go` | Control UTM VMs via ScriptingBridge ✅ |
| Resource lookup | `for-mac/virgl.go` | Call virglrenderer API via cgo |

**Guest (vsockenc GStreamer element):**

| Component | Location | Purpose |
|-----------|----------|---------|
| vsockenc element | `desktop/gst-vsockenc/` | GStreamer encoder element delegating to host |
| vsock client | `desktop/gst-vsockenc/vsock_client.c` | Connect to host, send/receive frames |
| Resource extractor | `desktop/gst-vsockenc/resource_id.c` | DMA-BUF fd → virtio-gpu resource ID |

The vsockenc element replaces nvh264enc/x264enc in the desktop-bridge GStreamer pipeline when running on macOS.

## UTM Embedding Options (RESOLVED)

After analyzing UTM's architecture, here are the embedding options:

### UTM Architecture Summary

```
UTM.app/
├── Contents/
│   ├── MacOS/
│   │   ├── UTM               # Main app (Swift/SwiftUI)
│   │   └── utmctl            # CLI tool (Swift, uses ScriptingBridge)
│   ├── Frameworks/
│   │   ├── qemu-aarch64-softmmu.framework  # QEMU as dylib (loaded via dlopen)
│   │   ├── virglrenderer.0.framework       # virglrenderer with ANGLE
│   │   ├── glib-2.0.0.framework            # Dependencies
│   │   └── ... (many more)
│   ├── XPCServices/
│   │   └── QEMUHelper.xpc    # XPC service that forks/runs QEMU
│   └── Resources/
│       ├── qemu/             # BIOS, firmware files
│       └── CocoaSpice_CocoaSpiceRenderer.bundle
```

**Key insights:**
- QEMU is built as a **dylib** (not executable), loaded via `dlopen()`
- `QEMULauncher` calls `dlsym(dylibPath, "qemu_init")`, `qemu_main_loop()`, `qemu_cleanup()`
- `utmctl` uses **ScriptingBridge** (AppleScript API) to control UTM
- UTM uses XPC for process isolation (QEMUHelper runs QEMU in separate process)

### Option 1: ScriptingBridge Control (Phase 1)

**Approach**: Ship UTM.app with for-mac, control via ScriptingBridge/utmctl

```go
// for-mac/utm_darwin.go
// #cgo LDFLAGS: -framework Foundation -framework AppKit
// #import <Foundation/Foundation.h>
// Use NSAppleScript or ScriptingBridge to control UTM
```

**Pros:**
- Simplest integration, works immediately
- Leverages UTM's mature VM management
- Can use `utmctl exec` for guest commands
- Can use `utmctl ip-address` for networking

**Cons:**
- Requires UTM GUI app running (can use `--hide` flag)
- Cannot access IOSurface directly (no zero-copy encoding)
- Software encoding in guest only

**Use for Phase 1** - Get a working macOS app quickly

### Option 2: Extract QEMU + Run Directly

**Approach**: Extract QEMU dylibs from UTM, load directly in our process

```go
// for-mac/qemu_darwin.go
// Use cgo to dlopen QEMU framework and call qemu_init/qemu_main_loop

// #cgo LDFLAGS: -framework Foundation
// void* loadQemu(const char* path);
// int startQemu(void* ctx, int argc, char** argv);
```

**Pros:**
- Don't need UTM app running
- Direct control over QEMU arguments
- Smaller footprint (don't ship full UTM UI)

**Cons:**
- Still can't access virglrenderer internals for IOSurface
- Need to handle QEMU process lifecycle ourselves
- More complex than Option 1

### Option 3: Fork virglrenderer for IOSurface Export (Phase 2)

**Approach**: Fork UTM's virglrenderer to expose resource → IOSurface mapping

```c
// Our addition to virglrenderer
IOSurfaceRef virgl_get_iosurface_for_resource(int res_id);
```

**Implementation:**
1. virglrenderer already creates MTLTexture backed by IOSurface (for ANGLE)
2. Add API to look up IOSurface by virtio-gpu resource ID
3. Guest sends resource ID via vsock
4. Host looks up IOSurface, encodes with VideoToolbox

**Pros:**
- **Zero-copy encoding path** - GPU memory stays on GPU
- Full control over encoding pipeline
- Can optimize latency

**Cons:**
- Need to maintain virglrenderer fork
- Complex integration
- Need to understand virglrenderer internals

**Use for Phase 2** - Zero-copy VideoToolbox encoding

### Recommended Phased Approach

**Phase 1: Working App (2-3 weeks)**
- Use Option 1 (ScriptingBridge + bundled UTM)
- Software x264 encoding in guest (existing pipewirezerocopysrc fallback)
- Validates VM works, networking, Docker, dev containers

**Phase 2: Zero-Copy Encoding (2-3 weeks)**
- Fork virglrenderer, add IOSurface export API
- Build VideoToolbox encoder in for-mac
- Implement vsock protocol for frame pointer exchange
- Replace guest-side encoding with host-side VideoToolbox

### for-mac + UTM Integration (Phase 1)

```go
// app.go - additions for macOS
type App struct {
    ctx         context.Context
    vm          *VMManager
    encoder     *VideoEncoder
    videoServer *VideoServer
    utm         *UTMManager  // NEW: UTM integration
}

// utm_darwin.go
type UTMManager struct {
    vmName    string
    vmRunning bool
}

func (u *UTMManager) Start() error {
    // Use osascript or ScriptingBridge to start VM
    cmd := exec.Command("osascript", "-e",
        fmt.Sprintf(`tell application "UTM" to start virtual machine "%s"`, u.vmName))
    return cmd.Run()
}

func (u *UTMManager) GetIP() (string, error) {
    // Use utmctl to get guest IP
    cmd := exec.Command("/Applications/UTM.app/Contents/MacOS/utmctl",
        "ip-address", u.vmName)
    output, err := cmd.Output()
    return strings.TrimSpace(string(output)), err
}
```

## Open Questions

1. ~~**UTM embedding**: Can UTM be used as a framework, or do we need to fork/extract components?~~ **RESOLVED** - See UTM Embedding Options above
2. **virglrenderer VideoToolbox**: Is anyone working on this? Worth contributing upstream?
3. **Multiple displays**: How to handle multiple dev container displays simultaneously?
4. **Resource isolation**: How does virtio-gpu handle multiple Docker containers' displays?

## Implementation Status

### Host Components (for-mac/) ✅ COMPLETE

| File | Status | Description |
|------|--------|-------------|
| `encoder.go` | ✅ Complete | VideoToolbox H.264 encoder with cgo, callback registry |
| `vsock.go` | ✅ Complete | vsock server for frame requests from guest |
| `virgl.go` | ✅ Complete | virglrenderer lookup interface (resource ID → IOSurface) |
| `utm.go` | ✅ Complete | UTM VM control via utmctl/ScriptingBridge |
| `app.go` | ✅ Complete | Main app with VsockServer + VideoToolboxEncoder integration |
| `video.go` | ✅ Complete | Video encoding stats/state |
| `websocket.go` | ✅ Complete | WebSocket server for browser streaming |
| `vm.go` | ✅ Complete | VM manager interface |

### Guest Components (desktop/gst-vsockenc/) ✅ COMPLETE

| File | Status | Description |
|------|--------|-------------|
| `gstvsockenc.h` | ✅ Complete | Header with protocol definitions |
| `gstvsockenc.c` | ✅ Complete | GStreamer encoder element (vsockenc) |
| `meson.build` | ✅ Complete | Meson build configuration |

### Data Flow Summary

```
Guest (Linux VM / dev container):
1. mutter renders desktop (headless)
2. PipeWire ScreenCast captures compositor output → DMA-BUF fd
3. pipewiresrc → vsockenc GStreamer element
4. vsockenc extracts resource ID: DMA-BUF fd → GEM handle → virtio-gpu resource ID
5. vsockenc sends FrameRequest(resource_id, width, height, pts) over vsock
6. vsockenc waits for FrameResponse(pts, is_keyframe, nal_data)
7. vsockenc outputs H.264 NAL units to GStreamer pipeline
8. WebSocket sink streams to browser

Host (macOS):
1. VsockServer receives FrameRequest
2. ResourceToIOSurfaceID() converts resource_id → IOSurface ID via virglrenderer
3. VideoToolboxEncoder.EncodeIOSurface() encodes frame (zero-copy)
4. encoderOutputCallback() receives H.264 NAL units
5. VsockServer.SendEncodedFrame() sends FrameResponse back to guest
```

### Build Instructions

**Host (macOS ARM):**
```bash
cd helix/for-mac
go build -v .      # Requires macOS with Wails dependencies
```

**Guest (Linux, inside dev container):**
```bash
cd helix/desktop/gst-vsockenc
meson setup build
ninja -C build
# Install plugin to GStreamer plugin path
cp build/libgstvsockenc.so ~/.local/lib/gstreamer-1.0/
```

### Testing End-to-End

1. **Start UTM with Linux VM** (virtio-gpu acceleration enabled)
2. **Launch for-mac Wails app** (starts VsockServer on /tmp/helix-vsock.sock)
3. **Inside VM, start dev container** with helix-ubuntu image
4. **Inside dev container, run GStreamer pipeline:**
   ```bash
   gst-launch-1.0 pipewiresrc ! videoconvert ! vsockenc socket-path=/tmp/helix-vsock.sock ! appsink
   ```
5. **Connect browser** to WebSocket endpoint for H.264 stream

### Core Technical Challenge: QEMU-Side Handler

**The fundamental problem**: `virgl_renderer_resource_get_info_ext()` must be called from within QEMU's process, where the virglrenderer context exists. Our host-side encoder cannot call this function directly - the virglrenderer context is internal to QEMU.

**Proposed solution**: Fork UTM's QEMU and add a vsock handler that:
1. Listens on a specific vsock port (e.g., 5000)
2. Receives resource ID requests from guest
3. Calls `virgl_renderer_resource_get_info_ext(resource_id, &info_ext)`
4. Gets Metal texture from `info_ext.native_handle` (when `native_type == VIRGL_NATIVE_HANDLE_METAL_TEXTURE`)
5. Gets IOSurface from `MTLTexture.iosurface`
6. Encodes with VideoToolbox
7. Sends H.264 NAL units back over vsock

**QEMU files to modify:**
- `hw/display/virtio-gpu-virgl.c` - Main virtio-gpu virglrenderer code
- `include/hw/virtio/virtio-gpu.h` - Add frame export state
- New file: `hw/display/virtio-gpu-helix-export.c` - vsock frame export handler

This is the "correct architecture" that keeps frame data on the GPU throughout the pipeline.

### Known Limitations

1. **QEMU modification required** - The zero-copy path requires modifying QEMU to add the vsock frame export handler. This is non-trivial but necessary to avoid copying pixels between GPU and CPU memory.

2. **virglrenderer context dependency** - The `virgl_renderer_resource_get_info_ext` function only works within the QEMU process where virglrenderer is initialized. External processes cannot call this API.

3. **vsock path configuration** - The vsock socket path needs to be configured in UTM's QEMU settings and match the path in the guest's vsockenc element.

4. **Multiple containers** - Currently designed for single dev container. Multiple containers would need session multiplexing over vsock.

## Fallback Options (If QEMU Fork Proves Impractical)

**Decision**: Try zero-copy QEMU fork first. If that proves too complex or unmaintainable, fall back to these alternatives in order of preference.

### Fallback 1: ivshmem Shared Memory (One Copy)

**Approach**: Use QEMU's ivshmem (inter-VM shared memory) device to share memory between guest and host without modifying QEMU.

```
Guest: PipeWire DMA-BUF → copy to ivshmem (shared RAM)
Host: mmap ivshmem → IOSurface → VideoToolbox (zero-copy on host)
```

**Setup**:
```bash
# Host: create shared memory file (256MB ring buffer)
# UTM additional QEMU args:
-device ivshmem-plain,memdev=framebuf \
-object memory-backend-file,id=framebuf,share=on,mem-path=/tmp/helix-frames,size=256M
```

**Guest side**:
- Load ivshmem kernel driver
- mmap the shared memory region
- PipeWire captures DMA-BUF → glReadPixels/vaMapBuffer to ivshmem
- Signal host via vsock (just metadata: offset, size, timestamp, format)

**Host side**:
- mmap same file as guest
- Create IOSurface backed by this memory (if possible) or memcpy to IOSurface
- Encode with VideoToolbox
- Send H.264 NALs back via vsock

**Copies**: 1 (GPU→shared RAM in guest). Host side can potentially be zero-copy if IOSurface can back the shared memory.

**Bandwidth**: ~180 MB/s for 1080p60 YUV420. Well within DDR bandwidth.

**Pros**:
- No QEMU fork required
- UTM supports custom QEMU arguments
- vsock only carries tiny signaling messages

**Cons**:
- One GPU→CPU copy in guest
- More complex guest daemon needed
- Ring buffer synchronization

### Fallback 2: Guest Software Encoding (Simplest)

**Approach**: Encode H.264 in the guest using software (x264), send compressed stream over vsock.

```
Guest: PipeWire → GStreamer → x264enc → vsock → Host
Host: Receive H.264, forward to WebSocket (no encoding needed)
```

**This is exactly what we do on Linux**, just with x264 instead of nvh264enc.

**Implementation**:
- Reuse existing pipewirezerocopysrc with x264enc fallback
- No changes to desktop-bridge architecture
- vsock carries ~2-5 MB/s H.264 (vs 180 MB/s raw)

**CPU Cost**: ~10-15% of one VM core for 1080p60 at "veryfast" preset. Apple Silicon VMs are fast (HVF acceleration), so this is acceptable.

**Pros**:
- No QEMU modifications
- All existing code works
- Minimal bandwidth over vsock
- Proven reliable (this is our Linux fallback path)

**Cons**:
- Uses VM CPU for encoding
- Slightly higher latency than hardware encode
- Not "zero-copy"

### Decision Matrix

| Approach | Copies | QEMU Fork? | Complexity | Performance |
|----------|--------|------------|------------|-------------|
| QEMU virglrenderer export | 0 | Yes | High | Best |
| ivshmem shared memory | 1 | No | Medium | Good |
| Guest x264 encoding | 2+ | No | Low | Acceptable |

**Decision**: Start with QEMU fork (zero-copy). If we can't get zero-copy working (e.g., virglrenderer doesn't expose Metal textures as expected, or IOSurface isn't accessible), fall back to ivshmem shared memory (one copy). Guest software encoding is a last resort only - it likely won't achieve the "native feel" we're targeting.

## QEMU Frame Export Implementation Plan

### Overview

The QEMU modification adds a vsock-based frame export mechanism that:
1. Receives virtio-gpu resource IDs from the guest
2. Looks up the corresponding Metal texture via virglrenderer
3. Encodes frames using VideoToolbox
4. Sends H.264 NAL units back to the guest

### Implementation Files

**New files to create:**

```
qemu-utm/
├── hw/display/
│   ├── helix-frame-export.c     # Main frame export implementation
│   └── helix-frame-export.h     # Header with protocol definitions
├── include/hw/virtio/
│   └── helix-frame-export.h     # Public API
└── contrib/helix/
    └── meson.build              # Build configuration
```

### Protocol Definition (helix-frame-export.h)

```c
#ifndef HELIX_FRAME_EXPORT_H
#define HELIX_FRAME_EXPORT_H

#include <stdint.h>

// Message types
#define HELIX_MSG_FRAME_REQUEST   1  // Guest -> Host: encode this resource
#define HELIX_MSG_FRAME_RESPONSE  2  // Host -> Guest: encoded NAL data
#define HELIX_MSG_KEYFRAME_REQ    3  // Guest -> Host: request keyframe
#define HELIX_MSG_PING            4  // Keepalive
#define HELIX_MSG_PONG            5  // Keepalive response

// Frame request structure
typedef struct HelixFrameRequest {
    uint32_t resource_id;   // virtio-gpu resource ID
    uint32_t width;
    uint32_t height;
    uint32_t format;        // Pixel format (BGRA, NV12, etc.)
    int64_t pts;            // Presentation timestamp (ns)
    int64_t duration;       // Frame duration (ns)
} __attribute__((packed)) HelixFrameRequest;

// Frame response structure
typedef struct HelixFrameResponse {
    int64_t pts;
    uint8_t is_keyframe;
    uint32_t nal_size;
    // NAL data follows
} __attribute__((packed)) HelixFrameResponse;

#endif // HELIX_FRAME_EXPORT_H
```

### Core Implementation (helix-frame-export.c)

```c
// Key functions to implement:

// 1. Initialize the frame export subsystem
int helix_frame_export_init(VirtIOGPU *g, int vsock_port);

// 2. Handle incoming frame request
static void handle_frame_request(HelixFrameExport *fe, HelixFrameRequest *req) {
    struct virgl_renderer_resource_info_ext info_ext;

    // Look up the Metal texture for this resource
    int ret = virgl_renderer_resource_get_info_ext(req->resource_id, &info_ext);
    if (ret != 0) {
        error_report("Failed to get resource info for %d", req->resource_id);
        return;
    }

    if (info_ext.native_type != VIRGL_NATIVE_HANDLE_METAL_TEXTURE) {
        error_report("Resource %d is not a Metal texture", req->resource_id);
        return;
    }

    // Get IOSurface from Metal texture
    // info_ext.native_handle is MTLTexture*
    // MTLTexture.iosurface gives us the IOSurface

    // Encode with VideoToolbox
    helix_encode_iosurface(fe->encoder, info_ext.native_handle,
                          req->pts, req->duration);
}

// 3. Send encoded frame back to guest
static void encoder_callback(void *ctx, CMSampleBufferRef sample) {
    HelixFrameExport *fe = ctx;
    // Extract NAL units and send via vsock
    helix_send_frame_response(fe, sample);
}
```

### Integration with virtio-gpu-virgl.c

Add initialization call in `virtio_gpu_virgl_init()`:

```c
int virtio_gpu_virgl_init(VirtIOGPU *g)
{
    // ... existing code ...

    // Initialize Helix frame export if enabled
    if (g->conf.helix_frame_export) {
        ret = helix_frame_export_init(g, g->conf.helix_vsock_port);
        if (ret != 0) {
            error_report("Failed to initialize Helix frame export: %d", ret);
            // Non-fatal, continue without frame export
        }
    }

    return 0;
}
```

### Build Configuration

Add to `hw/display/meson.build`:

```meson
if host_machine.system() == 'darwin'
  softmmu_ss.add(files('helix-frame-export.c'))
  softmmu_ss.add(dependency('videotoolbox'))
  softmmu_ss.add(dependency('corevideo'))
  softmmu_ss.add(dependency('coremedia'))
endif
```

### QEMU Command Line Options

Add new options for frame export:

```
-device virtio-gpu-gl-pci,helix-frame-export=on,helix-vsock-port=5000
```

### Testing Strategy

1. **Unit test virglrenderer lookup**: Verify `virgl_renderer_resource_get_info_ext` returns valid Metal texture
2. **Unit test VideoToolbox encoding**: Encode test IOSurface, verify H.264 output
3. **Integration test**: Full guest→host→guest round trip with test resource
4. **Performance test**: Measure frame latency and encoding throughput

## References

- [UTM GitHub](https://github.com/utmapp/UTM)
- [UTM Graphics Documentation](https://github.com/utmapp/UTM/blob/main/Documentation/Graphics.md)
- [CocoaSpice](https://github.com/utmapp/CocoaSpice)
- [virglrenderer](https://gitlab.freedesktop.org/virgl/virglrenderer)
- [QEMU virtio-gpu docs](https://qemu.readthedocs.io/en/latest/system/devices/virtio-gpu.html)
- [Apple VideoToolbox](https://developer.apple.com/documentation/videotoolbox)
- [WWDC21: Low-latency video encoding](https://developer.apple.com/videos/play/wwdc2021/10158/)
- [Collabora: State of GFX virtualization](https://www.collabora.com/news-and-blog/blog/2025/01/15/the-state-of-gfx-virtualization-using-virglrenderer/)

## Progress Log

### 2026-02-02: QEMU Frame Export Implementation

**Completed:**
- ✅ Created Ubuntu 25.10 ARM64 VM in UTM with Venus enabled
- ✅ Verified Venus/Vulkan working: `vulkaninfo` shows "Virtio-GPU Venus (Apple M1 Pro)"
- ✅ Created QEMU fork branch `helix-frame-export` in `qemu-utm/`
- ✅ Implemented frame export files:
  - `qemu-utm/hw/display/helix/helix-frame-export.h` - Protocol definitions
  - `qemu-utm/hw/display/helix/helix-frame-export.c` - VideoToolbox encoder + virglrenderer integration
  - `qemu-utm/hw/display/helix/meson.build` - Build configuration
- ✅ Updated `qemu-utm/hw/display/meson.build` to include helix subdir
- ✅ Committed to `helix-frame-export` branch in qemu-utm

**Next steps:**
1. Create GitHub fork: Need to create `helixml/qemu` (or similar) to push the QEMU changes
2. Build modified QEMU: Set up full UTM build environment to compile our fork
3. Integration test: Load VM with modified QEMU, test virgl_renderer_resource_get_info_ext
4. End-to-end test: Guest vsockenc → host frame export → H.264 back to guest

**Files changed in helix repo:**
- `.gitignore` - Removed qemu-utm/ from ignore list
- `design/2026-02-02-macos-arm-desktop-port.md` - This file

**QEMU fork location:**
- Local: `/Users/luke/pm/helix/qemu-utm/` (branch: `helix-frame-export`)
- Remote: TBD - need to create helixml/qemu fork on GitHub
