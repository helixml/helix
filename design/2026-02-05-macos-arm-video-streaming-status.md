# macOS ARM Video Streaming - Status Update

**Date:** 2026-02-05 17:45
**Status:** Build system issues preventing testing

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

## Current Blockers

### ❌ QEMU Build System Issues

**Problem**: Custom QEMU builds don't install correctly into UTM.app

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
4. ❌ Library paths still broken → VM won't start

## Next Steps

### Option 1: Fix Library Paths (Recommended)
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

## Success Criteria

- ✅ VM starts without crashing
- ✅ Desktop container starts
- ✅ vsockenc extracts DmaBuf resource IDs (not 0)
- ✅ QEMU receives explicit resource IDs
- ✅ QEMU rejects resource_id=0 with error message
- ✅ Video streaming works without crashes
- ✅ 30-60 FPS with active content (vkcube)

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
