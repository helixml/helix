# macOS ARM Desktop Port - Architecture Design

**Date**: 2026-02-02
**Last Updated**: 2026-02-04 07:25 UTC
**Status**: Phase 2 Complete - Modified QEMU Built and Patched into UTM.app

## Executive Summary

Port Helix desktop streaming to macOS ARM64 (Apple Silicon). Use UTM/QEMU VM with virtio-gpu for GPU acceleration, delegate video encoding to host VideoToolbox for zero-copy H.264 encoding.

**Distribution**: Docker Desktop-style app - single `.app` with bundled VM, no manual configuration.

**Key Architecture Decisions:**
1. **VM Platform**: UTM 5.0+ (provides virglrenderer with Metal texture export API)
2. **GPU Acceleration**: virtio-gpu with Venus/Vulkan + virgl OpenGL support
3. **Video Encoding**: VideoToolbox on host (via vsockenc GStreamer element)
4. **Storage**: overlay2 driver for nested Docker (works perfectly with DinD on ARM64)

**Status:**
- ✅ VM running Ubuntu 25.10 ARM64 with Venus/Vulkan GPU acceleration
- ✅ helix-ubuntu desktop image built (ARM64-native)
- ✅ helix-sandbox container running with overlay2 storage driver (fast image transfers)
- ✅ Test session created and running (ses_01kgjkc6qcxs3qf568xbt4p3yv)
- ✅ **PipeWire ScreenCast working with DmaBuf enabled**
- ✅ **Video streaming functional** (H.264 via WebSocket, x264enc software encoding)
- ✅ **vsockenc GStreamer element implemented** (C, with DmaBuf→virtio-gpu resource ID extraction)
- ✅ **vsockenc integrated into helix-ubuntu ARM64 build** (meson, installs to /usr/lib/gstreamer-1.0/)
- ✅ **QEMU integration module discovered** - helix-frame-export already complete in for-mac/qemu-helix/
- ✅ Fixed vsockenc meson build (added required PACKAGE defines)
- ✅ **helix-frame-export integrated into UTM's QEMU fork** (~/pm/qemu-utm)
  - Copied for-mac/qemu-helix/ to hw/display/helix/
  - Modified hw/display/meson.build to build helix module on macOS
  - Modified hw/display/virtio-gpu-virgl.c to init frame export
  - Committed to utm-edition branch: 4237f5099b
- ✅ **vsockenc build successful** - helix-ubuntu:7c082c with libgstvsockenc.so installed
- ✅ **code-macos sandbox profile added** - GPU_VENDOR=virtio for virtio-gpu
- ✅ **desktop-bridge updated** - selectEncoder() prioritizes vsockenc, pipeline configured
- ✅ **VM disk expanded** - Resized from 256GB (100% full) to 512GB, 418GB free
- ✅ **VM CPU count increased** - Changed from 4 cores to 20 cores for faster builds
- ✅ **helix-ubuntu:7c082c pulled into sandbox** - Image available for testing
- ✅ **Modified QEMU built successfully** - Built from ~/pm/qemu-utm with helix-frame-export
  - Fixed virglrenderer dependency issues (manual paths in meson.build)
  - Fixed virtio-gpu-virgl.c API compatibility (added version guards)
  - Binary: libqemu-aarch64-softmmu.dylib (29MB) with helix symbols verified
  - Pushed to GitHub: https://github.com/helixml/qemu-utm (utm-edition branch)
- ✅ **UTM.app patched with modified QEMU** - Replaced binary and re-signed
  - Location: ~/pm/helix/UTM/build/Build/Products/Release/UTM.app
  - Ad-hoc signed for local testing
- ⏳ Next: Test end-to-end zero-copy pipeline with vsockenc → helix-frame-export → VideoToolbox

**Remaining Work:**
1. ~~Integrate vsockenc into helix-ubuntu desktop image build~~ ✅ Done (helix-ubuntu:169abe)
2. ~~Implement QEMU vsock handler to access virglrenderer~~ ✅ Done (helix-frame-export complete)
3. ~~Integrate helix-frame-export into UTM's QEMU fork~~ ✅ Done (committed 4237f5099b)
4. ~~Build modified QEMU and integrate into UTM.app~~ ✅ Done
   - ✅ Built QEMU from ~/pm/qemu-utm with helix-frame-export module
   - ✅ Fixed build issues (virglrenderer deps, API compatibility)
   - ✅ Replaced QEMU binary in UTM.app and re-signed (ad-hoc)
   - ✅ Verified helix-frame-export symbols in binary
5. ~~Complete helix-ubuntu build and test vsockenc~~ ✅ Done (helix-ubuntu:169abe ready)
6. ~~Add code-macos sandbox profile~~ ✅ Done (GPU_VENDOR=virtio detection)
7. ~~Update desktop-bridge to use vsockenc encoder~~ ✅ Done (ws_stream.go updated)
8. **Test zero-copy encoding path end-to-end:**
   - Configure vsock device in QEMU for guest→host communication
   - Start helix session with helix-ubuntu:169abe image
   - Verify helix-frame-export initialization in logs
   - Test video streaming with `helix spectask stream`
   - Measure performance vs software x264enc encoding

## Deployment Strategy

**Production Deployment Approach:**

Instead of building UTM from source, we can **patch and re-sign production UTM releases**:

1. **Download official UTM.app** (5.0+ required for virglrenderer API)
2. **Build only our modified QEMU** with helix-frame-export module
3. **Replace QEMU binary** in UTM.app bundle:
   ```bash
   # Copy built QEMU into existing UTM.app
   cp qemu-system-aarch64 \
      UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu
   ```
4. **Re-sign bundle** (ad-hoc signing for local use):
   ```bash
   codesign --force --deep --sign - UTM.app
   ```
5. **Distribute patched UTM.app** with Helix

**Benefits:**
- ⚡ **Much faster** - Building QEMU takes ~1 hour vs building entire UTM.app (~3-4 hours)
- 🔄 **Easy updates** - Patch new UTM releases as they come out
- 📦 **Smaller builds** - Only need to build QEMU, not all UTM dependencies
- ✅ **Tested base** - Start from stable UTM releases

**Build Time Comparison:**
- Full UTM.app build: 3-4 hours (builds all dependencies + QEMU + GUI app)
- QEMU-only build: ~1 hour (just QEMU with helix-frame-export)
- Patching production UTM: 5 minutes (copy binary + re-sign)

**What Gets Patched:**
- Only `qemu-aarch64-softmmu` binary (the ARM64 QEMU system emulator)
- Location: `UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu`
- Size: ~29MB
- Changes: Adds helix-frame-export module for VideoToolbox encoding via vsock

## QEMU Build Process

### Build Approach

Instead of using UTM's full dependency build system (which had reliability issues), we built QEMU directly against pre-built dependencies from UTM's sysroot:

```bash
# Configure QEMU with helix-frame-export module
cd ~/pm/qemu-utm
export PKG_CONFIG_PATH="$HOME/pm/helix/UTM/sysroot-macOS-arm64/lib/pkgconfig"
./configure \
  --enable-shared-lib \
  --disable-cocoa \
  --cpu=aarch64 \
  --target-list=aarch64-softmmu \
  --extra-cflags="-I$HOME/pm/helix/UTM/sysroot-macOS-arm64/include" \
  --extra-ldflags="-L$HOME/pm/helix/UTM/sysroot-macOS-arm64/lib"

# Build (produces qemu-system-aarch64-unsigned + libqemu-aarch64-softmmu.dylib)
make -j28
```

### Build Issues Fixed

#### 1. virglrenderer Dependency Not Found

**Problem**: QEMU configure couldn't find virglrenderer via pkg-config, so helix-frame-export.m couldn't include virglrenderer.h.

**Solution**: Added fallback manual dependency in `hw/display/helix/meson.build`:

```meson
virglrenderer = dependency('virglrenderer', required: false)
if not virglrenderer.found()
  virglrenderer_inc = include_directories('/Users/luke/pm/helix/UTM/sysroot-macOS-arm64/include/virgl')
  virglrenderer_lib = declare_dependency(
    include_directories: virglrenderer_inc,
    link_args: ['-L/Users/luke/pm/helix/UTM/sysroot-macOS-arm64/lib', '-lvirglrenderer']
  )
  virglrenderer = virglrenderer_lib
endif
```

**Commit**: 886c4e4797

#### 2. virglrenderer API Version Incompatibility

**Problem**: `hw/display/virtio-gpu-virgl.c` defined `virgl_borrow_texture_for_scanout()` wrapper function, but this function is only used when `VIRGL_VERSION_MAJOR < 1`. The function definition wasn't version-guarded, causing compilation errors with virglrenderer 1.2.0.

**Error messages**:
```
error: variable has incomplete type 'struct virgl_renderer_texture_info'
error: call to undeclared function 'virgl_renderer_borrow_texture_for_scanout'
```

**Solution**: Wrapped the function definition in version guard at `virtio-gpu-virgl.c:415`:

```c
#if VIRGL_VERSION_MAJOR < 1
static GLuint virgl_borrow_texture_for_scanout(uint32_t id, bool *y_0_top,
                                               uint32_t *width,
                                               uint32_t *height,
                                               void **d3d_tex2d)
{
    // ... implementation ...
}
#endif
```

This matches the usage pattern at line 502 where the function is only called when `VIRGL_VERSION_MAJOR < 1`.

**Commit**: 886c4e4797

### Build Verification

After successful build, verified helix-frame-export module is compiled into the binary:

```bash
# Check symbols
nm libqemu-aarch64-softmmu.dylib | grep helix
# Output:
# 0000000000357f34 t _helix_encode_iosurface
# 0000000000358300 t _helix_frame_export_cleanup
# 0000000000358380 t _helix_frame_export_init
# 0000000000357e30 t _helix_get_iosurface_for_resource

# Check error/log strings
strings libqemu-aarch64-softmmu.dylib | grep "helix:"
# Output includes expected error messages from helix-frame-export.m
```

### Repository

Modified QEMU source: https://github.com/helixml/qemu-utm (utm-edition branch)

**Key commits**:
- `4237f5099b`: Initial helix-frame-export integration (copied from for-mac/qemu-helix)
- `dda666bc6d`: Renamed helix-frame-export.c → .m for Objective-C compilation
- `886c4e4797`: Fixed build issues (virglrenderer deps + API compatibility)

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

### Venus/Vulkan Known Limitations (2026-02-03)

**Status**: Venus is **experimental** in UTM 5.0+. Testing on Ubuntu 25.10 ARM64 VM reveals:

- **OpenGL (virgl) works**: `glxinfo` shows virgl renderer, `glxgears` gets ~52 FPS (vsync)
- **Vulkan (Venus) partially broken**: `vulkaninfo` shows `VK_ERROR_INCOMPATIBLE_DRIVER (-9)` from `libvulkan_virtio.so`, falls back to llvmpipe software rendering
- **GNOME can fall back**: While modern mutter prefers Vulkan, it can use OpenGL backend when Vulkan unavailable

**UTM 5.0 Release Notes state**:
- Apple CoreGL backend does **NOT** support Vulkan
- **DXVK does not work** due to missing features in Venus implementation
- Venus passthrough is incomplete

**UTM Graphics Configuration Options** (found in VM settings UI):

1. **Renderer Backend** (how host renders guest GL/Vulkan):
   - `Default` - Auto-select (usually ANGLE OpenGL)
   - `ANGLE (OpenGL)` - OpenGL → Metal via ANGLE
   - `ANGLE (Metal)` - Direct Metal backend (faster, better Vulkan support)
   - `Apple Core OpenGL` - Deprecated macOS OpenGL (no Vulkan support)

2. **Vulkan Driver** (guest Vulkan → host translation):
   - `Default` - Auto-select
   - `Disabled` - No Vulkan passthrough
   - `MoltenVK` - Use MoltenVK for Vulkan → Metal translation (recommended for Venus)
   - `KosmicKrisp` - Mesa Venus driver name (guest-side)

**Recommended Configuration for Venus/Vulkan:**
- Renderer Backend: `ANGLE (Metal)` - Uses Metal directly instead of OpenGL
- Vulkan Driver: `MoltenVK` - Explicitly enable Vulkan → Metal translation

**Setting Graphics Options Programmatically:**

These are **global UTM preferences** (stored in `~/Library/Preferences/com.utmapp.UTM.plist`), not per-VM settings:

```bash
# Set Renderer Backend to ANGLE (Metal)
defaults write com.utmapp.UTM QEMURendererBackend -int 2

# Set Vulkan Driver to MoltenVK
defaults write com.utmapp.UTM QEMUVulkanDriver -int 2

# Restart UTM for changes to take effect
```

**Value Reference:**
- `QEMURendererBackend`: 0=Default, 1=ANGLE(OpenGL), 2=ANGLE(Metal), 3=Apple Core OpenGL
- `QEMUVulkanDriver`: 0=Default, 1=Disabled, 2=MoltenVK, 3=KosmicKrisp

**Implication for Helix**: We can proceed with **virgl OpenGL acceleration** for frame capture. Modern GNOME will use OpenGL rendering path when Vulkan is unavailable. The virglrenderer stack still provides GPU-accelerated rendering via OpenGL → ANGLE → Metal, which is sufficient for desktop streaming.

**Workaround if needed**: Set `MUTTER_DEBUG_FORCE_KMS_MODE=simple` to explicitly force OpenGL backend.

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

## Distribution Architecture (Docker Desktop Model)

### Bundle Structure

```
Helix.app/
├── Contents/
│   ├── MacOS/
│   │   ├── Helix              # Wails app (Swift/Go)
│   │   └── utmctl             # UTM CLI for VM control
│   ├── Frameworks/
│   │   ├── qemu-aarch64-softmmu.framework
│   │   ├── virglrenderer.0.framework
│   │   └── ... (UTM dependencies)
│   ├── Resources/
│   │   ├── VM/
│   │   │   └── helix-linux.utm/    # Preconfigured Ubuntu VM
│   │   │       ├── config.plist
│   │   │       └── Data/
│   │   │           └── disk.qcow2  # VM disk with Helix pre-installed
│   │   └── qemu/                   # BIOS, firmware
│   └── Info.plist
```

### Preconfigured VM Image

**What's pre-installed:**
- Ubuntu 25.10 ARM64
- Docker CE + Docker Compose (official packages)
- Helix sandbox built and ready
- Go 1.23.6
- PipeWire + GStreamer with vsockenc plugin
- GPU drivers (virgl) configured
- SSH server with host key access
- vsock support enabled

**Graphics Configuration:**
- virtio-gpu-gl-pci with `blob=true,venus=true`
- Host UTM settings: ANGLE (Metal) + MoltenVK
- Port forwarding: SSH (22→2222), API (8080→8080)

**VM Management:**
- for-mac app sets UTM graphics preferences on first launch:
  ```go
  exec.Command("defaults", "write", "com.utmapp.UTM", "QEMURendererBackend", "-int", "2").Run()
  exec.Command("defaults", "write", "com.utmapp.UTM", "QEMUVulkanDriver", "-int", "2").Run()
  ```
- App starts/stops VM via bundled `utmctl`
- API requests proxied from native app to VM's Helix instance
- Video streaming handled via vsock or WebSocket

### User Experience

1. User downloads `Helix.app` (single DMG/PKG installer)
2. First launch: App configures UTM settings, starts VM (takes ~30s)
3. Native macOS menubar shows Helix status
4. Web UI opens to Helix running in VM (localhost:8080)
5. Dev containers run inside VM with GPU acceleration
6. App handles all VM lifecycle transparently

**No manual VM setup required** - everything preconfigured and bundled.

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
| `main.go` | Wails entry point, macOS menu, systray integration |
| `app.go` | Main app struct, VM lifecycle, ZFS/settings/tray methods |
| `vm.go` | VMManager - QEMU process control with virtio-gpu + vsock |
| `utm.go` | UTMManager - utmctl CLI integration for VM control |
| `video.go` | VideoEncoder - encoding stats/state |
| `websocket.go` | WebSocket server for H.264 streaming (port 8765) |
| `encoder.go` | VideoToolbox H.264 encoder (Obj-C via cgo) |
| `vsock.go` | vsock frame exchange server (guest frame requests) |
| `virgl.go` | virglrenderer integration (resource ID → IOSurface) |
| `settings.go` | AppSettings persistence (~/.helix/settings.json) |
| `zfs.go` | ZFS stats collector via SSH (dedup ratio, compression, pool usage) |
| `tray.go` | macOS menu bar icon (getlantern/systray) |
| `tray_icon.go` | Generated 22x22 hexagon PNG for menu bar |
| `frontend/index.html` | Sidebar + content layout with SVG nav icons |
| `frontend/src/style.css` | Helix dark theme (matches web UI design tokens) |
| `frontend/src/main.js` | Sidebar navigation, Home/VM/Storage/Settings views |

### Dependencies

- **Wails v2**: Go ↔ WebView bridge with macOS native features
- **getlantern/systray**: macOS menu bar icon
- **QEMU**: Custom build with helix-frame-export (bundled in Helix.app)
- **VideoToolbox**: Apple's hardware encoder framework (system framework)

### End-to-End Build Process

**Full build from source:**
```bash
# 1. Build custom QEMU with helix-frame-export
cd ~/pm/helix
./for-mac/qemu-helix/build-qemu-standalone.sh  # → installs into UTM.app

# 2. Build the Wails desktop app
cd for-mac
wails build                                      # → build/bin/Helix.app

# 3. Bundle QEMU into Helix.app (future: automated in wails build)
# Copy qemu-aarch64-softmmu + 14 dylibs + EFI firmware into Helix.app/Contents/Frameworks/
```

**Development workflow:**
```bash
cd for-mac
wails dev    # Hot-reload: Go changes rebuild, frontend via Vite HMR
```

### End-to-End Integration Flow

```
User launches Helix.app
  → systray icon appears in macOS menu bar
  → Wails window opens with sidebar UI
  → Home view shows "Start VM" placeholder

User clicks Start VM (or uses tray menu):
  → app.StartVM() → VMManager.Start() → spawns qemu-system-aarch64
  → QEMU boots Ubuntu ARM64 VM with:
    - virtio-gpu-gl-pci (virgl3d GPU acceleration)
    - vhost-vsock-pci (host↔guest communication)
    - port forwarding: SSH(2222), API(8080), Video(8765)
  → VM runs Docker → helix-sandbox → helix-ubuntu containers
  → ZFSCollector starts polling via SSH every 10s

VM running:
  → Home view loads iframe to http://localhost:8080 (Helix web UI)
  → VM view shows live stats (CPU, memory, uptime, encoding FPS)
  → Storage view shows ZFS dedup ratio and disk usage
  → Settings persist to ~/.helix/settings.json

Video streaming path:
  Guest: PipeWire ScreenCast → DMA-BUF → vsockenc/pipewiresrc
  Host: VsockServer → IOSurface → VideoToolbox H.264 → WebSocket
  Browser: WebSocket → MSE/WebCodecs → <video>
```

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

## Desktop App UX Design

### Network Access Settings
- **Default**: Web interface only accessible on localhost (127.0.0.1)
- **Optional**: Expose to local network (configurable in Settings page)
- When exposed, users can be configured via the admin panel
- Security: Off by default, explicit opt-in required

### Default User Experience
- App runs as local admin user by default (no login required for single-user)
- Main window shows embedded web view of Helix interface
- No separate browser needed for basic usage

### Settings Page Requirements
- Toggle: "Allow access from other devices on network" (default: OFF)
- When ON: Show bind address (0.0.0.0) and port
- Link to admin panel for user management (when network access enabled)
- Warning about security implications when enabling network access

### Desktop App UI Architecture (Docker Desktop-style)

**Implemented**: Sidebar + content layout matching Helix web UI branding.

**Layout:**
```
┌─────────────────────────────────────────┐
│ ⬡ Helix                       [─][□][×]│
├────────┬────────────────────────────────┤
│ SIDEBAR│  MAIN CONTENT                  │
│        │                                │
│ Home   │  [WebView: Helix UI iframe]    │
│ VM     │  or                            │
│ Storage│  [Dashboard / Settings / etc]  │
│ Settngs│                                │
│        │                                │
│ ──────│                                │
│ ● VM OK│                                │
│ 3 sess.│                                │
└────────┴────────────────────────────────┘
```

**Views:**
1. **Home** — Embedded iframe showing `http://localhost:8080` (Helix web UI) when VM is running. Placeholder with hexagon logo when stopped.
2. **VM** — Status card (state, CPU, memory, uptime, sessions), video encoding stats (FPS, clients, frames), Start/Stop buttons, SSH/API quick actions with copy.
3. **Storage** — ZFS dedup ratio hero banner, pool usage with progress bars, disk breakdown (root, ZFS, host actual after dedup). Data polled every 10s via SSH.
4. **Settings** — VM config (CPUs, memory, disk path), network ports (SSH, API, video), save/reset with toast notification.

**Design tokens** (matching `frontend/src/themes.tsx`):
- Background: `#121214` (primary), `#1e1e24` (panels)
- Borders: `#282838`
- Teal accent: `#00d5ff` (active states, links)
- Magenta accent: `#EF2EC6` (highlights)
- Gradient: `#00c8ff → #6f00ff` (progress bars)
- Glass-effect sidebar with `backdrop-filter: blur(20px)`

**System tray** (getlantern/systray):
- Helix hexagon icon in macOS menu bar
- Menu: status line, Open Helix, Start/Stop VM, Quit
- Wails window hides to tray on close (`HideWindowOnClose: true`)
- Systray runs on main goroutine (macOS requirement), Wails in a goroutine

**QEMU bundling strategy:** Bundle `qemu-aarch64-softmmu` + ~14 dylibs + EFI firmware directly into Helix.app (~900MB). No full UTM UI needed.

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
| `app.go` | ✅ Complete | Main app with VsockServer + VideoToolboxEncoder + ZFS stats + settings |
| `video.go` | ✅ Complete | Video encoding stats/state |
| `websocket.go` | ✅ Complete | WebSocket server for browser streaming |
| `vm.go` | ✅ Complete | VM manager interface |
| `settings.go` | ✅ Complete | AppSettings struct + JSON persistence at ~/.helix/settings.json |
| `zfs.go` | ✅ Complete | ZFS stats collector via SSH (pool size, dedup ratio, compression) |
| `tray.go` | ✅ Complete | macOS menu bar icon using getlantern/systray |
| `tray_icon.go` | ✅ Complete | Generates 22x22 hexagon PNG for menu bar (template image) |

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
# Install frontend dependencies and build
cd frontend && npm install && npm run build && cd ..
# Build Go app (requires macOS with Wails CLI: go install github.com/wailsapp/wails/v2/cmd/wails@latest)
wails build        # Production build → build/bin/Helix.app
# Or for development with hot-reload:
wails dev          # Opens app with Vite dev server
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

## Current Status Summary (2026-02-03)

**VM Environment:** ✅ Fully Operational
- macOS ARM64 VM (Ubuntu 25.10, 64GB RAM, 256GB disk)
- Venus/Vulkan GPU acceleration working (via virtio-gpu + MoltenVK)
- Docker CE + Docker Compose installed and running
- GPU devices accessible: `/dev/dri/card0`, `/dev/dri/renderD128`
- Vendor ID: `0x1af4` (virtio-gpu), Device ID: `0x1050`

**Build Status:** ✅ Complete (ARM64-Native)
- helix-ubuntu desktop image: Built successfully (ID: b7fb9b)
- helix-sandbox container: Running and healthy with vfs storage driver
- All dependencies: ARM64-native (LibreOffice, Chromium, Ghostty ARM64)
- Cross-platform issues resolved: AMD64 stages commented out

**Image Transfer:** 🔄 In Progress
- helix-ubuntu:b7fb9b successfully pushed to host registry
- Transfer to sandbox's nested dockerd: In progress (vfs storage driver is slow)
- Once complete: Ready for session creation and testing

**Next Steps:**
1. Complete image transfer to sandbox
2. Create test Helix session with helix-ubuntu desktop
3. Verify PipeWire ScreenCast works with Venus/Vulkan
4. Test baseline video streaming (x264enc software encoding)
5. Implement vsockenc GStreamer element for VideoToolbox delegation

**Sandbox Profile Decision:**
- Current: Using `code-software` profile (misleading name for virtio-gpu)
- **Decision: Add `code-macos` profile** (not code-virtio)
  - Rationale: vsockenc is macOS-specific (delegates to VideoToolbox)
  - Windows with virtio-gpu can use nvenc in guest (Windows supports CUDA in VMs)
  - Profile name should reflect the host OS, not the virtualization technology
- Required changes:
  - docker-compose.dev.yaml: New `code-macos` profile definition
  - stack script: GPU vendor detection for 0x1af4 → set code-macos profile on macOS hosts
  - desktop-bridge: Select vsockenc encoder when running on macOS (vendor 0x1af4)

## Progress Log

### 2026-02-03: Video Streaming Functional, Ready for vsockenc Implementation

**Major Milestone:** End-to-end video streaming pipeline working on ARM64 VM!

**Completed:**
- ✅ Fixed overlay2 storage driver (was incorrectly using vfs)
  - Image transfers: vfs took 30+ min, overlay2 takes ~2-3 min
  - Kernel 6.17.0 has overlay module loaded - no special config needed
- ✅ Created test session with helix-ubuntu desktop (ses_01kgjkc6qcxs3qf568xbt4p3yv)
- ✅ **PipeWire ScreenCast confirmed working:**
  - ScreenCast sessions created successfully
  - **DmaBuf enabled** (critical for zero-copy path)
  - Screenshot capture working (139KB JPEG via PipeWire node 53)
- ✅ **Video streaming functional:**
  - H.264 frames flowing via WebSocket
  - Codec: H.264 Baseline with zero-latency decode
  - Currently using x264enc (software encoding)
  - FPS low (0.3 fps) on static desktop - expected for damage-based ScreenCast

**GPU Detection Issue:**
- Vendor ID 0x1af4 (virtio-gpu) not recognized by detect-render-node.sh
- Falls back to software rendering mode
- **Solution needed:** Implement code-macos profile with vendor 0x1af4 detection

**Next Steps:**
1. ~~**Design vsockenc GStreamer element**~~ ✅ Implemented in C
2. **Integrate vsockenc into helix-ubuntu build**
3. **Implement host-side VideoToolbox encoder** (vsock-encoder-server)
4. **Add code-macos sandbox profile** for virtio-gpu (vendor 0x1af4)
5. Test zero-copy encoding: DmaBuf → vsock → VideoToolbox → H.264

**Key Achievement:** The entire video pipeline works - PipeWire capture with DmaBuf, H.264 encoding, WebSocket streaming. This validates the architecture before implementing vsockenc.

### 2026-02-03: vsockenc GStreamer Element Implemented

**vsockenc Implementation Complete:**
- ✅ Implemented in C (`desktop/gst-vsockenc/gstvsockenc.c`, 688 lines)
- ✅ Full Helix Frame Export Protocol support with structured messages
- ✅ **DmaBuf → virtio-gpu resource ID extraction** via DRM ioctls
- ✅ vsock communication with host encoder (CID 2, port 5000)
- ✅ Async receive thread for encoded H.264 frames
- ✅ Configurable bitrate and keyframe interval
- ✅ Proper PTS/DTS handling and frame queue management

**Protocol Implementation:**
- Helix Frame Export Protocol with magic `0x52465848` ('HXFR')
- Message types: FRAME_REQUEST, FRAME_RESPONSE, CONFIG_REQ, ERROR, PING/PONG
- Frame request: resource_id, width, height, format (BGRA/RGBA/NV12), stride, pts, duration
- Frame response: pts, dts, is_keyframe, nal_count + variable-length NAL units

**Key Technical Implementation:**
```c
// Extract virtio-gpu resource ID from DmaBuf
int drm_fd = open("/dev/dri/renderD128", O_RDWR);
struct drm_prime_handle prime_handle = { .fd = dmabuf_fd };
ioctl(drm_fd, DRM_IOCTL_PRIME_FD_TO_HANDLE, &prime_handle);
resource_id = prime_handle.handle;  // For virtio-gpu, GEM handle == resource ID
```

**Build System:**
- Meson build configured (`desktop/gst-vsockenc/meson.build`)
- Dependencies: gstreamer-1.0, gstreamer-video, gstreamer-allocators, libdrm
- Installs to `/usr/lib/gstreamer-1.0/libgstvsockenc.so`

**Remaining Work:**
1. ~~Integrate vsockenc meson build into helix-ubuntu Dockerfile~~ ✅ Done
2. Align protocol between vsockenc (C) and vsock.go (Go)
3. Implement QEMU modification for virglrenderer access
4. Test resource ID extraction with real PipeWire DmaBuf frames

### 2026-02-03: vsockenc Integration and Protocol Design

**vsockenc Build Integration:**
- ✅ Added ARM64 build stage to Dockerfile.ubuntu-helix
- ✅ Meson build compiles vsockenc from `desktop/gst-vsockenc/`
- ✅ Installs `/usr/lib/aarch64-linux-gnu/gstreamer-1.0/libgstvsockenc.so`
- ✅ Build dependencies: meson, ninja, pkg-config, GStreamer dev, libdrm
- ⏳ Testing build in VM (packages installing)

**Host-Side Infrastructure Discovered:**
- `for-mac/vsock.go` - VsockServer implementation with frame protocol
- `for-mac/virgl.go` - virglrenderer integration with ResourceToIOSurfaceID
- `for-mac/encoder.go` - VideoToolbox encoder (existing)

**Protocol Mismatch Found:**
vsockenc (C) uses **Helix Frame Export Protocol**:
- Magic: `0x52465848` ('HXFR')
- HelixMsgHeader: magic, msg_type, flags, session_id, payload_size
- HelixFrameRequest: full header + resource_id, width, height, format, stride, pts, duration
- HelixFrameResponse: full header + pts, dts, is_keyframe, nal_count + NAL units

vsock.go (Go) uses **Simple Protocol**:
- Header: type (1 byte) + length (4 bytes)
- FrameRequest: flat struct (no header)
- FrameResponse: flat struct

**Action Required:** Update vsock.go to match Helix Frame Export Protocol.

**QEMU Integration Module Already Implemented!**

Found complete QEMU module in `for-mac/qemu-helix/`:
- ✅ **helix-frame-export.h** - Protocol definitions matching vsockenc exactly
- ✅ **helix-frame-export.c** (500+ lines) - Complete implementation:
  - `helix_get_iosurface_for_resource()` - Calls virgl_renderer_resource_get_info_ext from QEMU process
  - `helix_encode_iosurface()` - IOSurface → CVPixelBuffer → VideoToolbox (zero-copy)
  - `encoder_output_callback()` - Async H.264 NAL unit handling
  - `handle_frame_request()` - vsock message handler
- ✅ **README.md** - Integration instructions for UTM's QEMU fork
- ✅ **meson.build** - Build system integration

**Integration Steps** (from README):
1. Clone UTM's QEMU fork from https://github.com/utmapp/qemu
2. Copy `for-mac/qemu-helix/` to `hw/display/helix/` in QEMU
3. Add `subdir('helix')` to `hw/display/meson.build`
4. Modify `hw/display/virtio-gpu-virgl.c` to call `helix_frame_export_init()`
5. Add QEMU option `--device virtio-gpu,helix-frame-export=on`
6. Rebuild UTM with modified QEMU

**Status:** ~~Module is feature-complete and ready to integrate. Just needs QEMU fork + rebuild UTM.~~ ✅ **Integration complete!**

### 2026-02-03: QEMU Integration Complete

**helix-frame-export Successfully Integrated into UTM's QEMU:**

Completed integration in `~/pm/qemu-utm/` (utm-edition branch):
- ✅ Copied `for-mac/qemu-helix/` → `hw/display/helix/`
- ✅ Modified `hw/display/meson.build`:
  - Added `subdir('helix')` in virtio-gpu section
  - helix module builds conditionally on macOS (`host_machine.system() == 'darwin'`)
  - Depends on: VideoToolbox, CoreVideo, CoreMedia, IOSurface, Metal, virglrenderer
- ✅ Modified `hw/display/virtio-gpu-virgl.c`:
  - Added `#include "helix/helix-frame-export.h"` (macOS only)
  - Added `helix_frame_export_init(g, HELIX_VSOCK_PORT)` to `virtio_gpu_virgl_init()`
  - Non-fatal error handling - continues without frame export if init fails
- ✅ Committed: `4237f5099b` "Add Helix frame export for zero-copy VideoToolbox encoding"

**What This Enables:**
- QEMU can now call `virgl_renderer_resource_get_info_ext()` to get MTLTexture from virtio-gpu resource
- MTLTexture.iosurface provides IOSurface for zero-copy VideoToolbox encoding
- vsock communication between guest (vsockenc) and host (QEMU helix module)
- Complete zero-copy path: Guest GPU → virglrenderer → Metal → VideoToolbox → H.264

**Next Steps:**
1. Build modified QEMU
2. Rebuild UTM.app with modified QEMU
3. Test with helix-ubuntu desktop container running vsockenc

### 2026-02-03: ARM64 Desktop Build Complete, Transfer In Progress

**Achievements:**
- ✅ Fixed Dockerfile.ubuntu-helix for ARM64-native builds
  - Commented out AMD64-specific CUDA/Rust build stages
  - Replaced Chrome (amd64-only) with Chromium ARM64
  - Fixed Ghostty to use ARM64 packages
  - Replaced OnlyOffice with LibreOffice (ARM64 available)
- ✅ Built helix-ubuntu desktop image successfully
  - Image ID: b7fb9b60b3e3
  - Size: ~7.4GB compressed layers
  - All components ARM64-native
- ✅ Fixed sandbox container restart loop
  - Changed storage driver from overlay2 to vfs (DinD compatibility)
  - Made helix-sway optional (was causing crashes)
- ✅ Identified GPU vendor: 0x1af4 (virtio-gpu)
  - Need new sandbox profile for macOS/virtio-gpu systems
  - Current workaround: COMPOSE_PROFILES=code-software

**Challenges Resolved:**
- ✅ **Storage Driver Issue**: Fixed overlay2 support (was incorrectly disabled)
  - Root cause: Mistakenly assumed overlay2 doesn't work with nested DinD on ARM64
  - Reality: overlay2 works perfectly (kernel 6.17.0 has overlay module loaded)
  - Solution: Changed daemon.json from vfs → overlay2, restarted dockerd
  - Result: Dramatically faster image operations (vfs was taking 30+ min, overlay2 takes ~2-3 min)

**Technical Decisions Made:**
1. **Storage Driver**: overlay2 (standard, fast, fully supported on ARM64)
2. **Sandbox Profile**: Use `code-macos` (not code-virtio - reflects host OS where encoding happens)
3. **Build Strategy**: ARM64-only builds, no cross-compilation

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
- Remote: https://github.com/helixml/qemu (branch: `helix-frame-export`)

### 2026-02-02: UTM Build Fixes (Late Evening)

**Problem:** UTM's QEMU tarball has a bug where `ui/sdl2-gl.c` has inconsistent state - the function signature was patched but the body still contains an old `backing_borrow()` call that no longer exists.

**Root Cause Analysis:**
1. UTM's `qemu-10.0.2-utm.tar.xz` tarball is pre-patched
2. But the patches are in an inconsistent state - some hunks applied, others didn't
3. The `sdl2_gl_scanout_texture()` function has:
   - New signature with `ScanoutTextureNative native` parameter
   - But still has `GLuint backing_texture = backing_borrow(...)` call in body
4. Additionally, macOS shared library builds fail with `_qemu_main` undefined because dylibs reference symbols from the main executable

**Fixes Applied to UTM Patch File:**

Added two patches to `/Users/luke/pm/helix/UTM/patches/qemu-10.0.2-utm.patch`:

1. **sdl2-gl.c Fix**: Remove vestigial `backing_borrow()` call
```diff
--- a/ui/sdl2-gl.c
+++ b/ui/sdl2-gl.c
-    GLuint backing_texture = backing_borrow(backing_id, &backing_y_0_top,
-                                            &backing_width, &backing_height,
-                                            &d3d_tex2d);
```

2. **meson.build Fix**: Add `-undefined dynamic_lookup` for macOS shared lib builds
```diff
--- a/meson.build
+++ b/meson.build
+  if get_option('shared_lib')
+    emulator_link_args += ['-Wl,-undefined,dynamic_lookup']
+  endif
```

**Build Result:**
- ✅ QEMU builds successfully with all target architectures
- ✅ All `libqemu-*-softmmu.dylib` shared libraries created
- ✅ All `qemu-system-*` binaries installed
- ❌ Mesa/virglrenderer build fails due to missing `libclc` (OpenCL dependency) - separate issue

**Installed QEMU Artifacts:**
- `/Users/luke/pm/helix/UTM/sysroot-macOS-arm64/bin/qemu-system-aarch64`
- `/Users/luke/pm/helix/UTM/sysroot-macOS-arm64/lib/libqemu-aarch64-softmmu.dylib` (30MB)
- Plus all other target architectures

### 2026-02-02: UTM.app Successfully Built!

**Mesa/virglrenderer Dependencies Fixed:**
- Installed `libclc` from Homebrew for OpenCL support
- Installed `spirv-llvm-translator` for SPIRV-Tools
- Installed `spirv-tools` for Mesa shader compilation
- Created custom venv at `/tmp/helix-utm-venv` with required Python packages:
  - six, pyparsing, pyyaml, mako, distlib, setuptools, packaging

**virglrenderer Build:**
- ✅ Built successfully (2.9MB library)
- ✅ Provides GPU virtualization for guest VMs

**MoltenVK Build:**
- ✅ Built manually with Xcode (10.6MB dylib → 5.1MB after lipo for arm64)
- ✅ Installed to sysroot with ICD JSON

**Kosmickrisp (Mesa Vulkan) Workaround:**
- ❌ Full build requires extensive X11 dependencies (libxrandr, etc.)
- ✅ Created stub `vulkan_kosmickrisp.framework` to satisfy Xcode build
- Note: Not needed for Venus/Vulkan passthrough - MoltenVK is sufficient

**Library-to-Framework Conversion:**
- Created `/tmp/utm-fixup.sh` to convert 87 dylibs to macOS frameworks
- Frameworks installed to `sysroot-macOS-arm64/Frameworks/`

**UTM.app Build:**
- ✅ **BUILD SUCCEEDED** with Xcode
- Build target: arm64 only (not universal binary)
- App location: `/Users/luke/pm/helix/UTM/build-macOS-arm64/UTM.app` (754MB)
- Signed with "Sign to Run Locally" (ad-hoc signing)

**Key Components Built:**
| Component | Size | Status |
|-----------|------|--------|
| QEMU (all targets) | 30MB+ each | ✅ |
| virglrenderer | 2.9MB | ✅ |
| libepoxy | 1.3MB | ✅ |
| MoltenVK | 5.1MB | ✅ |
| GStreamer + plugins | Multiple | ✅ |
| SPICE | Multiple | ✅ |
| UTM.app (total) | 754MB | ✅ |

**Next Steps:**
1. Test UTM.app launches and can create VMs
2. Test Venus/Vulkan passthrough with existing Ubuntu VM
3. Integrate helix frame export code into QEMU
4. Test frame capture and VideoToolbox encoding

### 2026-02-03: Mac Studio Setup & VM Testing

**New Hardware:**
- Mac Studio M3 Ultra (96GB RAM, 28 cores)
- macOS 26.2 (Tahoe)

**Environment Setup:**
- ✅ SSH key generated and added to GitHub
- ✅ Git config set (me@lukemarsden.net)
- ✅ Xcode 26.2 installed
- ✅ Metal Toolchain downloaded
- ✅ Passwordless sudo configured

**UTM Build on Mac Studio:**
- ✅ Copied sysroot from old Mac via rsync (complete with headers, libs, frameworks)
- ✅ UTM.app built successfully from source
- ✅ Ad-hoc code signed (`codesign --force --deep --sign -`)
- ✅ VM copied from old Mac (Linux.utm, 11GB)

**Code Signing Limitations Discovered:**
- vmnet (Shared/Bridged networking) requires Apple-signed entitlement `com.apple.vm.networking`
- Ad-hoc signed apps cannot use vmnet - switched to "Emulated" networking (QEMU user-mode NAT)
- GPU passthrough (virtio-gpu-gl) does NOT require special entitlements - just normal OpenGL/Metal access

**VM Configuration (HVF Mode - not TCG):**
- VM running with `Hypervisor=true` (using Apple Hypervisor Framework for ARM64)
- virtio-gpu-gl-pci with `hostmem=256M,blob=true,venus=true`
- SSH port forwarding: guest:22 → host:2222
- Ubuntu 25.10 ARM64 (Questing)

**GPU Acceleration Verified:**
- ✅ **OpenGL (virgl)**: `glxinfo` shows "virgl" renderer, `glxgears` gets ~52 FPS (vsync)
- ⚠️ **Vulkan (Venus)**: Not working - `VK_ERROR_INCOMPATIBLE_DRIVER`, falls back to llvmpipe
- Venus is experimental in UTM 5.0, but virgl OpenGL is sufficient for our needs

**Helix VM Setup Complete:**
- ✅ Docker CE 29.2.1 installed from official repository
- ✅ Docker Compose v5.0.2 installed (plugin)
- ✅ Go 1.23.6 installed
- ✅ Helix repository cloned
- ✅ vsock support confirmed: `/dev/vsock` and `/dev/vhost-vsock` present
- ✅ PipeWire running in user session
- ✅ GPU devices: `/dev/dri/card0` (virtio-gpu) and `/dev/dri/renderD128` (render node)

**Graphics Configuration Testing (2026-02-03):**

Tested all UTM graphics backend combinations to enable Venus/Vulkan:

| Renderer Backend | Vulkan Driver | Vulkan Result | OpenGL Result | Notes |
|------------------|---------------|---------------|---------------|-------|
| Default | Default | llvmpipe | virgl | Original config, baseline |
| ANGLE (OpenGL) | MoltenVK | llvmpipe | llvmpipe | Both fail |
| Default | MoltenVK | **Venus (M3 Ultra)** ✅ | virgl | **WORKING!** |
| ANGLE (Metal) | KosmicKrisp | llvmpipe | llvmpipe | Both fail |

**Venus/Vulkan Success:**
- **Device**: `Virtio-GPU Venus (Apple M3 Ultra)` ✅
- **Driver**: Mesa Venus 25.2.8
- **Root cause**: libMoltenVK.dylib was in wrong location
- **Fix**: MoltenVK_icd.json uses relative path `./libMoltenVK.dylib`, needs dylib in `icd.d/` directory
- **Solution**: Copy libMoltenVK.dylib to `UTM.app/Contents/Resources/vulkan/icd.d/`
- **Updated**: build-utm-macos.sh now installs MoltenVK correctly

**Key Findings:**
- Venus/Vulkan NOW WORKING with Default renderer + MoltenVK driver ✅
- Guest Vulkan apps use Apple M3 Ultra GPU via MoltenVK on host
- virgl OpenGL still works (shows "virgl" renderer)
- Build script updated to fix MoltenVK packaging automatically

**Conclusion for Phase 1:**
- ✅ Venus/Vulkan acceleration working end-to-end
- ✅ OpenGL/virgl acceleration working
- **Recommended config**: Default renderer + MoltenVK (QEMURendererBackend=0, QEMUVulkanDriver=2)
- Guest can use both Vulkan (via Venus) and OpenGL (via virgl) simultaneously

**VM Setup Status:**
- ✅ SSH access working (port 2222)
- ✅ Docker CE 29.2.1 + Docker Compose v5.0.2 installed
- ✅ Go 1.23.6 installed
- ✅ Helix repository cloned
- ✅ GPU devices accessible inside Docker containers (`/dev/dri/card0`, `/dev/dri/renderD128`)
- ✅ vsock support confirmed
- ✅ PipeWire running
- ✅ **GPU acceleration working end-to-end:**
  - Host: Venus/Vulkan using Apple M3 Ultra via MoltenVK
  - Docker containers: Can access virtio-gpu and use Venus driver
  - Verified: `docker run --device=/dev/dri` shows "Virtio-GPU Venus (Apple M3 Ultra)"

**Building Helix Stack in VM (Completed):**
- ✅ VM memory increased to 64GB for build
- ✅ Zed binary (308MB, release build) copied to VM
- ✅ qwen-code repository cloned
- ✅ ARM64 build fixes for Dockerfile.ubuntu-helix:
  - Commented out x86_64 CUDA symlink (not applicable to ARM/virtualized GPUs)
  - Changed Go download from linux-amd64 to linux-arm64
  - Replaced Google Chrome (amd64-only) with Chromium (has ARM64 build)
    - Updated all paths: /etc/chromium/policies, /opt/chromium, /usr/bin/chromium-browser
    - Chromium wrapper script with --password-store=basic flag
    - Chrome DevTools MCP server works with Chromium
  - Updated Ghostty installation to use ARM64 packages
    - Previously assumed ARM64 packages didn't exist
    - Ghostty 1.2.3 has full ARM64 support
- ✅ helix-ubuntu desktop image built successfully (7.42GB, image ID: 0f7b633fa9af)
  - GNOME 49.0 with Wayland
  - Mesa Vulkan drivers 25.2.8 (Venus-ready)
  - Chromium browser
  - Ghostty terminal 1.2.3
  - gst-pipewire-zerocopy Rust plugin compiled
  - desktop-bridge and settings-sync-daemon Go binaries
- ✅ helix-sandbox container built successfully
  - Docker-in-Docker with ARM64 support
  - Hydra multi-container isolation daemon
  - Skipped NVIDIA toolkit (will use VideoToolbox on macOS host)
  - Skipped ROCm (amd64-only)
  - Ready to host helix-ubuntu sessions

**Remaining Work for Golden Image:**
1. ✅ **Build Zed** - Already built on host, binary copied to VM
2. ✅ **Build desktop images** - helix-ubuntu built and ready
3. ✅ **Build Helix sandbox** - Built with ARM64 support
4. **Test frame capture** - Verify PipeWire ScreenCast works in dev containers
5. **Build vsockenc plugin** - GStreamer element for guest→host frame delegation (future)
6. **Test end-to-end** - Start session, capture frames, verify video streaming works

**Sandbox Container Status (VM):**
- ✅ Sandbox container running and healthy
- ✅ Dockerd running with vfs storage driver (overlay2 incompatible with nested DinD)
- ✅ Hydra daemon running with RevDial connection to API
- ✅ Sandbox heartbeat daemon monitoring disk usage
- ✅ Desktop image loaded: helix-ubuntu (0f7b63)
- ✅ Fixed: Made helix-sway optional (was causing container crashes when version file missing)

## Video Streaming Pipeline Architecture (Clarified)

**IMPORTANT**: This section clarifies the video encoding architecture based on actual implementation requirements.

### Component Responsibilities

**pipewirezerocopysrc** (GStreamer Rust plugin):
- Captures frames from PipeWire ScreenCast
- Provides DMA-buf-backed video frames to the GStreamer pipeline
- **Does NOT care about encoding** - it's encoder-agnostic
- Works the same way on NVIDIA GPUs, virtio-gpu, and software rendering

**desktop-bridge** (Go server in dev containers):
- **Detects GPU vendor** at startup
- **Selects appropriate encoder** based on detected GPU:
  - NVIDIA GPU → `nvh264enc` (uses NVENC hardware encoder)
  - virtio-gpu/macOS → `vsockenc` (new element, sends frames to host via vsock)
  - Software/fallback → `x264enc` (CPU encoding)
- Constructs GStreamer pipeline with appropriate encoder element
- Streams encoded H.264 over WebSocket to browser

### Pipeline on NVIDIA GPU (Linux)
```
PipeWire ScreenCast → pipewirezerocopysrc (DMA-buf frames)
    → nvh264enc (NVENC hardware encoding, zero-copy)
    → desktop-bridge WebSocket → browser
```

### Pipeline on virtio-gpu (macOS VM)
```
PipeWire ScreenCast → pipewirezerocopysrc (DMA-buf frames from virtio-gpu)
    → vsockenc (new GStreamer element, macOS-specific):
        1. Extracts virtio-gpu resource ID from DMA-buf
        2. Sends resource ID to host via vsock
        3. Host receives resource ID via vsock server
        4. Host looks up MTLTexture via virglrenderer API
        5. Host encodes with VideoToolbox (VTCompressionSession)
        6. Host sends H.264 NAL units back via vsock
        7. vsockenc outputs NAL units to GStreamer pipeline
    → desktop-bridge WebSocket → browser
```

**Note on Windows**: On Windows hosts with virtio-gpu, guests can potentially use nvenc directly (Windows supports CUDA in VMs), so vsockenc delegation is macOS-specific.

### Host-Side VideoToolbox Encoder (macOS)

**vsock-encoder-server** (new Go daemon on macOS host):
- Listens on vsock port for encoding requests
- Receives virtio-gpu resource IDs from guest
- Calls virglrenderer API to get MTLTexture from resource ID
- Uses VideoToolbox API (VTCompressionSession) to encode frames
- Sends compressed H.264 NAL units back to guest
- **No GStreamer dependency** - uses VideoToolbox directly via Cgo

### GPU Vendor Detection (desktop-bridge)

desktop-bridge detects GPU vendor at startup:

```go
// Detect GPU vendor from DRM render node
func detectGPUVendor() string {
    // Read /sys/class/drm/card0/device/vendor
    vendor := readFile("/sys/class/drm/card0/device/vendor")

    switch vendor {
    case "0x10de":  // NVIDIA
        return "nvidia"
    case "0x1af4":  // Red Hat (virtio-gpu)
        return "virtio"
    default:
        return "unknown"
    }
}

// Select encoder based on GPU vendor
func selectEncoder(vendor string) string {
    switch vendor {
    case "nvidia":
        return "nvh264enc"
    case "virtio":
        return "vsockenc"  // Delegates to host VideoToolbox
    default:
        return "x264enc"   // Software fallback
    }
}
```

### Implementation Plan

**Phase 1: Testing (Current)**
1. ✅ Build helix-ubuntu desktop image
2. ✅ Build helix-sandbox container
3. ✅ Start sandbox and verify all services running
4. **Next**: Create test session, verify PipeWire ScreenCast works
5. **Next**: Test existing pipewirezerocopysrc with software encoding (x264enc fallback)

**Phase 2: vsockenc Implementation**
1. Create `vsockenc` GStreamer element (Rust, similar structure to pipewirezerocopysrc)
2. Add GPU vendor detection to desktop-bridge
3. Modify desktop-bridge pipeline construction to use vsockenc for virtio-gpu
4. Create `vsock-encoder-server` (Go daemon for macOS host)
5. Integrate virglrenderer API calls (get MTLTexture from resource ID)
6. Integrate VideoToolbox encoding (VTCompressionSession)

**Phase 3: Integration Testing**
1. Test vsockenc in VM with vsock-encoder-server on host
2. Verify zero-copy path (GPU memory → VideoToolbox without CPU copies)
3. Benchmark performance (FPS, latency, CPU usage)
4. Compare with NVENC performance on Linux

**Next Immediate Steps:**
1. ✅ Sandbox container running
2. **Create test Helix session** using helix-ubuntu desktop
3. **Verify PipeWire ScreenCast** captures frames from GNOME
4. **Test with x264enc** (software encoding) as baseline
5. **Design vsockenc GStreamer element** (Rust plugin)

## In-Place Upgrades (Future)

Once the `install.sh` approach is working, support in-place upgrades by re-running `install.sh`:

**Requirements:**
1. **Publish ARM64 Docker images** — helix-ubuntu, helix-sandbox images must be published for linux/arm64
2. **Publish host-level VM binary** — The QEMU + Helix.app bundle needs a download location (GitHub releases or CDN)
3. **install.sh handles upgrades** — Re-running `install.sh` should:
   - Pull latest ARM64 Docker images into the VM
   - Update the host-level Helix.app binary (QEMU + Wails frontend)
   - Preserve user data (VM disk, settings, ZFS pools)
   - Handle version migration if config format changes
4. **Version check** — App should check for updates and prompt user (or auto-update)

**Key insight:** The install.sh approach makes upgrades much simpler since we can just re-run the script. The main challenge is ensuring ARM images are published alongside amd64 images in CI (multi-arch manifests).

## Scanout Mode Integration Notes (2026-02-08)

### Key Fixes Applied

1. **DRM socket mount**: `/run/helix-drm.sock` must be bind-mounted into the `sandbox-macos` Docker Compose service. Without it, desktop containers can't request DRM leases.

2. **Monitor connector detection**: In DRM scanout mode, Mutter uses real DRM connectors (e.g., `Virtual-5`, `Virtual-14`) instead of headless `Meta-0`. desktop-bridge queries `org.gnome.Mutter.DisplayConfig.GetResources()` to detect the actual connector name. `GetCurrentState()` doesn't work with godbus (complex nested structs cause `dbus.Store: length mismatch`).

3. **ScreenCast session required for input**: Even in scanout mode where video comes from QEMU TCP (not PipeWire), a linked ScreenCast session is required. Mutter's `NotifyPointerMotionAbsolute` and `NotifyTouchDown` look up the stream from `session->screen_cast_session` for coordinate mapping. Without it, absolute mouse and touch silently fail.

4. **SharedVideoSource for scanout**: Scanout frames now go through `SharedVideoSource` (same as PipeWire path) for:
   - GOP buffer (keyframe catchup for mid-stream joins)
   - Frame keepalive (re-send last frame every 100ms on static screens)
   - Single QEMU TCP connection shared across all WebSocket clients
   - `FrameSource` interface abstracts over `GstPipeline` and `ScanoutSource`

5. **Cursor monitoring**: The `monitorCursor()` goroutine must be started in scanout mode too. The GNOME Shell extension (`helix-cursor@helix.ml`) sends cursor shape changes via Unix socket regardless of video mode.

6. **Resolution plumbing**: `ScanoutSource` reads `GAMESCOPE_WIDTH`/`GAMESCOPE_HEIGHT` env vars for DRM lease requests instead of hardcoding 1080p.

### EDID for 5K Resolution

QEMU's virtio-gpu default mode list doesn't include 5120x2880. Enabled via EDID:
- UTM config: `-global virtio-gpu-gl-pci.edid=on -global virtio-gpu-gl-pci.xres=5120 -global virtio-gpu-gl-pci.yres=2880`
- Wails app: `virtio-gpu-gl-pci,id=gpu0,edid=on,xres=5120,yres=2880`

EDID only adds modes to the available list — doesn't allocate framebuffers. Containers requesting 1080p still get 1080p (exact mode match in `getPreferredMode`). Requires VM restart to take effect.

### VideoToolbox Bitrate Scaling

`helix-frame-export.c` scales bitrate with pixel count (~4 bits/pixel):
- 1080p: 8 Mbps
- 4K (3840x2160): ~33 Mbps
- 5K (5120x2880): ~59 Mbps

Running on localhost so bandwidth is not a concern.

### Virtio-GPU Connector Limits

`VIRTIO_GPU_MAX_SCANOUTS` is 16 (hardcoded in virtio-gpu protocol spec, both QEMU and guest kernel). This gives 15 usable scanouts (index 0 = VM console). Bumping to 64 requires patching both QEMU and guest kernel headers since the `pmodes` array in the virtio config space is fixed-size.

### Cursor Pipeline (Independent of Video)

Cursor works identically in scanout and PipeWire modes:
1. GNOME Shell extension detects cursor type via hotspot fingerprinting (Helix-Invisible transparent theme)
2. Sends CSS cursor name over `/run/user/1000/helix-cursor.sock`
3. desktop-bridge `monitorCursorPipeWire()` reads socket, sends `StreamMsgCursorName` (0x51) over WebSocket
4. Frontend renders SVG cursor overlay

### Input Pipeline (Independent of Video)

All input goes through D-Bus RemoteDesktop session:
- Keyboard: `NotifyKeyboardKeycode` / `NotifyKeyboardKeysym` (no stream path needed)
- Mouse buttons: `NotifyPointerButton` (no stream path needed)
- Relative mouse: `NotifyPointerMotionRelative` (no stream path needed)
- **Absolute mouse**: `NotifyPointerMotionAbsolute` (needs stream path from linked ScreenCast)
- **Touch**: `NotifyTouchDown/Motion/Up` (needs stream path from linked ScreenCast)
- Scroll: `NotifyPointerAxis` with FINGER source for smooth trackpad scrolling

### Damage Keepalive

The cursor-based damage keepalive (`runDamageKeepalive`) is disabled in scanout mode (video doesn't come from PipeWire). Static screen keepalive is handled by `SharedVideoSource.broadcastFrames()` which re-sends the last H.264 frame every 100ms from the GOP buffer.

### TODO

- [x] Show scanout usage in Helix for Mac UI (X/15 displays in use)
  - Added `for-mac/drm_stats.go` — ScanoutCollector polls DRM connector status via SSH every 5s
  - Added `GetScanoutStats()` Wails binding in app.go
  - Frontend shows active/total displays with progress bar and per-display details in VM view
- [x] Plumb per-session bitrate from JS frontend through to VideoToolbox encoder via `HELIX_MSG_CONFIG_REQ`
  - Frontend already sends bitrate in WebSocket init JSON (`bitrate` field, kbps)
  - Added `WriteConfigRequest()` to Go protocol (`api/pkg/drm/protocol.go`)
  - Added `SetBitrate(kbps)` to `ScanoutSource` — sends CONFIG_REQ to QEMU TCP
  - `startScanoutMode()` passes `v.config.Bitrate` to ScanoutSource before Start()
  - QEMU `client_handler_thread()` now handles `HELIX_MSG_CONFIG_REQ` — reconfigures per-scanout encoder
  - `HelixScanoutEncoder` struct gains `bitrate` field, `create_scanout_encoder()` accepts bitrate param
  - Default bitrate auto-scales from resolution (~4 bits/pixel, minimum 5 Mbps)
- ~~Bump `VIRTIO_GPU_MAX_SCANOUTS` to 64~~ — Deferred (requires kernel patch, not worth the effort)
- [x] Test 5K resolution end-to-end after VM restart with EDID
  - Fixed UTM plist format: AdditionalArguments uses bare `<string>` elements, not `<dict>` wrappers
  - Fixed QEMU virtio-gpu-base.c: EDID xres/yres was only applied to scanout 0, now applies to all scanouts
  - Fixed mutter-lease-launcher: hardcoded `RequestLease(1920, 1080)`, now reads GAMESCOPE_WIDTH/HEIGHT
  - Fixed GRUB kernel cmdline: `video=Virtual-1:1920x1080` to prevent console running at 5K
  - Frontend default bitrate scaled with resolution: ~4 bits/pixel (59 Mbps at 5K, 33 Mbps at 4K)
- [x] Frame export performance optimization (triple-copy → single-copy)
  - **Before**: GPU → temp malloc → DisplaySurface → new IOSurface (3×59MB at 5K = 177MB/frame)
  - **After**: GPU → cached IOSurface via `virgl_renderer_transfer_read_iov()` (1×59MB, no malloc)
  - Cached IOSurface reused across frames (created once per resolution)
  - QEMU CPU usage at 5K was 87.6% due to memory bandwidth (3.5 GB/sec of memcpy)
  - Expected improvement: ~3x frame rate from eliminating unnecessary copies
- [ ] Further performance investigation: virglrenderer GPU readback is still 59 MB/frame at 5K.
  Consider dirty region tracking (SPICE does 32×32 block comparison) or lower resolution as practical default.
