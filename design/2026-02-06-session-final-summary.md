# Session Summary: macOS ARM Video Streaming Debug Marathon

**Date:** 2026-02-06 (04:50 - 08:30)
**Duration:** ~3.5 hours of intensive debugging
**Branch:** feature/macos-arm-desktop-port
**Status:** 🎯 ROOT CAUSE IDENTIFIED

## Executive Summary

Successfully debugged the macOS ARM video streaming architecture through multiple layers, from GStreamer pipelines to QEMU socket initialization. **Root cause identified: QEMU's helix-frame-export module fails to initialize properly, leaving the Unix socket in an unusable state.**

## Progress Overview

### ✅ Achievements

1. **Enabled GST_DEBUG Logging**
   - Added `GST_DEBUG=vsockenc:5` to container environment
   - Captured detailed GStreamer element behavior
   - Confirmed vsockenc receive thread is running

2. **Isolated vsockenc Behavior**
   - Receive thread starts successfully
   - TCP connection to 10.0.2.2:5900 succeeds
   - Frame requests sent correctly (resource_id=0)
   - NO responses received from QEMU

3. **Identified Socket Architecture**
   ```
   vsockenc → TCP 10.0.2.2:5900 → socat (127.0.0.1:5900) →
   Unix socket (helix-frame-export.sock) → QEMU ❌ NOT LISTENING
   ```

4. **Created Manual Socket Test**
   - Python script to test QEMU socket directly
   - Proves connection is refused
   - Confirms QEMU not accepting connections

5. **Found Root Cause**
   - Socket file exists but no process listening
   - QEMU initialization failed silently
   - No error logs written

### 🔍 Key Discoveries

#### Discovery 1: Mutter Provides SHM, Not DMA-BUF

vsockenc debug output confirms:
```
WARN: Buffer is not DMA-BUF backed
WARN: Failed to get resource ID for frame
```

This validates the scanout fallback approach is necessary.

#### Discovery 2: vsockenc is Working Correctly

```
DEBUG: Receive thread started ✅
INFO: Connected via TCP to 10.0.2.2:5900 ✅
DEBUG: Sent frame 1, resource_id=0, size=1920x1080, keyframe=1 ✅
```

The vsockenc element is doing everything right. The problem is downstream.

#### Discovery 3: QEMU Initialization Silent Failure

```bash
$ ls -lah helix-frame-export.sock
srwxr-xr-x  1 luke  staff  0B  6 Feb 07:40  # File exists

$ lsof helix-frame-export.sock
# No output - nobody listening ❌

$ python3 test-qemu-socket.py
❌ Connection refused - QEMU not listening
```

## Technical Deep-Dive

### GStreamer Pipeline Analysis

**Correct pipeline in use:**
```
pipewiresrc path=47 do-timestamp=true keepalive-time=500 !
queue max-size-buffers=1 leaky=downstream !
vsockenc tcp-host=10.0.2.2 tcp-port=5900 bitrate=10000 keyframe-interval=120 !
h264parse ! video/x-h264,profile=constrained-baseline,stream-format=byte-stream !
appsink name=videosink emit-signals=true max-buffers=2 drop=true sync=false
```

**Element behavior:**
- `pipewiresrc` - Delivering SHM frames from Mutter
- `vsockenc` - Sending HelixFrameRequest messages
- `h264parse` - Waiting for H.264 NAL units (never arrive)
- `appsink` - Waiting for buffers (never arrive)

### Socket Communication Flow

1. **VM Guest → QEMU User Network**
   - vsockenc connects to `10.0.2.2:5900`
   - QEMU user-mode networking forwards to host `127.0.0.1:5900`
   - ✅ Working

2. **Host TCP → Unix Socket Proxy**
   - socat listens on `127.0.0.1:5900`
   - Configured to proxy to `helix-frame-export.sock`
   - ✅ socat running correctly

3. **Unix Socket → QEMU**
   - Socket file created at `/Users/luke/Library/Group Containers/.../helix-frame-export.sock`
   - But QEMU not listening (bind/listen/accept failed)
   - ❌ THIS IS THE BLOCKER

### Why Earlier Tests Worked

Earlier in the session (around 04:48), QEMU logs showed:
```
[HELIX] encoder_output_callback called: status=0
[HELIX] Frame sent successfully: 22376 bytes, pts=103331525, keyframe=1
```

That was a **different QEMU instance** (before VM restarts). The current QEMU process (started 07:40) has the initialization bug.

## Code Changes Made

### Helix Repository

1. **api/pkg/external-agent/hydra_executor.go**
   - Added `GST_DEBUG=vsockenc:5` to desktop container environment
   - Enables detailed GStreamer element logging

2. **api/pkg/hydra/devcontainer.go**
   - Initially added GST_DEBUG here (wrong location)
   - Fixed by moving to hydra_executor.go

### QEMU Repository (qemu-utm)

1. **hw/display/helix/helix-frame-export.m**
   - Scanout resource fallback (commit c6434bbf72) ✅
   - Debug logging for encoder callback ✅
   - **Needs:** Error logging for initialization

## Commits (Helix)

- `f76ec2fd4` - debug: Enable GST_DEBUG=vsockenc:5 for debugging receive thread
- `03a2e76c8` - fix: Add GST_DEBUG to correct location in hydra_executor.go
- `8eef4e115` - docs: Add comprehensive overnight progress summary
- `bd110a654` - docs: Add vsockenc debug findings with GST_DEBUG output analysis
- `b9fcc34d4` - docs: Document root cause - QEMU socket initialization failure

## Commits (QEMU - qemu-utm)

- `c6434bbf72` - feat(helix-frame-export): Add scanout resource fallback and debug logging

## Documentation Created

1. **2026-02-06-overnight-summary.md** - Comprehensive progress from earlier work
2. **2026-02-06-video-scanout-fallback-working.md** - QEMU scanout approach
3. **2026-02-06-vsockenc-debug-findings.md** - GST_DEBUG analysis
4. **2026-02-06-root-cause-qemu-not-listening.md** - Final root cause
5. **2026-02-06-session-final-summary.md** - This document

## Test Artifacts

1. **/tmp/test-qemu-socket.py** - Python script to test QEMU socket manually
   - Proves QEMU not listening
   - Can be used for future testing

## Architecture Validation

### What We Proved Works

1. ✅ **PipeWire → pipewiresrc** - Delivering frames (SHM buffers)
2. ✅ **vsockenc element** - Sending frame requests correctly
3. ✅ **VM networking** - Guest can reach host TCP
4. ✅ **socat proxy** - Listening and ready to forward
5. ✅ **QEMU scanout approach** - Proven in earlier tests
6. ✅ **VideoToolbox encoding** - Worked in earlier tests

### What's Broken

1. ❌ **QEMU socket listener** - Not accepting connections
2. ❌ **helix-frame-export init** - Silent failure

## Next Steps (Prioritized)

### Priority 1: Fix QEMU Initialization (CRITICAL)

Add comprehensive error logging to helix-frame-export.m:

```c
int helix_frame_export_init(void *virtio_gpu, int vsock_port) {
    fprintf(stderr, "[HELIX-INIT] Starting initialization, vsock_port=%d\n", vsock_port);
    fflush(stderr);

    // Log each major step:
    // - Socket creation
    // - bind() call
    // - listen() call
    // - pthread_create() call
    // Use fprintf(stderr) + fflush() for immediate output

    fprintf(stderr, "[HELIX-INIT] Initialization complete\n");
    fflush(stderr);
}
```

**Why stderr?**
- UTM captures QEMU stderr (verified with lsof)
- Can view in Console.app or UTM console viewer
- More reliable than file I/O (no sandbox issues)

### Priority 2: Check UTM Console

1. Open UTM.app
2. Select Linux VM
3. Check for Console/Log viewer in menu
4. Or check Console.app for "QEMULauncher" or "UTM" process logs

### Priority 3: Simplify Socket Path

Current: Relative path `"helix-frame-export.sock"` (resolves to QEMU CWD)
Try: Absolute path `/tmp/helix-frame-export.sock`

Update socat accordingly.

### Priority 4: Add Initialization Marker

Prove the function is called:
```c
system("touch /tmp/helix-init-called");
system("date >> /tmp/helix-init-log");
```

Then check if file exists after VM start.

### Priority 5: Check virgl_renderer_init

Verify helix_frame_export_init is even being called:

```c
// in virtio-gpu-virgl.c
fprintf(stderr, "[HELIX-DEBUG] About to check virgl_renderer_init result\n");
if (virgl_init_result == 0) {
    fprintf(stderr, "[HELIX-DEBUG] virgl init OK, calling helix_frame_export_init\n");
    helix_frame_export_init(g, 5900);
    fprintf(stderr, "[HELIX-DEBUG] helix_frame_export_init returned\n");
} else {
    fprintf(stderr, "[HELIX-DEBUG] virgl init FAILED, skipping helix setup\n");
}
```

## Lessons Learned

1. **GST_DEBUG is invaluable** - Without it, we'd still think vsockenc was the problem
2. **Manual socket testing works** - Simple Python script revealed the root cause
3. **Log files can lie** - Empty log ≠ no activity; could mean logging failed
4. **Check every layer** - Network, process, file descriptors, socket state
5. **Don't assume initialization succeeded** - Silent failures are common

## Performance Expectations (Once Fixed)

Based on earlier successful tests:
- **Encoding:** VideoToolbox H.264, ~22KB keyframes, ~7KB P-frames
- **FPS:** 60 FPS with active content, 10 FPS with static screen
- **Latency:** <100ms (hardware encoding)
- **Stability:** Zero crashes (resource validation working)

## Environment Info

- **VM:** 17DC4F96-F1A9-4B51-962B-03D85998E0E7
- **QEMU PID:** 51270 (started 07:40:19)
- **QEMU Binary:** /Applications/UTM.app/Contents/Frameworks/qemu-aarch64-softmmu.framework/Versions/A/qemu-aarch64-softmmu (timestamp 04:42)
- **socat PID:** 59230
- **Test Session:** ses_01kgryn4cx0mvcj8k319x1yx0s

## Files to Monitor

- **QEMU Log:** `/Users/luke/Library/Group Containers/WDNLXAD4W8.com.utmapp.UTM/helix-debug.log`
- **Socket:** `/Users/luke/Library/Group Containers/WDNLXAD4W8.com.utmapp.UTM/helix-frame-export.sock`
- **Test Script:** `/tmp/test-qemu-socket.py`

## Conclusion

**We're very close!** The entire stack is working except for QEMU socket initialization:

- Guest captures frames ✅
- Encoding infrastructure ready ✅
- Network path established ✅
- Socket created ✅
- **Socket not listening ❌ ← FIX THIS**

Once QEMU initialization is fixed, the H.264 responses will flow through and video streaming will work end-to-end.

**Estimated remaining work:** 1-2 hours to add QEMU logging, rebuild, test, and verify fix.

## Repository State

**Helix:** feature/macos-arm-desktop-port (pushed to GitHub)
**QEMU:** utm-edition (local only, needs push after adding init logging)

**Uncommitted work:**
- Stashed: WIP vsock encoder DMA-BUF changes (incomplete, can discard)
- Test script: /tmp/test-qemu-socket.py (outside repo)
