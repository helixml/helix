# QEMU Build Verification - 2026-02-04

## Summary

Successfully built modified QEMU with helix-frame-export module and patched it into UTM.app. The module is verified to be compiled into the binary, but end-to-end testing requires the Helix development environment (Docker, helix CLI) which is not currently running.

## What Was Completed

### 1. QEMU Build ✅

Built QEMU from `~/pm/qemu-utm` with helix-frame-export module integrated:

**Build artifacts**:
- `qemu-system-aarch64-unsigned` (56KB stub executable)
- `libqemu-aarch64-softmmu.dylib` (29MB library with helix-frame-export)

**Repository**: https://github.com/helixml/qemu-utm (utm-edition branch)

**Key commits**:
- `4237f5099b` - Initial helix-frame-export integration
- `dda666bc6d` - Renamed .c → .m for Objective-C
- `886c4e4797` - Fixed virglrenderer deps + API compatibility

### 2. Build Issues Fixed ✅

#### Issue 1: virglrenderer Dependency Not Found
- **Problem**: pkg-config couldn't find virglrenderer
- **Fix**: Added manual dependency with explicit paths in `hw/display/helix/meson.build`

#### Issue 2: virglrenderer API Version Incompatibility
- **Problem**: `virgl_borrow_texture_for_scanout()` function definition not version-guarded
- **Fix**: Wrapped function in `#if VIRGL_VERSION_MAJOR < 1` guard in `virtio-gpu-virgl.c`

### 3. UTM.app Patching ✅

**Location**: `~/pm/helix/UTM/build/Build/Products/Release/UTM.app`

**Steps taken**:
1. Backed up original QEMU binary
2. Copied `libqemu-aarch64-softmmu.dylib` to UTM framework directory:
   ```
   UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu
   ```
3. Re-signed with ad-hoc signature: `codesign --force --deep --sign - UTM.app`

**Verification**:
- Code signature: `Signature=adhoc` ✅
- Binary size: 29MB ✅
- Timestamp: 2026-02-04 07:20 ✅

### 4. Module Verification ✅

Verified helix-frame-export symbols are present in the compiled QEMU binary:

```bash
nm libqemu-aarch64-softmmu.dylib | grep helix
```

**Found symbols**:
- `_helix_encode_iosurface` - Encodes IOSurface via VideoToolbox
- `_helix_frame_export_init` - Initializes vsock listener
- `_helix_frame_export_cleanup` - Cleanup handler
- `_helix_frame_export_process_msg` - Handles vsock messages from guest
- `_helix_get_iosurface_for_resource` - Extracts IOSurface from Metal texture

**Found strings** (error/log messages from helix-frame-export.m):
- "helix: Helix frame export initialized on vsock port %d"
- "helix: virgl_renderer_resource_get_info_ext failed: %d"
- "helix: Resource %u is not a Metal texture (type=%d)"
- "helix: VTCompressionSessionCreate failed: %d"
- "helix: CVPixelBufferCreateWithIOSurface failed: %d"

This confirms the helix-frame-export module is fully compiled and linked into the QEMU binary.

## What Needs Testing

### Current Blocker: Development Environment Not Running

The following components are not available for testing:
- Docker daemon not running
- Go toolchain not in PATH
- helix CLI not built (`/tmp/helix` doesn't exist)
- No API key configuration files (`.env.usercreds`, `.env.userkey`)

### Next Steps for Testing

To test the zero-copy pipeline, the following is needed:

1. **Start Docker environment**:
   ```bash
   # Start Helix development stack
   ./stack start
   ```

2. **Build helix CLI**:
   ```bash
   cd api && CGO_ENABLED=0 go build -o /tmp/helix .
   ```

3. **Configure credentials** (create `.env.usercreds`):
   ```bash
   export HELIX_API_KEY="hl-xxx"
   export HELIX_URL="http://localhost:8080"
   export HELIX_PROJECT="prj_xxx"
   export HELIX_UBUNTU_AGENT="agent_xxx"
   ```

4. **Configure UTM VM for vsock**:
   The current running VM doesn't have vsock device configured. Need to add to QEMU command line:
   ```
   -device vhost-vsock-pci,guest-cid=3
   ```

5. **Start test session**:
   ```bash
   /tmp/helix spectask start --agent $HELIX_UBUNTU_AGENT --project $HELIX_PROJECT -n "zero-copy test"
   ```

6. **Monitor helix-frame-export initialization**:
   ```bash
   # Check system logs for helix module initialization
   log show --predicate 'eventMessage CONTAINS "helix"' --last 5m --style syslog

   # Should see: "helix: Helix frame export initialized on vsock port 5001"
   ```

7. **Test video streaming**:
   ```bash
   # Wait for GNOME to initialize (~15 seconds)
   sleep 15

   # Run benchmark with vkcube for active damage (60 FPS)
   /tmp/helix spectask benchmark ses_xxx --video-mode zerocopy --duration 30
   ```

8. **Verify zero-copy path**:
   - Check desktop-bridge logs for "Using encoder: vsockenc"
   - Check QEMU logs for helix-frame-export processing frames
   - Measure FPS and latency vs software x264enc baseline

## Architecture Flow (When Working)

```
Guest (helix-ubuntu container)
  ├── GNOME ScreenCast → PipeWire → DmaBuf
  ├── pipewirezerocopysrc → virtio-gpu resource ID extraction
  └── vsockenc → sends resource ID via vsock port 5001

Host (macOS)
  ├── QEMU receives vsock message
  ├── helix-frame-export calls virgl_renderer_resource_get_info_ext()
  ├── Gets Metal texture from virtio-gpu resource
  ├── Extracts IOSurface from Metal texture
  ├── CVPixelBufferCreateWithIOSurface()
  ├── VTCompressionSessionEncodeFrame() → H.264
  └── Returns encoded frame to vsockenc → WebSocket

Client (Browser)
  └── Receives H.264 frames via WebSocket
```

## Current Status

✅ **Build phase complete** - QEMU with helix-frame-export successfully built and patched
⏳ **Testing phase pending** - Requires Helix dev environment to be running

The modified QEMU is ready and waiting for the development environment to be started for end-to-end testing.

## Files Modified

### QEMU Repository (~/pm/qemu-utm)

- `hw/display/helix/helix-frame-export.m` - VideoToolbox encoding module
- `hw/display/helix/meson.build` - Build configuration with virglrenderer fallback
- `hw/display/virtio-gpu-virgl.c` - Added version guard for virglrenderer API compatibility
- `hw/display/meson.build` - Added helix subdirectory

### Helix Design Docs

- `design/2026-02-02-macos-arm-desktop-port.md` - Updated with build status and QEMU build process documentation

## Next Session Recommendations

When resuming work on this project:

1. **Check if UTM.app is still running** with the Linux VM
2. **Start Docker environment** if not running: `./stack start`
3. **Build helix CLI** if needed: `cd api && CGO_ENABLED=0 go build -o /tmp/helix .`
4. **Set up test credentials** in `.env.usercreds`
5. **Add vsock device to UTM VM configuration** (may need to edit VM settings or use CLI)
6. **Run end-to-end zero-copy pipeline test** as outlined above
7. **Compare performance** with software x264enc baseline

The infrastructure is ready - just needs the runtime environment to test the complete pipeline.
