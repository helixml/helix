# Ubuntu GNOME Resolution & MIT-SHM Debugging Guide

**Date:** 2025-12-08
**Status:** In Progress - Fundamental incompatibility identified

---

## Decision Summary (2025-12-08)

**Goal:** A solidly working Ubuntu desktop experience. The look and feel of Ubuntu is more important than using the latest Ubuntu version.

**Decision:** Choose ONE of these approaches:

### Option A: Ubuntu 22.04 + GNOME 42/43 (Recommended)
- Use Ubuntu 22.04 LTS as base
- GNOME 42/43 is proven stable with Xwayland/Gamescope
- Classic Ubuntu look - users will recognize it
- "Ubuntu 22.04" is a perfectly acceptable product
- Similar to what Zorin uses (based on Ubuntu 22.04)

### Option B: XFCE with Ubuntu Theming
- Use the existing GOW XFCE base image
- Apply Yaru theme (GTK theme, icons, cursors)
- Ubuntu fonts and wallpaper
- **Yes, XFCE can look like Ubuntu:**
  - Yaru-dark GTK theme works on XFCE
  - Yaru icon theme works on XFCE
  - Ubuntu fonts work on XFCE
  - Panel can be configured to match Ubuntu layout
- Faster, lighter, proven stable with Gamescope

### Recommendation
**Option A (Ubuntu 22.04 + GNOME 42/43)** is recommended because:
- Authentic GNOME experience
- Ubuntu 22.04 LTS is well-supported until 2027
- Users expect GNOME on Ubuntu
- Zorin's GOW image already proves this works

**Next Step:** Test with Zorin's base image or create Ubuntu 22.04 + GNOME image.

---

## Executive Summary

Ubuntu 25.04 with GNOME Shell 48 running on Xwayland under Gamescope has a fundamental incompatibility with MIT-SHM (X11 Shared Memory). This causes crashes when performing various window operations (dragging to edges, resizing, screenshots). The crashes also reset the display resolution from 1920x1080 to 5120x2880.

---

## Issues Discovered

### 1. Resolution Mismatch (PARTIALLY FIXED)

**Symptom:** Firefox and other apps maximize to 5120x2880 instead of 1920x1080, showing only a quarter of the window.

**Root Cause:**
- Gamescope expects 1920x1080 (set via `GAMESCOPE_WIDTH`/`GAMESCOPE_HEIGHT`)
- Xwayland starts with default resolution 5120x2880
- `xorg.sh` sets resolution via xrandr BEFORE GNOME starts
- Mutter (GNOME's compositor) resets resolution to its preferred mode (highest available)
- Applications see the wrong resolution and maximize incorrectly

**Attempted Fixes:**
1. ❌ xrandr in xorg.sh before GNOME - Mutter overrides it
2. ❌ xrandr autostart with 1 second delay - Mutter still overrides
3. ✅ monitors.xml configuration - Works initially but lost on crash
4. ⚠️ xrandr autostart with 5+3 second delay - Backup, but doesn't help after crash

**Current State:** Resolution is correct on initial boot if monitors.xml is read, but resets to 5120x2880 after any GNOME Shell crash.

### 2. MIT-SHM Crashes (UNRESOLVED)

**Symptom:** Screen goes gray, GNOME Shell crashes and restarts (or gives up).

**Error Message:**
```
(gnome-shell:362): Mtk-ERROR **: Received an X Window System error.
The error was 'BadMatch (invalid parameter attributes)'.
(Details: serial 6897 error_code 8 request_code 130 (MIT-SHM) minor_code 4)
gnome-session-binary[1]: WARNING: Application 'org.gnome.Shell.desktop' killed by signal 5
```

**Triggers:**
- Dragging windows to screen edges (left or right)
- Taking screenshots (via screenshot-server)
- Possibly other compositor operations

**Root Cause:**
MIT-SHM (X11 Shared Memory extension) allocates buffers for window compositing. When there's a mismatch between:
- The allocated buffer size
- The actual window geometry being rendered

A `BadMatch` error occurs and GNOME Shell crashes.

**Attempted Fixes:**
1. ❌ `NO_AT_BRIDGE=1` - Disable accessibility bridge
2. ❌ `MUTTER_DEBUG_DISABLE_HW_CURSORS=1` - Disable hardware cursors
3. ❌ `MUTTER_DEBUG_FORCE_FALLBACK=1` - Force software rendering
4. ❌ `GDK_DISABLE=vulkan` - Disable Vulkan in GTK
5. ⚠️ `edge-tiling=false` in dconf - Prevents edge-drag crashes but not other MIT-SHM crashes

**Current State:** Crashes still occur, especially during screenshot operations.

### 3. Wallpaper Not Showing (FIXED)

**Symptom:** Solid purple (#2C001E) instead of Ubuntu wallpaper.

**Root Cause:** Resolution mismatch caused wallpaper rendering to fail. At 5120x2880, the 3840x2160 wallpaper didn't render correctly.

**Fix:** Setting correct resolution via monitors.xml and delayed xrandr autostart.

### 4. inotify Errors (FIXED)

**Symptom:** "Too many open files (os error 24)" breaking Zed.

**Root Cause:** Host system's `max_user_instances` was 128, shared across all containers.

**Fix:**
```bash
sudo sysctl fs.inotify.max_user_instances=1024
```

---

## Files Modified

### wolf/ubuntu-config/startup-app.sh

Key changes:
- Added feature flags for incremental debugging
- Creates `~/.config/monitors.xml` for Mutter resolution config
- Creates resolution fix autostart entry with longer delay
- Sets Firefox as default browser
- Disables GNOME screensaver proxy

### wolf/ubuntu-config/dconf-settings.ini

Key changes:
- Added `[org/gnome/mutter]` section with `edge-tiling=false`
- Configured Ubuntu Yaru dark theme
- Set up keyboard shortcuts

### Dockerfile.ubuntu-helix

Key changes:
- Added `NO_AT_BRIDGE=1` environment variable in desktop.sh
- Various debugging attempts (most reverted)

---

## Debug Commands Reference

### Get Container ID
```bash
docker exec helix-sandbox-nvidia-1 docker ps --format '{{.ID}} {{.Image}}' | grep ubuntu
```

### Check Current Resolution
```bash
docker exec helix-sandbox-nvidia-1 docker exec -u retro <ID> \
  bash -c "export DISPLAY=:9 && xrandr"
```

### Check GNOME Shell Crashes
```bash
docker exec helix-sandbox-nvidia-1 docker logs <ID> 2>&1 | \
  grep -E "ERROR|killed|BadMatch|MIT-SHM|crash|respawn"
```

### Check monitors.xml
```bash
docker exec helix-sandbox-nvidia-1 docker exec <ID> \
  cat /home/retro/.config/monitors.xml
```

### Manually Fix Resolution
```bash
docker exec helix-sandbox-nvidia-1 docker exec -u retro <ID> \
  bash -c "export DISPLAY=:9 && xrandr --output XWAYLAND0 --mode 1920x1080"
```

### Check dconf Settings
```bash
docker exec helix-sandbox-nvidia-1 docker exec -u retro <ID> \
  bash -c "DISPLAY=:9 gsettings get org.gnome.mutter edge-tiling"
```

### Full Container Logs
```bash
docker exec helix-sandbox-nvidia-1 docker logs <ID> 2>&1 | tail -100
```

---

## Alternative Approaches to Consider

### 1. Use XFCE Instead of GNOME

The base image (`ghcr.io/games-on-whales/xfce:edge`) already includes XFCE which works well with Gamescope. We could:
- Remove GNOME installation
- Configure XFCE with Ubuntu-like theming (Yaru theme, similar fonts)
- Avoid all MIT-SHM issues since XFCE uses different compositing

**Pros:** Proven to work, lighter weight, faster startup
**Cons:** Different UI from Ubuntu, some users may expect GNOME

### 2. Use Zorin's Older GNOME

Zorin uses `ghcr.io/mollomm1/gow-zorin-18:latest` based on Ubuntu 22.04 with GNOME 42/43. This older GNOME may not have the MIT-SHM issues.

**Pros:** Still GNOME, familiar UI
**Cons:** Older software, may have other issues

### 3. Use Ubuntu 24.04 LTS Base

Instead of Ubuntu 25.04 (Plucky Puffin), use Ubuntu 24.04 LTS (Noble Numbat) which has GNOME 46, not 48.

**Pros:** LTS stability, slightly older GNOME
**Cons:** May still have MIT-SHM issues (needs testing)

### 4. Native Wayland Mode

Run GNOME on Wayland directly instead of Xwayland. Gamescope can potentially host Wayland clients.

**Pros:** Avoid all X11/Xwayland issues
**Cons:** May not work with Gamescope, untested, complex

### 5. Patch Mutter/GNOME Shell

Disable MIT-SHM at the Mutter level through source patches.

**Pros:** Fix the actual problem
**Cons:** Requires maintaining patches, complex, slow iteration

---

## Next Steps

1. **Test Zorin container** - Verify if the same MIT-SHM crashes occur with older GNOME
2. **Test XFCE styling** - See if XFCE can be themed to look Ubuntu-like
3. **Research MIT-SHM disable** - Check if there's an environment variable or config to disable MIT-SHM in Mutter
4. **Consider Ubuntu 24.04** - Test if GNOME 46 has the same issues

---

## Technical Deep Dive: MIT-SHM

### What is MIT-SHM?

MIT-SHM (X11 Shared Memory Extension) allows X clients to share memory with the X server for efficient image transfer. Instead of copying pixel data over the X protocol, clients can:

1. Allocate shared memory segment
2. Draw into shared memory
3. Tell X server to use that memory directly

### Why Does It Crash?

The `BadMatch` error (error_code 8) occurs when:
- The shared memory segment size doesn't match the expected image size
- The pixel format/depth doesn't match
- The geometry (width/height) is invalid

In GNOME Shell 48 on Xwayland:
- Mutter allocates SHM buffers based on its understanding of screen geometry
- When resolution changes or window geometry doesn't match expectations
- The SHM operations fail with BadMatch

### Relevant Code Paths

The error comes from `request_code 130` which is the MIT-SHM extension, `minor_code 4` which is `ShmPutImage` or similar operation.

The crash happens in Mutter's X11 compositor backend when it tries to composite windows using shared memory.

---

## Commits Made

| Commit | Description |
|--------|-------------|
| a449f3ade | Added resolution fix autostart entry |
| 819f8753a | Added NO_AT_BRIDGE and MUTTER_DEBUG_DISABLE_HW_CURSORS |
| 49ffffb02 | Added MUTTER_DEBUG_FORCE_FALLBACK |
| 263a2c391 | Disabled edge-tiling in dconf |
| e3298c134 | Added monitors.xml for proper Mutter config |

---

## Environment Information

- **Host OS:** Ubuntu 24.04 (Linux 6.8.0-86-generic)
- **Container OS:** Ubuntu 25.04 (Plucky Puffin)
- **GNOME Shell:** 48.0
- **Gamescope:** 3.15.14
- **GPU:** NVIDIA RTX 4000 SFF Ada Generation
- **Target Resolution:** 1920x1080
- **Xwayland Framebuffer:** 5120x2880

---

## Conclusion

The MIT-SHM crashes are a fundamental incompatibility between GNOME Shell 48 and the Xwayland/Gamescope environment. While we can work around some triggers (like edge-tiling), the core issue remains.

The most practical solutions are:
1. **Switch to XFCE** - Already in base image, known to work
2. **Use older GNOME** - Via Zorin or Ubuntu 24.04 base
3. **Wait for upstream fixes** - GNOME 49 may address these issues

For production use, XFCE with Ubuntu theming is recommended until the GNOME/Xwayland compatibility improves.
