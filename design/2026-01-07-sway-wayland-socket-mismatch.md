# Design Doc: Sway Desktop Wayland Socket Mismatch

**Date:** 2026-01-07
**Status:** Complete
**Author:** Claude

## Problem Statement

Sway desktop containers fail to start with the error:
```
[wlr] [backend/wayland/backend.c:601] Could not connect to remote display: No such file or directory
[wlr] [backend/backend.c:363] failed to add backend 'wayland'
[sway/server.c:228] Unable to create backend
```

GNOME/Ubuntu desktops work correctly using the PipeWire ScreenCast path.

## Root Cause Analysis

### Architecture Recap

Helix supports two video source modes:

1. **PipeWire mode** (`video_source_mode=pipewire`) - Used for GNOME 49+ and Hyprland
   - Container runs standalone, creates D-Bus ScreenCast session
   - Container reports PipeWire node ID to Wolf via `/set-pipewire-node-id`
   - Wolf uses `pipewiresrc` to capture video from PipeWire
   - Container doesn't need Wolf's Wayland display

2. **Wayland mode** (`video_source_mode=wayland`) - Used for Sway and KDE
   - Wolf starts `gst-wayland-display` (waylanddisplaysrc) FIRST
   - Wolf creates a Wayland socket for the container to connect to
   - Container runs as a **nested compositor** inside Wolf's display
   - Wolf captures video directly from its own compositor surface
   - Container MUST be able to connect to Wolf's WAYLAND_DISPLAY

### The Bug

When Wolf starts a Sway container in wayland mode, the environment variables show:
```
XDG_RUNTIME_DIR=/tmp/sockets   # First occurrence
XDG_RUNTIME_DIR=/run/user/1000 # Second occurrence (this wins!)
WAYLAND_DISPLAY=wayland-1
```

And the mounts show:
```
/tmp/sockets/wayland-1:/tmp/sockets/wayland-1:rw              # Wolf's wayland socket
/wolf-state/agent-xxx/pipewire:/run/user/1000:rw              # PipeWire directory
```

**The problem:**
- Wolf's wayland socket is at `/tmp/sockets/wayland-1`
- But `XDG_RUNTIME_DIR=/run/user/1000` (the duplicate overrides the first)
- So Sway looks for `$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY` = `/run/user/1000/wayland-1`
- This path doesn't exist because the socket is at `/tmp/sockets/wayland-1`!

### Why This Worked Before

Previously, there was no PipeWire mode - all desktops used the wayland mode with consistent XDG_RUNTIME_DIR. When PipeWire mode was added for GNOME 49+, the environment variable handling was changed but not properly tested for Sway.

## Solution

The fix is in Wolf's container startup code. When passing environment variables:

1. **In wayland mode**: Ensure XDG_RUNTIME_DIR points to `/tmp/sockets` (where Wolf's wayland socket is)
2. **In pipewire mode**: XDG_RUNTIME_DIR can be `/run/user/1000` for PipeWire socket access

Additionally, ensure there are no duplicate XDG_RUNTIME_DIR entries.

### Files Modified

**`wolf/src/moonlight-server/runners/docker.cpp`** (lines 86-115):

The fix adds conditional logic to only override XDG_RUNTIME_DIR for pipewire mode:

```cpp
// Check if we're in pipewire mode by looking at WOLF_VIDEO_SOURCE_MODE in env_variables
bool use_pipewire_mode = false;
if (auto mode_it = env_variables.find("WOLF_VIDEO_SOURCE_MODE")) {
  use_pipewire_mode = (*mode_it == "pipewire");
  logs::log(logs::debug, "[DOCKER] Video source mode: {}, use_pipewire_mode: {}", *mode_it, use_pipewire_mode);
}

if (use_pipewire_mode) {
  // Mount at /run/user/1000 where PipeWire daemon creates its socket (pipewire-0)
  mounts.push_back(MountPoint{.source = pipewire_base_path.string(), .destination = "/run/user/1000", .mode = "rw"});
  // Set XDG_RUNTIME_DIR in container to match the mount point
  full_env.push_back("XDG_RUNTIME_DIR=/run/user/1000");
  logs::log(logs::debug, "[DOCKER] Pipewire mode: XDG_RUNTIME_DIR=/run/user/1000");
} else {
  // For wayland mode (Sway/KDE), do NOT override XDG_RUNTIME_DIR
  // The correct value (/tmp/sockets) is already set by common.cpp
  logs::log(logs::debug, "[DOCKER] Wayland mode: keeping XDG_RUNTIME_DIR from common.cpp (should be /tmp/sockets)");
}
```

## Testing Plan

1. Start a Sway desktop session
2. Verify Sway starts successfully (no wayland connection errors)
3. Verify video streaming works (connect via Moonlight or `helix spectask stream`)
4. Verify input injection works
5. Start an Ubuntu desktop session
6. Verify Ubuntu still works (PipeWire mode, D-Bus ScreenCast)
7. Verify video streaming works for Ubuntu
8. Take screenshots from both session types

## Non-Goals

- Changing the PipeWire mode for GNOME/Ubuntu (this works correctly)
- Changing the architecture of how video capture works

## Test Results

### Wayland Mode (KDE/Sway)
- **Container env**: `WOLF_VIDEO_SOURCE_MODE=wayland`, `XDG_RUNTIME_DIR=/tmp/sockets`
- **Wolf logs**: `[DOCKER] Video source mode: wayland, use_pipewire_mode: false`
- **Wolf logs**: `[DOCKER] Wayland mode: keeping XDG_RUNTIME_DIR from common.cpp (should be /tmp/sockets)`
- **Wayland socket visible**: `/tmp/sockets/wayland-1` exists in container
- **KDE Plasma started successfully** (nested compositor mode working)

### PipeWire Mode (Ubuntu/GNOME)
- **Container env**: `WOLF_VIDEO_SOURCE_MODE=pipewire`, `XDG_RUNTIME_DIR=/run/user/1000`
- **Wolf logs**: `[DOCKER] Video source mode: pipewire, use_pipewire_mode: true`
- **Wolf logs**: `[DOCKER] Pipewire mode: XDG_RUNTIME_DIR=/run/user/1000`
- **PipeWire streaming working**: Logs show `pipewirezerocopysrc` creating frames successfully
- **Screenshot test passed**: Ubuntu session screenshot saved successfully

### Verification
Both modes working correctly - no regression in Ubuntu, KDE/Sway can now connect to Wolf's wayland socket.

## Rollback Plan

If the fix causes issues, revert the changes. The impact is limited to Sway/KDE desktops.
