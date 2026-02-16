# macOS ARM Video Streaming - Comprehensive Status

**Date:** 2026-02-05 22:35
**Branch:** feature/macos-arm-desktop-port (32 commits ahead)

## Summary

Successfully built ARM64 support for Helix on macOS and debugged video streaming architecture. Fixed GPU device mounting and pipeline configuration. Identified final blocker: vsockenc resource ID extraction from DMA-BUF.

## What's Working

1. ✅ **VM Stability** - Runs without crashes, QEMU handles resource validation correctly
2. ✅ **GPU Devices** - Containers have /dev/dri/card0 and /dev/dri/renderD128 mounted
3. ✅ **Pipeline** - Correct configuration: pipewiresrc (no always-copy) → vsockenc → QEMU
4. ✅ **Network** - vsockenc connects to QEMU via TCP 10.0.2.2:5900 (socat proxy)
5. ✅ **QEMU Ready** - Receives connections, ready to encode with VideoToolbox

## Current Blocker

**vsockenc sends resource_id=0 instead of extracting GEM handle from DMA-BUF**

QEMU logs show:
```
[HELIX] Guest connected!
[HELIX] Frame request RAW: resource_id=0
[HELIX] resource_id=0 not supported - guest must provide explicit resource ID
```

This means vsockenc's DMA-BUF extraction code is failing at line 365-419 in gst-vsockenc.c.

## Root Cause Analysis

The vsockenc element should:
1. Receive DMA-BUF buffer from pipewiresrc
2. Extract file descriptor from GstDmaBufMemory
3. Call DRM_IOCTL_PRIME_FD_TO_HANDLE to get GEM handle
4. Use GEM handle as resource ID

It's failing and returning 0. Possible reasons:
- PipeWire negotiates SHM instead of DMA-BUF despite no always-copy
- DRM ioctl fails (permissions, wrong device, unsupported by virtio-gpu)
- Static screen produces no frames (damage-based ScreenCast)

## Key Commits

1. **b0599449d** - Fix Hydra GPU device mounting for virtio-gpu
2. **b7fcff40f** - Enable DMA-BUF for vsockenc (initial attempt, used pipewirezerocopysrc)
3. **0f1410d72** - Fix to use native pipewiresrc instead (pipewirezerocopysrc not on ARM64)

## Next Debugging Steps

### 1. Enable GStreamer Debug Logging
```bash
# Add to container environment
GST_DEBUG=vsockenc:5,pipewiresrc:5

# Look for vsockenc warnings:
# "Buffer is not DMA-BUF backed"
# "Failed to get GEM handle from DMA-BUF"
```

### 2. Test with Screen Activity
```bash
# Static screens don't generate damage - test with active content
docker exec ubuntu-xxx env DISPLAY=:0 vkcube &
# Then stream while vkcube is running
```

### 3. Verify PipeWire Buffer Type
```bash
# Check what buffer type PipeWire actually negotiated
pw-dump | jq '.[] | select(.id == 47)'
# Look for DmaBuf vs MemPtr/MemFd
```

### 4. Test DRM Ioctl Directly
```bash
# Inside container, test if DRM ioctl works
python3 << EOF
import fcntl, struct
fd = open('/dev/dri/renderD128', 'r')
DRM_IOCTL_PRIME_FD_TO_HANDLE = 0xc00c642e
# Test if ioctl is supported
EOF
```

## Architecture Reference

```
┌─────────────────────────────────────────────┐
│ Guest (Ubuntu VM)                           │
├─────────────────────────────────────────────┤
│ GNOME ScreenCast → PipeWire DMA-BUF         │
│         ↓                                   │
│ pipewiresrc (no always-copy)                │
│         ↓                                   │
│ vsockenc extracts resource ID from DMA-BUF  │
│         ↓                                   │
│ TCP 10.0.2.2:5900 (send resource ID)        │
└─────────────────────────────────────────────┘
              ↓
┌─────────────────────────────────────────────┐
│ Host (macOS)                                │
├─────────────────────────────────────────────┤
│ socat: TCP 5900 → UNIX socket               │
│         ↓                                   │
│ QEMU helix-frame-export                     │
│  - Receive resource ID                      │
│  - virgl_renderer_transfer_read_iov()       │
│  - Read GPU framebuffer                     │
│  - VideoToolbox H.264 encode                │
│  - Send to API via callback                 │
└─────────────────────────────────────────────┘
```

## Files Modified

- `api/pkg/hydra/devcontainer.go` - GPU device mounting
- `api/pkg/desktop/ws_stream.go` - Pipeline selection
- `haystack_service/pyproject.toml` - ARM64 platform markers
- `Dockerfile.sway-helix` - ARM64 CUDA
- `stack` - Auto-transfer images, code-macos
- `docker-compose.dev.yaml` - Remove hardcoded IPs
- `qemu-utm/hw/display/helix/helix-frame-export.m` - Resource validation
- `scripts/fix-qemu-paths-recursive.sh` - Library path fixes

## Performance Expectations

Once resource ID extraction works:
- 60 FPS with active content (vkcube, terminal output)
- 10 FPS with static screen (damage-based keepalive timer)
- <100ms latency (hardware encoding)
- Zero crashes (validated resource handling)
