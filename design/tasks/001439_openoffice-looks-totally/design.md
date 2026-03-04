# Design: OnlyOffice 4K Rendering Fix for GNOME Headless Wayland

## Summary

Fix OnlyOffice Desktop Editors rendering issues at 4K resolution in GNOME headless Wayland. The app only renders in the top-left quarter of the screen at high resolutions.

## Investigation Findings

### Root Cause: XWayland + GNOME Scaling Mismatch

OnlyOffice is a **Qt5 + CEF (Chromium Embedded Framework)** application that:
- Uses Qt 5.9 (bundled, not system Qt)
- Only has X11/XCB platform plugin (no Wayland support)
- Runs via XWayland on GNOME Wayland

**The Problem:**
1. GNOME is running at 4K (3840x2160) with scale factor 2.0
2. XWayland reports **logical resolution** (1920x1080) to X11 apps
3. OnlyOffice creates a 1920x1080 window
4. But the actual surface is 3840x2160 (physical)
5. Result: Content renders in top-left quarter only

**Evidence:**
```bash
# XWayland reports logical resolution, not physical
$ DISPLAY=:0 xdpyinfo | grep dimensions
dimensions:    1920x1080 pixels (508x286 millimeters)

# But GNOME is actually at 4K with scale 2
$ gdbus call ... GetCurrentState
# Shows: 3840x2160@60.000 with scale 2.0
```

### Why Qt Scaling Env Vars Don't Help

Tried these approaches - none fixed the window geometry issue:
- `QT_SCALE_FACTOR=2` - Makes content render at 2x but window is still 1920x1080
- `QT_SCREEN_SCALE_FACTORS="2"` - Same result
- `QT_AUTO_SCREEN_SCALE_FACTOR=0` - No effect

The problem is the **window size** is wrong, not just the content scaling. OnlyOffice asks XWayland for screen size, gets 1920x1080, and creates a window that size.

### OnlyOffice Architecture

OnlyOffice bundles Qt 5.9.9 with only X11 support:
```
/opt/onlyoffice/desktopeditors/
├── DesktopEditors          # Main binary
├── libQt5Core.so.5         # Qt 5.9.9 (bundled)
├── libQt5Gui.so.5
├── libQt5XcbQpa.so.5       # X11 platform
├── libcef.so               # Chromium Embedded Framework
└── platforms/
    ├── libqxcb.so          # Only X11 plugin!
    ├── libqlinuxfb.so
    ├── libqminimal.so
    ├── libqoffscreen.so
    └── libqvnc.so
```

No `libqwayland*.so` plugins exist - native Wayland is not an option without rebuilding OnlyOffice.

## Solution Options

### Option 1: Enable XWayland Native Scaling (Preferred)

GNOME/Mutter has an experimental feature `xwayland-native-scaling` that makes XWayland report physical resolution instead of logical.

```bash
gsettings set org.gnome.mutter experimental-features \
  "['scale-monitor-framebuffer', 'xwayland-native-scaling']"
```

**Issue:** Requires Mutter restart. Need to add this to `startup-app.sh` before gnome-shell starts.

### Option 2: Don't Use Compositor Scaling at 4K

Instead of `scaling-factor=2`, use only client-side scaling:
- Set compositor scale to 1.0
- Use `GDK_SCALE=2` and `QT_SCALE_FACTOR=2` for native Wayland apps
- X11 apps via XWayland will see true 4K resolution

**Downside:** All X11 apps will have tiny UI unless they respect Qt/GDK scale vars.

### Option 3: Run OnlyOffice at 1080p Explicitly

Force OnlyOffice to run in a 1080p window and let Mutter upscale:
```bash
# In wrapper script
export QT_SCREEN_SCALE_FACTORS="1"
# Plus force window geometry somehow
```

**Issue:** Qt5 XCB doesn't have good programmatic geometry control.

### Option 4: Build OnlyOffice with Wayland Support

Build from source with Qt 5.15+ including QtWayland.

**Issue:** Complex build system, many dependencies. Not practical short-term.

## Recommended Fix

**Implement Option 1** - Enable `xwayland-native-scaling` in `startup-app.sh`:

```bash
# In desktop/ubuntu-config/startup-app.sh, before gnome-shell starts:
gsettings set org.gnome.mutter experimental-features \
  "['scale-monitor-framebuffer', 'xwayland-native-scaling']"
```

This makes XWayland report 3840x2160 to X11 apps. OnlyOffice will create a proper 4K window, and Qt's scaling will handle HiDPI rendering.

## Files to Modify

| File | Change |
|------|--------|
| `desktop/ubuntu-config/startup-app.sh` | Add `xwayland-native-scaling` to experimental features |
| `Dockerfile.ubuntu-helix` | Ensure `qtwayland5` package is installed (already done) |

## Cursor Issue

Separate from the 4K rendering issue. OnlyOffice renders its own cursor via Qt/CEF. To use the system cursor theme:

```bash
export XCURSOR_THEME=Helix-Invisible
export XCURSOR_SIZE=48
```

Add to the OnlyOffice wrapper script in Dockerfile.

## Testing

1. Rebuild: `./stack build-ubuntu`
2. Start 4K session (3840x2160 with HELIX_ZOOM_LEVEL=200)
3. Launch OnlyOffice
4. Verify full window renders (not just top quarter)
5. Verify cursor uses system theme

## References

- [GNOME XWayland Native Scaling](https://blogs.gnome.org/shell/2022/11/04/gnome-43-updates/)
- OnlyOffice GitHub issue #2105 - Wayland support request
- OnlyOffice uses Qt 5.9 + CEF, X11 only