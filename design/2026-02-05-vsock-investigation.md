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

### ✅ SOLVED: Initialization Working

**Problem:** `helix_frame_export_init()` appeared not to be executing - socket wasn't being created in `/tmp`.

**Root Cause:** macOS sandboxing blocks QEMU from writing to `/tmp`, causing socket creation to fail silently.

**Solution:** Changed socket path from `/tmp/helix-frame-export.sock` to relative path `helix-frame-export.sock` in QEMU's CWD:
- QEMU CWD: `/Users/luke/Library/Group Containers/WDNLXAD4W8.com.utmapp.UTM/`
- Socket successfully created and listening
- Confirmed via marker file: `helix-init-called.txt`

**Verification:**
```bash
$ ls -la "/Users/luke/Library/Group Containers/WDNLXAD4W8.com.utmapp.UTM/" | grep helix
srwxr-xr-x   1 luke  staff     0  5 Feb 10:07 helix-frame-export.sock
-rw-r--r--   1 luke  staff    54  5 Feb 10:07 helix-init-called.txt
```

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

### Current Status: Initialization Working, Need Guest Access

The host-side socket is created and listening. Now need to make it accessible to the guest VM.

### Immediate: Connect Guest to Host Socket

**Options for guest-host communication:**

### ✅ Completed:
- [x] Socket creation working
- [x] Guest-host communication path established (TCP proxy)
- [x] Protocol PING/PONG tested and working

### 🚧 Next: Test Frame Export with Real GPU Resources

1. **Create test virtio-gpu resource in guest**
   - Run simple Vulkan/GL app to create a framebuffer
   - Get resource ID from virglrenderer
   - OR use existing GNOME desktop framebuffer

2. **Test virgl_renderer_resource_get_info_ext()**
   - Send FrameRequest with real resource ID
   - Verify function returns Metal texture handle
   - Check IOSurface backing exists

3. **Test VideoToolbox encoding**
   - Verify IOSurface → CVPixelBuffer conversion works
   - Check H.264 encoding produces valid NALs
   - Measure encoding latency

4. **Build vsockenc replacement**
   - Modify desktop-bridge vsockenc to connect to 10.0.2.2:5900
   - Use PipeWire ScreenCast to get DmaBuf resource IDs
   - Send FrameRequest messages
   - Receive and output H.264 NALs

5. **End-to-end integration**
   - Test with helix-ubuntu container
   - Stream to browser via WebSocket
   - Measure FPS and latency
   - Compare to current x86 performance

## Known Challenges

### 1. Guest Access to Host Socket [IN PROGRESS]

**Current:** Socket exists at `/Users/luke/Library/Group Containers/WDNLXAD4W8.com.utmapp.UTM/helix-frame-export.sock` on host, but guest can't access it.

**Options:**

**A) virtserialport** (Recommended - proper solution)
- QEMU already has virtserialport devices for guest agent and SPICE
- Guest accesses via `/dev/virtio-ports/helix-frame-export`
- Requires:
  - Convert socket code to use chardev backend
  - Add `-device virtserialport,chardev=helix-export,name=helix-frame-export` to QEMU command
  - Add `-chardev socket,path=helix-frame-export.sock,server=on,wait=off,id=helix-export` to QEMU command
- Challenge: Need to modify UTM config or use AdditionalArguments

**B) 9p/virtfs** (Quick test)
- Mount host directory into guest
- Guest accesses socket directly via mounted path
- UTM supports shared folders in UI
- Quick way to test protocol before implementing virtserialport

**C) TCP proxy** (Workaround)
- Run socat on host: `socat TCP-LISTEN:5900,fork UNIX-CONNECT:helix-frame-export.sock`
- Guest connects to host via virtio-net
- Not zero-copy, adds latency
- Only for testing

**Current solution:** Using TCP proxy via socat for testing:
```bash
# On host:
socat TCP-LISTEN:5900,bind=127.0.0.1,fork,reuseaddr \
  UNIX-CONNECT:"/Users/luke/Library/Group Containers/WDNLXAD4W8.com.utmapp.UTM/helix-frame-export.sock" &

# Guest connects to:
10.0.2.2:5900  # QEMU user-mode networking forwards to host 127.0.0.1:5900
```

Verified connection works from guest.

**✅ PROTOCOL TESTED AND WORKING!**

Test client successfully sent PING and received PONG response:
```
Connecting to 127.0.0.1:5900...
Connected! Sending PING...
PING sent, waiting for PONG...
Received response:
  Magic: 0x52465848 (expected: 0x52465848)
  Type: 0x11 (expected: 0x11 for PONG)
  Session: 1

✅ SUCCESS! Helix frame export protocol working!
```

End-to-end communication path confirmed:
1. QEMU helix-frame-export module listening on UNIX socket ✅
2. socat TCP proxy forwarding to socket ✅
3. Client connects and sends messages ✅
4. Server receives and responds correctly ✅

**Note:** UTM ignores AdditionalArguments and SharedDirectories config for 9p/virtfs. TCP proxy is sufficient for testing. For production, will implement virtserialport in QEMU code directly.

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
