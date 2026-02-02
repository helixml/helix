# Intel GPU Video Freeze Fix

## Problem
On Intel GPU systems configured with `GPU_VENDOR=none` (software rendering), video streaming would freeze after the first frame. The GNOME ScreenCast source would go into "idle" state and stop producing frames.

## Root Cause
The `.env` file had `GPU_VENDOR=none` which disabled GPU acceleration and forced software rendering with OpenH264. The `pipewiresrc` element's `keepalive-time=500` property was not producing keepalive frames as expected, causing the pipeline to stall after the first frame.

## Fix
Enable Intel GPU hardware acceleration in `.env`:

```bash
GPU_VENDOR=intel
HELIX_RENDER_NODE=/dev/dri/renderD128
LIBVA_DRIVER_NAME=iHD
```

This enables:
- Intel VA-API hardware encoding (`vah264enc` or `qsvh264enc`)
- GPU-accelerated video processing with `vapostproc`
- Direct access to Intel GPU via `/dev/dri/renderD128`

## Verification Steps

1. **Check `.env` configuration:**
   ```bash
   grep GPU_VENDOR .env
   # Should show: GPU_VENDOR=intel
   ```

2. **Restart sandbox:**
   ```bash
   docker compose -f docker-compose.dev.yaml down sandbox-software
   docker compose -f docker-compose.dev.yaml up -d sandbox-software
   ```

3. **Start a session and check logs:**
   ```bash
   # Start a session via the UI
   # Then check desktop bridge logs for encoder:
   docker compose -f docker-compose.dev.yaml exec -T sandbox-software docker ps --format "{{.Names}}" | head -1 | xargs -I {} docker compose -f docker-compose.dev.yaml exec -T sandbox-software docker logs {} 2>&1 | grep encoder
   ```

   You should see:
   - `using VA-API encoder` or `using Intel QSV encoder` (NOT `using OpenH264 software encoder`)
   - `Broadcast frame` messages appearing regularly (not just once)

4. **Monitor frame broadcast:**
   ```bash
   # In another terminal, watch for broadcast messages:
   docker compose -f docker-compose.dev.yaml exec -T sandbox-software docker ps -q | head -1 | xargs -I {} docker compose -f docker-compose.dev.yaml exec -T sandbox-software docker logs -f {} 2>&1 | grep "Broadcast frame"
   ```

   Frames should appear continuously at ~60 FPS (or 10 FPS on static screens with keepalive).

## Technical Details

### Before (Software Rendering)
- Encoder: OpenH264 (CPU software encoder)
- Pipeline: `pipewiresrc → queue → videoconvert → openh264enc`
- Issue: `keepalive-time=500` not working, pipeline stalls after 1 frame
- FPS: 0 FPS on static screens (frozen)

### After (Intel GPU)
- Encoder: VA-API (`vah264enc`) or Intel QSV (`qsvh264enc`)
- Pipeline: `pipewiresrc → queue → vapostproc → vah264enc`
- GPU: `/dev/dri/renderD128` (Intel integrated graphics)
- FPS: 60 FPS with active content, 10 FPS on static screens (keepalive working)

## Alternative: Fix Software Rendering Path

If you need to use software rendering, the `keepalive-time` issue could be fixed by:
1. Adding a separate keepalive timer in Go that triggers fake damage events
2. Using a videotestsrc overlay at 1 FPS to keep the pipeline active
3. Upgrading gst-plugin-pipewire to a version where `keepalive-time` works correctly

However, hardware encoding is strongly recommended for performance.
