# vsock Implementation Investigation

**Date:** 2026-02-05
**Status:** In Progress - Debugging device initialization

## Goal

Implement zero-copy frame export from guest GPU to host VideoToolbox encoding via vsock/IPC.

## Current Status

### ✅ Confirmed Working

1. **VM is running** with HVF hardware acceleration
2. **virtio-gpu device is working** in the guest:
   - Guest sees PCI device 1AF4:1050 (virtio-gpu)
   - DRI devices present: `/dev/dri/card0`, `/dev/dri/renderD128`
   - GPU acceleration functional
3. **Venus/Vulkan path** is configured:
   - QEMU command line has `-device virtio-gpu-gl-pci`
   - SPICE has `gl=on`
   - This uses virglrenderer with Vulkan support
4. **Custom QEMU build** works:
   - Build time: 2-5 minutes (incremental)
   - helix-frame-export module compiles and links
   - All symbols present in binary

### ❌ Issue: Device Initialization Not Being Called

**Problem:** `helix_frame_export_init()` is not being executed, even though the virtio-gpu device IS working.

**Investigation:**

1. Added debug markers to initialization functions:
   - `virtio_gpu_gl_pci_initfn()` - PCI device init
   - `virtio_gpu_gl_device_realize()` - VirtIO device realization
   - `virtio_gpu_virgl_init()` - virgl renderer init
   - `helix_frame_export_init()` - our module init

2. **File-based markers failed** (sandboxing blocks `/tmp` writes)
   - Switched to `error_report()` logging
   - Need to find where these logs go

3. **Device IS created** (confirmed by checking guest DRI devices)
   - So initialization MUST be happening
   - Logs just aren't visible yet

### 🔧 Implementation Changes

#### 1. UNIX Socket Backend (Instead of vsock)

macOS doesn't have kernel vsock support. Implemented:

```c
// helix-frame-export.m
Socket path: /tmp/helix-frame-export.sock
Transport: UNIX domain socket (AF_UNIX, SOCK_STREAM)
Protocol: Same helix frame export protocol as vsock version
```

Guest will need a client that connects to this socket (instead of true vsock CID 2, port 5000).

#### 2. Fixed Library Paths

Updated `fix-qemu-paths.sh` to include missing libraries:
- libspice-server.1.dylib
- libvirglrenderer.1.dylib
- libusbredirparser.1.dylib
- libusb-1.0.0.dylib
- libgmodule-2.0.0.dylib

#### 3. Debug Logging

Added `[HELIX-DEBUG]` prefixed error_report() calls throughout initialization:
- PCI device init
- VirtIO device realize
- virgl renderer init
- helix frame export init

## Architecture

### Device Stack

```
QEMU Command Line: -device virtio-gpu-gl-pci
         ↓
virtio-gpu-pci-gl.c: virtio_gpu_gl_initfn()
         ↓
virtio-gpu-gl.c: virtio_gpu_gl_device_realize()
         ↓
virtio-gpu-gl.c: virtio_gpu_gl_handle_ctrl()  [on guest GPU commands]
         ↓
virtio-gpu-virgl.c: virtio_gpu_virgl_init()
         ↓
helix-frame-export.m: helix_frame_export_init()
```

### Frame Export Path (Zero-Copy)

```
Guest Container:
  GNOME renders → PipeWire ScreenCast → DmaBuf (virtio-gpu resource)
  vsockenc extracts resource ID
  Connects to /tmp/helix-frame-export.sock
  Sends FrameRequest(resource_id, pts, dimensions)
         ↓
Host (QEMU helix-frame-export):
  Accepts connection on UNIX socket
  Receives FrameRequest
  Calls virgl_renderer_resource_get_info_ext(resource_id)
  Gets Metal texture from resource (zero-copy!)
  Creates CVPixelBuffer from IOSurface
  VideoToolbox encodes → H.264 NALs
  Sends FrameResponse(pts, nal_data) back via socket
         ↓
Guest Container:
  vsockenc receives H.264 NALs
  Outputs to GStreamer → WebSocket → Browser
```

## Next Steps

### Immediate: Find QEMU Logs

**Priority:** Confirm `helix_frame_export_init()` is actually being called

Options:
1. Check Console.app for QEMULauncher logs
2. Find UTM debug log location (DebugLog=true in config)
3. Run QEMU directly outside UTM to see stderr
4. Use `dtruss` to trace file/socket operations

### Once Initialization Confirmed:

1. **Verify socket is created**
   - Check `/tmp/helix-frame-export.sock` exists
   - Verify permissions and ownership
   - Test connection from guest

2. **Build guest client**
   - Modify vsockenc to connect to UNIX socket (need to mount /tmp from host?)
   - OR use virtserialport for guest→host communication
   - OR implement host→guest socket forwarding

3. **Test protocol**
   - Send test FrameRequest from guest
   - Verify helix-frame-export receives it
   - Check virgl_renderer_resource_get_info_ext() works
   - Verify Metal texture extraction
   - Test VideoToolbox encoding

4. **End-to-end test**
   - Start helix-ubuntu container
   - Run desktop-bridge with vsockenc
   - Stream video to browser
   - Measure FPS and latency

## Known Challenges

### 1. Guest Access to Host Socket

Problem: Guest VM needs to connect to `/tmp/helix-frame-export.sock` on host

Options:
- **virtserialport**: QEMU device for guest↔host communication
- **9p/virtfs**: Mount host /tmp into guest
- **Port forwarding**: Forward a TCP port to the UNIX socket
- **vhost-user backend**: Custom vhost-user device (complex)

Current thinking: Use virtserialport as it's well-supported and designed for this.

### 2. virgl Resource Access

The `virgl_renderer_resource_get_info_ext()` function:
- Only available in virglrenderer with Metal backend
- Requires UTM 5.0+ (has the right virglrenderer version)
- Returns native Metal texture handle
- **Need to verify**: Does it actually work with Venus/Vulkan rendering?

### 3. Performance with HVF

- Current VM uses HVF (hardware virtualization)
- Should be fast enough for real-time encoding
- TCG would be too slow (~10x slower)

## Files Changed

### helix repo (committed)
- `for-mac/qemu-helix/fix-qemu-paths.sh` - Added missing library paths

### qemu-utm repo (committed to utm-edition branch)
- `hw/display/helix/helix-frame-export.m` - UNIX socket backend implementation
- `hw/display/virtio-gpu-virgl.c` - Unconditional helix init, debug logging
- `hw/display/virtio-gpu-gl.c` - Debug logging in device realize
- `hw/display/virtio-gpu-pci-gl.c` - Debug logging in PCI init

## Build Commands

### Rebuild QEMU (2-5 min)
```bash
cd ~/pm/qemu-utm && ninja -C build
```

### Install to UTM
```bash
sudo cp ~/pm/qemu-utm/build/libqemu-aarch64-softmmu.dylib \
     /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu
sudo ~/pm/helix/for-mac/qemu-helix/fix-qemu-paths.sh
sudo codesign --force --sign - \
     /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework
```

### Restart VM
```bash
/Applications/UTM.app/Contents/MacOS/utmctl stop Linux
/Applications/UTM.app/Contents/MacOS/utmctl start Linux
```

## Key Insights

1. **Device IS working** - Guest has functioning virtio-gpu with DRI
2. **Initialization happens** - Device wouldn't work otherwise
3. **Logging is the blocker** - Need to find where error_report() output goes
4. **Sandboxing blocks /tmp** - QEMU can't write arbitrary files
5. **Venus uses virglrenderer** - Same code path we're hooking into

## References

- Design doc: `design/2026-02-02-macos-arm-desktop-port.md`
- Previous status: `design/2026-02-05-current-status-summary.md`
- Overnight progress: `design/2026-02-04-overnight-progress-summary.md`
