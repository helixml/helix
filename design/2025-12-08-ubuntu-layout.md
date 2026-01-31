# Ubuntu GNOME Desktop Layout and Clipboard Implementation

**Date:** 2025-12-08
**Branch:** feature/ubuntu-desktop
**Status:** In Progress

## Overview

This document covers the implementation of Ubuntu GNOME desktop containers for Wolf streaming, including clipboard synchronization, window tiling, and desktop appearance fixes.

## Goals

1. **Clipboard Synchronization** - Bidirectional copy/paste between browser and Wolf desktop container
2. **Window Tiling** - Automatic 3-column layout (Terminal | Zed | Firefox) like Sway
3. **Desktop Appearance** - Ubuntu default wallpaper and theme
4. **Startup Script** - Auto-launch dev server and Firefox with the app

## Architecture

```
Browser (Host)
    ↓ WebSocket (Moonlight Web)
Wolf/Gamescope Compositor (sandbox container)
    ↓ Wayland/X11
Ubuntu GNOME Container (helix-ubuntu)
    ├── GNOME Shell on Xwayland (DISPLAY=:9)
    ├── screenshot-server (port 9876) - handles clipboard + screenshots
    ├── devilspie2 - window positioning
    └── settings-sync-daemon - Zed settings sync
```

## Completed Work

### 1. X11 Clipboard Support (WORKING)

**Problem:** screenshot-server only supported Wayland clipboard (`wl-paste`/`wl-copy`), but Ubuntu GNOME runs on Xwayland (X11).

**Solution:** Added X11 clipboard support using `xclip`.

**Files Modified:**
- `api/cmd/screenshot-server/main.go` - Added `isX11Mode()`, `handleGetClipboardX11()`, `handleSetClipboardX11()`

**Key Code:**
```go
// isX11Mode returns true if we should use X11 clipboard (xclip) instead of Wayland
func isX11Mode() bool {
    if clipboardModeChecked {
        return useX11Clipboard
    }
    clipboardModeChecked = true

    display := os.Getenv("DISPLAY")
    if display == "" {
        useX11Clipboard = false
        return false
    }

    _, err := exec.LookPath("xclip")
    if err != nil {
        useX11Clipboard = false
        return false
    }

    // Test if xclip can actually connect to the X server
    testCmd := exec.Command("xclip", "-selection", "clipboard", "-o")
    testCmd.Env = append(os.Environ(), fmt.Sprintf("DISPLAY=%s", display))
    output, err := testCmd.CombinedOutput()
    if err != nil && strings.Contains(string(output), "cannot open display") {
        useX11Clipboard = false
        return false
    }

    useX11Clipboard = true
    return true
}
```

**Testing:**
```bash
# Inside Ubuntu container
docker exec helix-sandbox-nvidia-1 docker exec <container_id> \
  curl -s http://localhost:9876/clipboard
# Returns: {"data":"clipboard content","type":"text"}

# Write to clipboard
docker exec helix-sandbox-nvidia-1 docker exec <container_id> \
  curl -s -X POST http://localhost:9876/clipboard \
  -H "Content-Type: application/json" \
  -d '{"data":"test","type":"text"}'
```

**Commit:** `1ab286001` - feat(clipboard): add X11 clipboard support for Ubuntu containers

### 2. dconf Load Fix (WORKING)

**Problem:** `dconf load` was running in `startup-app.sh` before D-Bus was started, causing "Cannot autolaunch D-Bus without X11 $DISPLAY" error.

**Solution:** Moved `dconf load` to `desktop.sh` where D-Bus session is already available.

**Files Modified:**
- `Dockerfile.ubuntu-helix` - Added dconf load to desktop.sh
- `wolf/ubuntu-config/startup-app.sh` - Removed early dconf load

**Commit:** `4b2260ad9` - fix(ubuntu): move dconf load to desktop.sh after D-Bus is started

### 3. Zed Window Class Fix (WORKING)

**Problem:** Zed reports window class as `dev.zed.Zed-Dev` (dev builds) not `Zed`, so devilspie2 wasn't matching it.

**Solution:** Updated devilspie2 config to match the correct window class.

**Files Modified:**
- `wolf/ubuntu-config/devilspie2/helix-tiling.lua`

**Key Code:**
```lua
-- Zed editor -> Middle column (column 2)
-- Zed reports class as "dev.zed.Zed-Dev" for dev builds, "dev.zed.Zed" for release
elseif win_class == "dev.zed.Zed-Dev" or win_class == "dev.zed.Zed"
       or win_class == "Zed" or win_class == "zed"
       or string.find(string.lower(tostring(win_class) or ""), "zed") then
    debug_print("Positioning Zed in middle column: " .. tostring(win_class))
    set_window_position(COLUMN_WIDTH, PANEL_HEIGHT)  -- x=640
    set_window_size(COLUMN_WIDTH, WINDOW_HEIGHT)
```

**Commit:** `d0b0d8f80` - fix(ubuntu): match Zed window class 'dev.zed.Zed-Dev' in devilspie2

### 4. Screenshot Server DISPLAY Environment

**Problem:** screenshot-server needs DISPLAY=:9 to use X11 clipboard.

**Solution:** Pass DISPLAY in the autostart entry.

**Files Modified:**
- `wolf/ubuntu-config/startup-app.sh` - autostart entry includes `DISPLAY=:9`

```bash
cat > ~/.config/autostart/screenshot-server.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Screenshot Server
Exec=/bin/bash -c "DISPLAY=:9 /usr/local/bin/screenshot-server"
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=0
NoDisplay=true
EOF
```

## Remaining Issues

### 1. Wallpaper Shows Purple Instead of Ubuntu Default

**Status:** NOT FIXED

**Symptoms:** Desktop shows solid purple (#2C001E) instead of Ubuntu wallpaper image.

**Investigation:**
- dconf settings ARE correctly set:
  ```
  picture-uri='file:///usr/share/backgrounds/warty-final-ubuntu.png'
  picture-uri-dark='file:///usr/share/backgrounds/warty-final-ubuntu.png'
  picture-options='zoom'
  ```
- Wallpaper file exists and is valid PNG (3840x2160)
- gsettings also shows correct values
- GNOME is not rendering the wallpaper for unknown reason

**Possible Causes:**
1. GNOME background service not starting properly
2. File permissions issue
3. Timing issue - settings applied before GNOME Shell ready
4. Xwayland/Gamescope rendering issue

**Debug Commands:**
```bash
# Check current settings
docker exec helix-sandbox-nvidia-1 docker exec -u retro <id> \
  bash -c "DISPLAY=:9 gsettings get org.gnome.desktop.background picture-uri"

# Check dconf dump
docker exec helix-sandbox-nvidia-1 docker exec -u retro <id> \
  bash -c "DISPLAY=:9 dconf dump /org/gnome/desktop/background/"

# Force wallpaper refresh
docker exec helix-sandbox-nvidia-1 docker exec -u retro <id> \
  bash -c "DISPLAY=:9 gsettings set org.gnome.desktop.background picture-uri 'file:///usr/share/backgrounds/warty-final-ubuntu.png'"
```

### 2. Cursor Dot (Gamescope Software Cursor)

**Status:** NOT FIXED - Requires Wolf-level changes

**Symptoms:** A small dot appears near the mouse cursor, offset from the actual cursor position.

**Root Cause:** This is Gamescope/wolf-ui's software cursor overlay, rendered by the compositor for input tracking during streaming.

**Solution Required:**
- Pass `-C 0` (hide cursor delay = 0) to wolf-ui/Gamescope when launched
- This is configured in Wolf binary, not in the container

**Gamescope Options:**
```
-C, --hide-cursor-delay   hide cursor image after delay
--cursor                  path to default cursor image
--cursor-scale-height     scale cursor against base height
```

**Wolf-level Fix Needed:**
The wolf-ui process is launched by Wolf. Need to modify Wolf to pass `-C 0` option.

### 3. Firefox Auto-Launch

**Status:** PARTIALLY WORKING

**Current Behavior:**
- Startup script runs successfully
- Dev server starts on port 3000
- `xdg-open http://localhost:3000` is called but Firefox may not appear

**Debug:**
```bash
# Check if Firefox is running
docker exec helix-sandbox-nvidia-1 docker exec <id> ps aux | grep firefox

# Check dev server
docker exec helix-sandbox-nvidia-1 docker exec <id> curl -s localhost:3000 | head -5

# Manually open Firefox
docker exec helix-sandbox-nvidia-1 docker exec -u retro <id> \
  bash -c "DISPLAY=:9 xdg-open http://localhost:3000"
```

## Key Files

### Container Configuration
- `Dockerfile.ubuntu-helix` - Ubuntu container build
- `wolf/ubuntu-config/startup-app.sh` - Container startup script
- `wolf/ubuntu-config/dconf-settings.ini` - GNOME settings (wallpaper, theme, etc.)
- `wolf/ubuntu-config/devilspie2/helix-tiling.lua` - Window tiling rules

### Clipboard Implementation
- `api/cmd/screenshot-server/main.go` - Screenshot + clipboard server
- `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` - Frontend clipboard sync (lines 948-1006, 1363-1443)
- `api/pkg/server/external_agent_handlers.go` - API clipboard endpoints

### How Frontend Clipboard Sync Works

1. **Remote → Local (Auto-sync every 2s):**
   - Frontend polls `GET /api/v1/external-agents/{sessionID}/clipboard`
   - API calls screenshot-server via RevDial
   - screenshot-server reads X11 clipboard with `xclip -selection clipboard -o`
   - If content changed, frontend writes to browser clipboard

2. **Local → Remote (Paste intercept):**
   - Frontend intercepts Ctrl+V / Cmd+V keystrokes
   - Reads browser clipboard
   - POSTs to `POST /api/v1/external-agents/{sessionID}/clipboard`
   - screenshot-server writes to X11 clipboard with `xclip -selection clipboard -i`
   - Then sends the paste keystroke to Wolf

## Testing Checklist

### Clipboard
- [ ] Copy text in browser, Ctrl+V in Wolf desktop - pastes correctly
- [ ] Copy text in Wolf desktop, auto-syncs to browser clipboard within 2s
- [ ] Logs show `[X11] Using X11 mode with DISPLAY=:9`

### Window Layout
- [ ] Zed opens in middle column (x=640)
- [ ] Terminal opens in left column (x=0)
- [ ] Firefox opens in right column (x=1280)

### Startup
- [ ] Container starts without errors
- [ ] Dev server runs on localhost:3000
- [ ] Firefox launches with the app

### Desktop
- [ ] Ubuntu wallpaper displays (currently failing)
- [ ] Yaru dark theme applied
- [ ] Ubuntu Dock visible on left

## Build Commands

```bash
# Build Ubuntu image
./stack build-ubuntu

# Check version
cat sandbox-images/helix-ubuntu.version

# Verify in sandbox
docker exec helix-sandbox-nvidia-1 docker images helix-ubuntu:latest

# Watch logs
docker logs -f helix-sandbox-nvidia-1 2>&1 | grep -E "ubuntu|Ubuntu|error"
```

## Commits in This Session

1. `1ab286001` - feat(clipboard): add X11 clipboard support for Ubuntu containers
2. `4b2260ad9` - fix(ubuntu): move dconf load to desktop.sh after D-Bus is started
3. `d0b0d8f80` - fix(ubuntu): match Zed window class 'dev.zed.Zed-Dev' in devilspie2

## Next Steps

1. **Investigate wallpaper issue** - Why is GNOME not rendering the wallpaper despite correct settings?
2. **Fix cursor dot** - Modify Wolf to pass `-C 0` to wolf-ui/Gamescope
3. **Improve Firefox auto-launch** - May need delay or different xdg-open approach
4. **Test clipboard in browser** - Verify end-to-end clipboard sync works in actual usage

## References

- [Games on Whales Documentation](https://games-on-whales.github.io/wolf/stable/user/configuration.html)
- [Gamescope Options](https://wiki.archlinux.org/title/Gamescope)
- [Gamescope Cursor Issue #511](https://github.com/ValveSoftware/gamescope/issues/511)
