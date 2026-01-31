# Ubuntu GNOME Desktop: Hardware GPU Rendering Fix

**Date:** 2025-12-08
**Status:** In Progress
**Branch:** `feature/ubuntu-desktop`

## Executive Summary

The Ubuntu GNOME desktop container launches but has broken display rendering. After extensive investigation, we discovered the root cause: the XFCE base image's launch scripts are incompatible with GNOME's Mutter compositor. The solution is to replicate Zorin's working GOW scripts (which use DISPLAY=:9 and a simpler Xwayland launch pattern).

**Critical requirement:** Must use hardware GPU rendering (no software fallback).

## Problem Description

When launching the Ubuntu GNOME desktop:
- GNOME loading animation shows (compositor partially works)
- Desktop doesn't fully render - fragment visible, rest black
- Errors in logs:
  - `libEGL warning: egl: failed to create dri2 screen`
  - `Xlib: extension "NV-GLX" missing on display ":0"`

## Root Cause Analysis

### Investigation Findings

We extracted and compared scripts from both base images:

**Working: Zorin's `gow-zorin-18` image**
- Uses `DISPLAY=:9`
- Simple Xwayland launch: `Xwayland :9 &`
- GNOME session via: `/usr/bin/dbus-launch /usr/bin/gnome-session`
- Explicitly disables `RUN_SWAY` with `ENV RUN_SWAY=""`

**Broken: Ubuntu's approach (based on XFCE's `launch-comp.sh` pattern)**
- Uses `DISPLAY=:0`
- Complex Xwayland launch with WAYLAND_DISPLAY passthrough:
  ```bash
  dbus-run-session -- bash -E -c "WAYLAND_DISPLAY=\$REAL_WAYLAND_DISPLAY Xwayland :0 & sleep 2 && gnome-session"
  ```

### Extracted Scripts from Zorin Base Image

**Zorin's `/opt/gow/xorg.sh`:**
```bash
#!/bin/bash
export DISPLAY=:9

function wait_for_x_display() {
    local display=":9"
    local max_attempts=100
    local attempt=0
    while ! xdpyinfo -display "$display" >/dev/null 2>&1; do
        if (( attempt++ >= max_attempts )); then
            echo "Xwayland failed to start on display $display"
            exit 1
        fi
        sleep 0.1
    done
}

function wait_for_dbus() {
    local max_attempts=100
    local attempt=0
    while ! dbus-send --system --dest=org.freedesktop.DBus --type=method_call --print-reply \
        /org/freedesktop/DBus org.freedesktop.DBus.ListNames >/dev/null 2>&1; do
        if (( attempt++ >= max_attempts )); then
            echo "DBus failed to start"
            exit 1
        fi
        sleep 0.1
    done
}

function launch_xorg() {
    # start xwayland at :9
    Xwayland :9 &
    wait_for_x_display
    XWAYLAND_OUTPUT=$(xrandr --display :9 | grep " connected" | awk '{print $1}')
    xrandr --output "$XWAYLAND_OUTPUT" --mode "${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT}"
}

launch_xorg
sudo /opt/gow/dbus.sh # start dbus as root
wait_for_dbus
/opt/gow/desktop.sh # start desktop
```

**Zorin's `/opt/gow/desktop.sh`:**
```bash
#!/bin/bash

function launch_desktop() {
  export XDG_DATA_DIRS=/var/lib/flatpak/exports/share:/home/retro/.local/share/flatpak/exports/share:/usr/local/share/:/usr/share/
  export XDG_CURRENT_DESKTOP=zorin:GNOME
  export DE=zorin
  export DESKTOP_SESSION=zorin
  export GNOME_SHELL_SESSION_MODE="zorin"
  export XDG_SESSION_TYPE=x11
  export XDG_SESSION_CLASS="user"
  export _JAVA_AWT_WM_NONREPARENTING=1
  export GDK_BACKEND=x11
  export MOZ_ENABLE_WAYLAND=0
  export QT_QPA_PLATFORM="xcb"
  export QT_AUTO_SCREEN_SCALE_FACTOR=1
  export QT_ENABLE_HIGHDPI_SCALING=1
  export LC_ALL="en_US.UTF-8"
  export DISPLAY=:9
  unset WAYLAND_DISPLAY
  export $(dbus-launch)

  gsettings set org.gnome.desktop.interface scaling-factor 1
  flatpak remote-add --if-not-exists --user flathub https://dl.flathub.org/repo/flathub.flatpakrepo

  /usr/bin/dbus-launch /usr/bin/gnome-session
}

launch_desktop
```

**Zorin's `/opt/gow/dbus.sh`:**
```bash
#!/bin/bash
service dbus start
```

### Critical Differences Table

| Aspect | Zorin (Works) | Ubuntu (Broken) |
|--------|---------------|-----------------|
| **DISPLAY** | `:9` | `:0` |
| **Xwayland launch** | Simple `Xwayland :9 &` | Complex wrapper with WAYLAND_DISPLAY passthrough |
| **GNOME launch** | `/usr/bin/dbus-launch /usr/bin/gnome-session` | `dbus-run-session -- bash -E -c "... gnome-session"` |
| **RUN_SWAY env** | `RUN_SWAY=""` (explicitly disabled) | Not set |

### Why XFCE Pattern Doesn't Work for GNOME

The XFCE `dbus-run-session -- bash -E -c "WAYLAND_DISPLAY=..."` pattern:
1. Was designed for xfwm4 (XFCE's compositor), not Mutter (GNOME's compositor)
2. The WAYLAND_DISPLAY passthrough is unnecessary - Xwayland inherits it from the environment
3. Using `sleep 2` instead of proper wait functions causes race conditions

### Important Note on RUN_SWAY

**`RUN_SWAY` is NOT needed for GNOME and should NOT be set:**

1. `Dockerfile.zorin-helix` explicitly sets `ENV RUN_SWAY=""` to disable it
2. In `api/pkg/external-agent/wolf_executor.go:61-62`, `RUN_SWAY=1` is only set for the Sway desktop type:
   ```go
   // Sway needs RUN_SWAY=1 for GOW launcher to start Sway compositor
   return []string{"RUN_SWAY=1"}
   ```
3. Setting `RUN_SWAY=1` would cause Sway to run instead of GNOME

## Implementation Plan

### Files to Modify

1. **`Dockerfile.ubuntu-helix`** - Replace GOW scripts with Zorin-style versions
2. **`wolf/ubuntu-config/startup-app.sh`** - Simplify to use `exec /opt/gow/xorg.sh`

### Step-by-Step Implementation

#### Step 1: Update `Dockerfile.ubuntu-helix` GOW Scripts (PARTIALLY DONE)

The xorg.sh script has been updated to use DISPLAY=:9. The changes made so far:

```dockerfile
# /opt/gow/xorg.sh - Launches Xwayland and waits for it to be ready
# Using DISPLAY=:9 to match Zorin's working GNOME configuration
RUN cat > /opt/gow/xorg.sh << 'XORG_EOF'
#!/bin/bash
export DISPLAY=:9

function wait_for_x_display() {
    local max_attempts=100
    local attempt=0
    while ! xdpyinfo -display ":9" >/dev/null 2>&1; do
        if (( attempt++ >= max_attempts )); then
            echo "Xwayland failed to start on display :9"
            exit 1
        fi
        sleep 0.1
    done
}

function wait_for_dbus() {
    local max_attempts=100
    local attempt=0
    while ! dbus-send --system --dest=org.freedesktop.DBus --type=method_call --print-reply \
        /org/freedesktop/DBus org.freedesktop.DBus.ListNames >/dev/null 2>&1; do
        if (( attempt++ >= max_attempts )); then
            echo "DBus failed to start"
            exit 1
        fi
        sleep 0.1
    done
}

Xwayland :9 &
wait_for_x_display
XWAYLAND_OUTPUT=$(xrandr --display :9 | grep " connected" | awk '{print $1}')
xrandr --output "$XWAYLAND_OUTPUT" --mode "${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}"
sudo /opt/gow/dbus.sh
wait_for_dbus
exec /opt/gow/desktop.sh
XORG_EOF
```

**STILL TODO:** Update `/opt/gow/desktop.sh` to use DISPLAY=:9 (currently still uses :0).

Find and replace in `Dockerfile.ubuntu-helix` around line 220-251:
- Change `export DISPLAY=:0` to `export DISPLAY=:9`

#### Step 2: Update `wolf/ubuntu-config/startup-app.sh`

**Current broken ending (lines ~346-387):**
```bash
# Source GOW utilities
source /opt/gow/bash-lib/utils.sh

# Set up environment (same as launch-comp.sh)
export XDG_DATA_DIRS=/var/lib/flatpak/exports/share:/home/retro/.local/share/flatpak/exports/share:/usr/local/share/:/usr/share/

# Start D-Bus (required for GNOME)
sudo /opt/gow/startdbus

# Configure environment for GNOME on X11 (matching launch-comp.sh pattern)
export DESKTOP_SESSION=ubuntu
export XDG_CURRENT_DESKTOP=ubuntu:GNOME
export XDG_SESSION_TYPE="x11"
# ... more env vars ...

# Force software rendering for Mutter
export LIBGL_ALWAYS_SOFTWARE=1
export MUTTER_DEBUG_FORCE_FALLBACK_GLES=1
# ... more software rendering flags ...

exec dbus-run-session -- bash -E -c "WAYLAND_DISPLAY=\$REAL_WAYLAND_DISPLAY Xwayland :0 & sleep 2 && gnome-session"
```

**Replace with (matching Zorin's working pattern):**
```bash
# ============================================================================
# GNOME Session Startup (Zorin-compatible pattern)
# ============================================================================
echo "Launching GNOME via GOW xorg.sh..."
exec /opt/gow/xorg.sh
```

This means removing:
- All software rendering flags (`LIBGL_ALWAYS_SOFTWARE`, `MUTTER_DEBUG_*`, etc.)
- The `source /opt/gow/bash-lib/utils.sh` line
- The environment variable setup section
- The `sudo /opt/gow/startdbus` line
- The `exec dbus-run-session -- bash -E -c "..."` line

### Step 3: Build and Test

```bash
# Commit changes first (required for image tag to update)
git add -A && git commit -m "fix(ubuntu): use Zorin-style GOW scripts for GPU rendering"

# Build the Ubuntu image
./stack build-ubuntu

# Launch a new Ubuntu sandbox session and test
```

## Verification Checklist

After implementing the changes, verify:

1. [ ] GNOME desktop fully renders (not just loading animation)
2. [ ] No EGL/GLX errors in container logs
3. [ ] Hardware GPU acceleration is working (check with `glxinfo | grep "direct rendering"`)
4. [ ] Ubuntu theming is correct (Yaru theme, Ubuntu wallpaper)
5. [ ] Window tiling works (Terminal, Zed, Firefox in thirds)
6. [ ] Screenshot server works
7. [ ] Settings sync daemon works

## Commands for Debugging

**Check container logs:**
```bash
docker compose -f docker-compose.dev.yaml logs -f sandbox
```

**Get into a running container:**
```bash
docker exec -it <container_id> bash
```

**Check GPU rendering:**
```bash
# Inside container
glxinfo | grep "direct rendering"
# Should output: direct rendering: Yes
```

**View startup debug log:**
```bash
# Inside container
cat /tmp/ubuntu-startup-debug.log
```

## Reference Files

- **Ubuntu Dockerfile:** `Dockerfile.ubuntu-helix`
- **Ubuntu startup script:** `wolf/ubuntu-config/startup-app.sh`
- **Zorin Dockerfile (working reference):** `Dockerfile.zorin-helix`
- **Zorin startup script (working reference):** `wolf/zorin-config/startup-app.sh`
- **Plan file:** `/home/kai/.claude/plans/wise-splashing-rivest.md`

## Previous Attempts (Failed)

For reference, these approaches were tried and did not work:

1. **Software rendering fallback** - Works but defeats the purpose (user rejected)
2. **DISPLAY=:9 vs :0 alone** - No difference without the other script changes
3. **Adding wait functions** - Helps but not sufficient without the launch pattern change
4. **Copying Zorin's approach to XFCE base** - Partially attempted, needs completion

## Architecture Notes

### How GOW Desktop Streaming Works

1. **Wolf** provides the streaming server (Moonlight-compatible)
2. **Gamescope** provides a GPU-accelerated Wayland compositor
3. **Xwayland** runs on top of Gamescope's Wayland, providing X11 for the desktop
4. **GNOME/Mutter** runs on Xwayland

The container's startup script (`/opt/gow/startup.sh`) is responsible for:
1. Setting up Xwayland with the correct display
2. Starting D-Bus
3. Launching the GNOME session

### Base Image Comparison

| Feature | `games-on-whales/xfce:edge` | `mollomm1/gow-zorin-18:latest` |
|---------|----------------------------|-------------------------------|
| **Desktop** | XFCE with xfwm4 | GNOME with Mutter |
| **GOW scripts** | `launch-comp.sh` (XFCE-optimized) | `xorg.sh` + `desktop.sh` (GNOME-optimized) |
| **GPU packages** | All present | All present |
| **Why we use it** | Has all GPU packages, smaller | Has working GNOME scripts |

We keep the XFCE base for its GPU packages but replace the GOW scripts with Zorin-style scripts.
