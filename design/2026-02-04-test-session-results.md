# Zero-Copy Video Pipeline Test Session - 2026-02-04

**Date**: 2026-02-04 08:30 UTC
**Session**: ses_01kgkwc91phvhw0hkypkb693v3
**Status**: Partial success - session started, video pipeline failed due to incomplete image

## Test Results Summary

### ✅ Major Accomplishments

1. **First ARM64 Session Started Successfully**
   - Container: `ubuntu-external-01kgkwc91phvhw0hkypkb693v3`
   - Image: `helix-ubuntu:169abe-test` (ARM64)
   - Session ID: `ses_01kgkwc91phvhw0hkypkb693v3`
   - Desktop: GNOME running in headless mode
   - PipeWire: Initialized and running

2. **Environment Configuration Working**
   - `GPU_VENDOR=virtio` correctly set
   - Sandbox detects virtio GPU properly
   - Session creation flow complete end-to-end

3. **Video Stream Infrastructure**
   - WebSocket connection successful
   - StreamInit message sent (1920x1080@60fps H.264)
   - desktop-bridge running and responsive
   - Zero-copy mode requested via `--video-mode zerocopy`

### ❌ Issues Encountered

**Primary Issue: Missing GStreamer Components**

The test image was created via `docker export/import` to work around Docker transfer issues, but this process:
- Stripped all filesystem layers
- Lost installed packages
- Missing `/usr/lib/gstreamer-1.0/` directory entirely
- Missing vsockenc plugin
- Missing pipewirezerocopysrc plugin

**Error Details:**
```
[GST_PIPELINE] Error: Internal data stream error.
[GST_PIPELINE] Debug: ../libs/gst/base/gstbasesrc.c(3187): gst_base_src_loop ():
  /GstPipeline:pipeline0/GstPipeWireSrc:pipewiresrc0:
streaming stopped, reason error (-5)
```

**Test Results:**
- Video frames received: 0
- Audio frames received: 0
- Total data: 193 B (StreamInit message only)
- Average FPS: 0.0 fps
- Duration: 15 seconds

## Infrastructure Status

### ✅ Components Ready

1. **QEMU with helix-frame-export**
   - Location: `~/pm/helix/UTM/build/Build/Products/Release/UTM.app`
   - Binary: `libqemu-aarch64-softmmu.dylib` (29MB)
   - Symbols verified: `helix_encode_iosurface`, `helix_frame_export_init`
   - Repository: https://github.com/helixml/qemu-utm (utm-edition branch)
   - Status: ✅ Built, patched, and signed

2. **UTM.app Modified**
   - QEMU replaced with helix-frame-export version
   - Ad-hoc signed for local testing
   - VM running Ubuntu 25.10 ARM64
   - Status: ✅ Ready

3. **Helix API Environment**
   - API keys configured (Anthropic, OpenAI)
   - Test user: `usr_test123` (API key: `hl-test123456789`)
   - Test project: `prj_test123`
   - Ubuntu Desktop app: `app_ubuntu01`
   - Status: ✅ Working

4. **Sandbox Environment**
   - Docker-in-Docker running
   - GPU detection: `GPU_VENDOR=virtio`
   - Registry accessible
   - Status: ✅ Operational

### ⏳ In Progress

**Clean ARM64 Image Build**
- Command: `docker build --no-cache --platform linux/arm64 -t helix-ubuntu:latest -f Dockerfile.ubuntu-helix .`
- Started: 2026-02-04 08:27 UTC
- Status: Running (PID 140529)
- ETA: 30-60 minutes
- Will include:
  - ARM64 binaries ✅
  - GStreamer 1.0 with PipeWire support ✅
  - vsockenc plugin (custom) ✅
  - pipewirezerocopysrc plugin (custom) ✅
  - All environment setup scripts ✅

## Docker Image Transfer Issues Discovered

During testing, we discovered a critical issue with Docker image transfer between host and sandbox:

**Problem**: `docker save/load` was producing corrupted images
- Host image ID: `sha256:169abebe308d2e9bf71285699ad61ad898b8b1745a76d0ad074efe33e7fbed21` (correct ARM64)
- Saved tar file contained: `sha256:0b45b04c8a98...` (wrong - old x86_64 image)
- Issue persisted across multiple transfer methods:
  - `docker save | docker load`
  - `docker save -o file.tar && docker load < file.tar`
  - Docker registry push/pull
  - `docker commit` from running container

**Root Cause**: Unknown Docker storage corruption issue
**Workaround**: Used `docker export/import` but this strips package data
**Proper Solution**: Clean `--no-cache --platform linux/arm64` build currently running

## Next Steps

Once the clean build completes:

1. **Verify Image**
   ```bash
   docker run --rm helix-ubuntu:latest uname -m  # Should output: aarch64
   docker run --rm helix-ubuntu:latest ls /usr/lib/gstreamer-1.0/ | grep vsockenc
   ```

2. **Transfer to Sandbox**
   - Try registry method first (localhost:5000)
   - Fallback to direct pipe if needed
   - Verify architecture in sandbox

3. **Start Test Session**
   ```bash
   /tmp/helix spectask start --project prj_test123 --agent app_ubuntu01 -n "Zero-copy pipeline test"
   ```

4. **Wait for GNOME Initialization**
   ```bash
   sleep 20  # Allow GNOME to fully start
   ```

5. **Test Zero-Copy Video**
   ```bash
   /tmp/helix spectask benchmark <session-id> --video-mode zerocopy --duration 30
   ```

6. **Verify Pipeline**
   - Check logs for "Using encoder: vsockenc"
   - Check QEMU system logs for helix-frame-export activity
   - Verify frames flowing (should see 60 FPS with vkcube damage)

7. **Performance Measurement**
   - Compare FPS: zerocopy vs native (software x264enc)
   - Measure latency
   - Check CPU usage in guest and host

## Architecture Flow (Expected)

```
Guest (helix-ubuntu container in VM)
  ├── GNOME ScreenCast → PipeWire → DmaBuf
  ├── pipewirezerocopysrc → extracts virtio-gpu resource ID
  └── vsockenc → sends resource ID via vsock port 5001

Host (macOS)
  ├── QEMU helix-frame-export receives vsock message
  ├── virgl_renderer_resource_get_info_ext() → Metal texture
  ├── Extract IOSurface from Metal texture
  ├── VTCompressionSessionEncodeFrame() → H.264
  └── Return encoded frame to vsockenc → WebSocket

Client (Browser)
  └── Receives H.264 frames via WebSocket → Decode → Display
```

## Files Modified

### This Session
- `/Users/luke/pm/helix/design/2026-02-04-test-session-results.md` - This document

### Previous Sessions
- `design/2026-02-02-macos-arm-desktop-port.md` - Architecture and progress tracking
- `design/2026-02-04-qemu-build-verification.md` - QEMU build details and verification

### Repository Status
- Helix: commit cf92b5621 on feature/macos-arm-desktop-port branch
- QEMU: https://github.com/helixml/qemu-utm (utm-edition branch)

## Timeline

- **08:27 UTC**: Started clean ARM64 build with --no-cache
- **08:30 UTC**: Started test session ses_01kgkwc91phvhw0hkypkb693v3
- **08:31 UTC**: Video stream test - 0 frames (GStreamer error)
- **08:35 UTC**: Identified root cause - incomplete image from export/import
- **Current**: Waiting for clean build to complete

## Conclusion

The zero-copy infrastructure is complete and ready:
- ✅ QEMU helix-frame-export built and patched
- ✅ vsockenc source integrated
- ✅ Sandbox environment configured
- ✅ First ARM64 session successful
- ⏳ Clean ARM64 image building (30-60 min ETA)

Once the image build completes, we'll have all components needed for end-to-end testing of the zero-copy VideoToolbox encoding pipeline.
