# macOS ARM Video Streaming - Status Update

**Date:** 2026-02-06 (updated)
**Status:** VIDEO STREAMING WORKING - 23 FPS on static screens, 26 FPS with active content (cursor-embedded keepalive fix)

## Summary

Successfully built ARM64 support for Helix on macOS, but encountering build system issues that prevent testing the video streaming functionality.

## Completed Work

### ✅ ARM64 Support
- `build-sandbox` now automatically transfers desktop images on first run
- Added `code-macos` profile support to `get_sandbox_names()`
- Merged docker0 networking fixes from main branch
- Both `helix-sway` and `helix-ubuntu` desktop images build successfully on ARM64

### ✅ QEMU Crash Fix (Theory)
Identified root cause of VM crashes:
- **Problem**: guest compositor frees scanout resources while QEMU reads them
- **Solution**: Reject `resource_id=0` (scanout) and require explicit DmaBuf resource IDs from guest

Code changes in `qemu-utm/hw/display/helix/helix-frame-export.m`:
1. Added resource validation before `virgl_renderer_transfer_read_iov()` (commit 3f5b75c994)
2. Reject scanout resources entirely (commit 97620617e1)

## Recent Progress (2026-02-05 18:25)

### ✅ Fixed GPU Device Mounting in Hydra

**Problem**: Desktop containers had no `/dev/dri/` devices mounted, preventing video capture.

**Root Cause**: `configureGPU()` in `api/pkg/hydra/devcontainer.go` only handled nvidia/amd/intel GPUs. On macOS with virtio-gpu, it fell through to the default case which did nothing.

**Fix**: Modified default case to mount `/dev/dri/renderD*` and `/dev/dri/card*` devices when available (commit b0599449d).

**Result**:
- ✅ Desktop containers now have `/dev/dri/card0` and `/dev/dri/renderD128` mounted
- ✅ Pipeline starts successfully with vsockenc
- ❌ Still 0 frames sent - likely due to no screen activity in headless GNOME

### 🔍 Next Investigation: Screen Activity Required

GNOME ScreenCast in headless mode is damage-based - it only produces frames when the screen changes. A static desktop produces 0 FPS.

**Need to test with**:
- vkcube (constant GPU rendering)
- Terminal with fast output
- Mouse movement via desktop-bridge input injection

## Previous Blockers (RESOLVED)

### ✅ QEMU Build System Issues (FIXED)

**Problem**: Custom QEMU builds don't install correctly into UTM.app

**Status**: RESOLVED - All library paths fixed recursively, VM boots successfully with patched QEMU.

**Symptoms**:
1. `./stack build-utm` compiles successfully
2. Object files contain the patched code (verified with `strings`)
3. Code is included in sysroot dylib
4. **BUT**: When copied to UTM.app, dylib has hardcoded sysroot paths:
   ```
   /Users/luke/pm/UTM/sysroot-macOS-arm64/lib/libpixman-1.0.dylib
   /Users/luke/pm/UTM/sysroot-macOS-arm64/lib/libjpeg.62.dylib
   ...
   ```
5. UTM's sandbox blocks access to these paths → VM won't start

**Root Cause**: Library paths need to be rewritten from absolute paths to `@rpath` paths, but `fix-qemu-paths.sh` script doesn't exist.

**Evidence**:
```bash
# Patch IS in object file:
$ strings ~/pm/qemu-utm/build/libcommon.a.p/hw_display_helix_helix-frame-export.m.o | grep "About to call"
[HELIX] About to call virgl_renderer_transfer_read_iov...

# But running VM uses old QEMU:
$ ls -lh /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu
-rwxr-xr-x  1 root  admin    33M  5 Feb 11:05  # BEFORE scanout rejection commit (11:09)
```

**Attempted Fixes**:
1. ✅ Forced recompilation with `touch helix-frame-export.m`
2. ✅ Verified code in object file
3. ✅ Installed from sysroot to UTM.app
4. ✅ Created `scripts/fix-qemu-paths.sh` to fix library paths
5. ✅ Fixed main QEMU dylib paths (14 libraries)
6. ✅ Copied dependency libraries to UTM Frameworks
7. ❌ **Deep dependency chain**: Copied libraries ALSO have sysroot paths
   - Example: `libspice-server.1.dylib` → `libssl.1.1.dylib` → more deps
   - Each dylib in the chain needs path fixing
   - Recursive dependency resolution needed

**Blocker Details**:
The custom QEMU has ~30+ dependency libraries, each with their own dependencies.
All paths must be recursively fixed to use `@rpath`. This requires:
- Iterating through all copied dylibs
- Running `install_name_tool` on each
- Handling transitive dependencies
- Testing each iteration

Estimated effort: 2-4 hours to build robust recursive path fixer.

## Recommended Path Forward

**Priority 1: Test with Stock UTM QEMU** ⭐
The fastest way to validate the rest of the stack is working:

1. Check if stock UTM has helix-frame-export (it shouldn't)
2. Test basic streaming to see if vsockenc → QEMU connection works
3. Verify resource ID extraction from DmaBuf
4. Expected: May crash on scanout resources, but proves pipeline connectivity

**Priority 2: Build Recursive Library Path Fixer**
Create enhanced `fix-qemu-paths.sh`:
```bash
# Pseudo-code:
for each dylib in /Applications/UTM.app/Contents/Frameworks/*.dylib:
    fix_library_paths(dylib)

for each dependency in dylib:
    if starts_with(dependency, "/Users/"):
        copy_to_frameworks(dependency)
        fix_library_paths(dependency)
        recurse(dependency)
```

**Priority 3: Alternative - Use UTM's Build System**
Instead of standalone build, integrate into UTM's own build:
- Clone UTM repo
- Add helix-frame-export to UTM's QEMU patches
- Use `Scripts/build.sh` which handles all library paths correctly
- Produces UTM.app with custom QEMU pre-integrated

## Next Steps (Original Options)

### Option 1: Recursive Library Path Fixer
Create `scripts/fix-qemu-paths.sh` to rewrite library paths:
```bash
#!/bin/bash
QEMU_DYLIB="/Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu"

# Get all sysroot paths
for lib in $(otool -L "$QEMU_DYLIB" | grep sysroot | awk '{print $1}'); do
    lib_name=$(basename "$lib")
    install_name_tool -change "$lib" "@rpath/$lib_name" "$QEMU_DYLIB"
done

# Update ID
install_name_tool -id "@rpath/qemu-aarch64-softmmu" "$QEMU_DYLIB"
```

### Option 2: Test with Stock QEMU First
- Restore original UTM QEMU (if backup exists)
- Test streaming with stock QEMU to verify:
  - Desktop containers start
  - vsockenc connects to QEMU
  - Resource ID extraction works
- **Expected**: Will still crash on scanout resources, but proves the pipeline works

### Option 3: Fix vsockenc to Send Explicit Resource IDs
The real solution is guest-side: vsockenc must successfully extract DmaBuf resource IDs.

Current code (`desktop/gst-vsockenc/gstvsockenc.c:365-420`):
- Opens `/dev/dri/renderD128` or `/dev/dri/card0`
- Calls `DRM_IOCTL_PRIME_FD_TO_HANDLE` to get GEM handle
- Uses GEM handle as resource ID
- **Falls back to 0 if any step fails**

Check why extraction fails:
```bash
# Inside desktop container:
docker compose exec -T sandbox-macos docker logs {CONTAINER_NAME} 2>&1 | grep -E "resource_id|DMA-BUF|Failed to"
```

## Testing Plan (Once Build Issues Resolved)

1. **Start VM**:
   ```bash
   /Applications/UTM.app/Contents/MacOS/utmctl start 17DC4F96-F1A9-4B51-962B-03D85998E0E7
   ```

2. **Start Services** (inside VM):
   ```bash
   cd ~/helix
   ./stack start
   ```

3. **Create Session**:
   ```bash
   export PATH=$PATH:/usr/local/go/bin
   cd ~/helix/api && CGO_ENABLED=0 go build -o /tmp/helix .

   export HELIX_API_KEY=`grep HELIX_API_KEY ~/helix/.env.usercreds | cut -d= -f2-`
   export HELIX_URL=`grep HELIX_URL ~/helix/.env.usercreds | cut -d= -f2-`
   export HELIX_PROJECT=`grep HELIX_PROJECT ~/helix/.env.usercreds | cut -d= -f2-`

   /tmp/helix spectask start --project $HELIX_PROJECT -n "macOS ARM test"
   ```

4. **Test Streaming**:
   ```bash
   # Wait for GNOME to start
   sleep 15

   # Stream video
   /tmp/helix spectask stream ses_XXX --duration 30
   ```

5. **Check Logs**:
   ```bash
   # Host QEMU logs
   tail -100 "/Users/luke/Library/Group Containers/WDNLXAD4W8.com.utmapp.UTM/helix-debug.log"

   # Desktop container logs
   docker compose -f docker-compose.dev.yaml exec -T sandbox-macos docker logs {CONTAINER} 2>&1 | grep -E "vsockenc|resource_id|DMA-BUF"
   ```

## End-to-End Debugging (2026-02-06)

### Bugs Found and Fixed

#### 1. Stale TCP Connection Deadlock (FIXED)
**Problem**: When Docker containers are killed, the TCP connection through SLiRP doesn't get a proper FIN/RST. The QEMU accept thread's `recv()` blocks forever on the dead connection, preventing any new clients from connecting. This makes port 15937 unreachable even from the VM itself.

**Fix**: Added `SO_RCVTIMEO` (30s) and `SO_KEEPALIVE` to client sockets in `vsock_accept_thread()`. Increased listen backlog from 1 to 5. (commit 16ab341bf2 in qemu-utm)

#### 2. Missing SPS/PPS NAL Units (FIXED)
**Problem**: VideoToolbox stores SPS/PPS parameter sets in `CMFormatDescription`, NOT in the data buffer. The encoder output callback only converted the AVCC data buffer to Annex B, omitting SPS/PPS. Without SPS/PPS, `h264parse` cannot parse the H.264 stream and buffers indefinitely - resulting in 0 frames reaching appsink despite vsockenc successfully finishing frames.

**Fix**: Extract parameter sets using `CMVideoFormatDescriptionGetH264ParameterSetAtIndex()` and prepend them with Annex B start codes before the slice data for keyframes. (commit ca33601473 in qemu-utm)

#### 3. H.264 Profile Mismatch (FIXED earlier)
**Problem**: QEMU's VideoToolbox used Main profile, but pipeline caps filter required `constrained-baseline`.

**Fix**: Removed profile constraint from caps filter for vsockenc mode in `ws_stream.go`.

#### 4. vsockenc Threading Issue (FIXED earlier)
**Problem**: Original vsockenc used a `recv_thread` to read responses and call `finish_frame()` from a non-streaming C thread, which could cause issues with go-gst CGO callbacks.

**Fix**: Made vsockenc synchronous - `handle_frame()` now sends request AND reads response inline, calling `finish_frame()` from the GStreamer streaming thread.

#### 5. Wrong Screen Captured - VM Desktop Instead of Container (FIXED)
**Problem**: vsockenc sends SHM buffers with resource_id=0. QEMU's helix-frame-export read from the VM's DisplaySurface (scanout 0), which shows the VM's main desktop, not the container's screen. The container's actual pixel data from PipeWire ScreenCast was being thrown away.

**Fix**: Added `HELIX_FLAG_PIXEL_DATA` protocol extension. When resource_id=0, vsockenc maps the SHM buffer and sends the raw pixel data (8,294,400 bytes for 1920x1080 BGRA) after the frame request header. QEMU receives the pixel data, creates an IOSurface from it, and encodes that instead of reading from DisplaySurface. (commits 504d20a11f in qemu-utm, 0908db89d in helix)

#### 6. Pipeline Element Ordering Breaks PipeWire Frame Production (FIXED)
**Problem**: Adding `videoconvert ! videoscale ! capsfilter` AFTER the leaky queue caused PipeWire to stop producing frames after the initial 2. The pipeline would work for 2 frames then stall permanently, even with active screen damage.

**Root Cause**: When videoconvert/videoscale are placed after the leaky queue, PipeWire buffers are held through the entire videoconvert→videoscale→vsockenc(TCP send) chain. The extended hold time prevents PipeWire from recycling buffers, causing the ScreenCast source to stop producing frames.

**Fix**: Move videoconvert/videoscale BEFORE the leaky queue:
```
# BROKEN: pipewiresrc → queue → videoconvert → videoscale → vsockenc (2 frames then stall)
# FIXED:  pipewiresrc → videoconvert → videoscale → queue → vsockenc (26.5 FPS sustained)
```
PipeWire buffers are released immediately after the fast software scale (~1ms), and only the small 960x540 BGRA buffer enters the leaky queue. (commit d6ff0e538)

#### 7. Static Screen Stall - PipeWire Produces No Frames (FIXED)
**Problem**: PipeWire ScreenCast on GNOME/virtio-gpu is strictly damage-based. On a completely static desktop, the pipeline stalls permanently after an initial burst of 2-8 frames.

**Failed approaches**:
- `pipewiresrc keepalive-time=500`: Resends last buffer on timeout, but the PipeWire thread loop gets spurious wakeups from other ScreenCast sessions sharing the same connection, resetting the 500ms timer before it fires.
- GNOME Shell D-Bus Eval (St.Widget visibility toggle, color toggle, `queue_redraw()`): Clutter actor changes don't generate compositor-level (DRM/KMS) damage on virtio-gpu headless mode.
- Pipeline restart every 3 seconds: Workaround that gave 2.3 FPS but with constant keyframe resets.

**Root Cause**: cursor-mode=Metadata (2) means cursor movement only updates PipeWire metadata without producing new video frames. Combined with no screen changes = no frames.

**Fix**: Changed linked ScreenCast session to cursor-mode=Embedded (1), so cursor is composited into the video frame. Then the damage keepalive goroutine injects 1px right/left cursor jitter via `NotifyPointerMotion` on the RemoteDesktop D-Bus API every 500ms. Each cursor movement generates real compositor-level damage → PipeWire produces a new frame. (commit b0b3f0b85)

**Result**: 23.2 FPS sustained on completely static screens (695 frames / 30s). No pipeline restarts needed.

#### 8. Ghostty GL Context Exhaustion on virtio-gpu (KNOWN)
**Problem**: Launching a second ghostty terminal instance fails with "Unable to acquire an OpenGL context for rendering" on virtio-gpu. The limited number of GL contexts are consumed by the existing ghostty instance and GNOME's ScreenCast sessions.

**Workaround**: Use GNOME Shell D-Bus virtual keyboard to type into the existing terminal.

### Data Flow (Verified End-to-End)

```
PipeWire ScreenCast (container) → pipewiresrc (SHM buffers) ✅
  → videoconvert (format normalize) ✅
  → videoscale (1920x1080 → 960x540, 4x bandwidth reduction) ✅
  → queue (leaky=downstream, max 1 buffer) ✅
  → vsockenc (maps buffer, sends 2MB pixel data over TCP) ✅
    → TCP 10.0.2.2:15937 → QEMU SLiRP → host 127.0.0.1:15937 ✅
    → helix-frame-export receives pixel data ✅
    → Creates IOSurface from raw pixels (not DisplaySurface!) ✅
    → VideoToolbox H.264 encode at 960x540 ✅
    → SPS/PPS extraction + AVCC→Annex B conversion ✅
  → vsockenc finish_frame ✅
  → h264parse ✅
  → appsink ✅
  → Go callback → SharedVideoSource → WebSocket → browser ✅
```

### Current Status

- **VIDEO STREAMING WORKING** at 26.5 FPS with 960x540 downscale optimization
- 1591 frames / 60 seconds sustained with terminal activity (rock solid)
- Container screen is correctly captured (not VM desktop)
- All frames use pixel data path (HELIX_FLAG_PIXEL_DATA)
- SPS/PPS properly extracted from VideoToolbox CMFormatDescription
- Baseline profile, level 3.1, constraint_set3_flag=1 (zero-latency decode)
- ~30ms per frame round-trip (2MB send + VideoToolbox encode + response)
- 1.3 Mbps average bitrate, 5.8 KB average frame size

## Success Criteria

- ✅ VM starts without crashing
- ✅ Desktop container starts
- ✅ vsockenc sends resource_id=0 (SHM) + raw pixel data via HELIX_FLAG_PIXEL_DATA
- ✅ QEMU receives pixel data and encodes via VideoToolbox
- ✅ vsockenc receives encoded H.264 frames back
- ✅ h264parse parses stream (SPS/PPS properly included)
- ✅ Video streaming works without crashes (1591 frames / 60s test)
- ✅ 26.5 FPS sustained with terminal activity (downscale optimization)
- ✅ 23.2 FPS sustained on static screens (cursor-embedded keepalive, 695 frames / 30s)
- ⚠️ ghostty second instance fails on virtio-gpu (limited GL contexts)

## Performance History

| Date | FPS | Resolution | Bottleneck |
|------|-----|-----------|------------|
| 2026-02-05 | 8.7 | 1920x1080 | 8MB/frame over TCP/SLiRP |
| 2026-02-06 | 6.8 | 1920x1080 | Same bottleneck, confirmed |
| 2026-02-06 | 26.2 | 960x540 | videoconvert+videoscale before queue |
| 2026-02-06 | 23.2 | 960x540 | Static screen: cursor-embedded keepalive (SOLVED) |

### Downscale Optimization (IMPLEMENTED)

Reduced pixel data from 8MB/frame (1920x1080 BGRA) to 2MB/frame (960x540 BGRA):
- `videoconvert ! videoscale ! video/x-raw,format=BGRA,width=960,height=540` before the leaky queue
- **CRITICAL**: These elements MUST be placed BEFORE the leaky queue, not after it
- When placed after the queue, PipeWire buffers are held too long during the
  videoconvert/videoscale/vsockenc chain, causing PipeWire to stop producing frames
- When placed before the queue, PipeWire buffers are released quickly during the
  fast software scale (~1ms), and only the small 960x540 buffer enters the queue

### Further Optimization Options
1. ✅ **Downscale before sending**: 960x540 = 2MB/frame (4x less data) - DONE
2. **Use NV12 instead of BGRA**: 960x540 NV12 = 0.78MB vs 2MB BGRA (2.5x more reduction)
3. **Pre-compress with LZ4/zstd**: Raw pixels compress ~2-4x
4. **Use virtio-vsock instead of TCP**: Lower overhead than SLiRP user-mode networking
5. **DMA-BUF zero-copy path**: If virtio-gpu resource IDs can be resolved, skip pixel transfer entirely

## Performance Notes

**Intermittent VM Slowness** (reported by user):
- Fresh boot: Fast ✅
- After running: Sometimes slow ❌
- After reboot: Fast again ✅

**Possible Causes** (from web research):
- HVF is enabled (`-accel hvf`) ✅
- I/O performance can be slow on UTM [[1]](https://github.com/utmapp/UTM/discussions/2533)
- Resource accumulation requiring reboots
- Custom QEMU builds may not be optimized [[2]](https://geekyants.com/blog/advanced-qemu-options-on-macos-accelerate-arm64-virtualization)

## References

- [On MacBook Air M1 it is extremely slow](https://github.com/utmapp/UTM/discussions/2533)
- [Advanced QEMU Options on macOS](https://geekyants.com/blog/advanced-qemu-options-on-macos-accelerate-arm64-virtualization)
- [QEMU and HVF on Apple Silicon](https://gist.github.com/aserhat/91c1d5633d395d45dc8e5ab12c6b4767)
