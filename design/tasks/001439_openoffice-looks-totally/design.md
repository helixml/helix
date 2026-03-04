# Design: OnlyOffice 4K Rendering + Cursor Theme Fix

## Summary

Fix two OnlyOffice issues in GNOME headless Wayland:
1. **4K rendering broken** - Only top quarter renders at high resolutions (works at 1080p)
2. **Ignores system cursor theme** - Renders its own cursor instead of using Helix-Invisible

## Investigation Findings

### Root Cause Confirmed

OnlyOffice is a **Qt5 + CEF (Chromium Embedded Framework)** application that:
- Bundles Qt 5.9.9 (old version)
- Only includes X11/XCB platform plugins - **NO Wayland support**
- Runs via XWayland on Wayland compositors

The 4K rendering issue is caused by:
1. GNOME headless runs at 3840x2160 with `scaling-factor=2` (for 200% zoom)
2. This makes XWayland report **logical resolution** (1920x1080) to X11 apps
3. OnlyOffice creates a 1920x1080 window
4. But the actual surface is 3840x2160
5. Result: OnlyOffice renders to top-left quarter only

### Verified via Testing

```bash
# GNOME virtual monitor is 4K
$ ps aux | grep gnome-shell
gnome-shell --headless --unsafe-mode --virtual-monitor 3840x2160@60

# But XWayland reports 1080p due to scale factor 2
$ DISPLAY=:0 xdpyinfo | grep dimensions
dimensions:    1920x1080 pixels (508x286 millimeters)

# GNOME scaling factor
$ gsettings get org.gnome.desktop.interface scaling-factor
uint32 2
```

### OnlyOffice Qt Analysis

```bash
# OnlyOffice bundles old Qt 5.9
$ strings /opt/onlyoffice/desktopeditors/libQt5Core.so.5 | grep "^5\." | head -1
5.9.9

# Only X11 platform plugins available
$ ls /opt/onlyoffice/desktopeditors/platforms/
libqlinuxfb.so  libqminimal.so  libqoffscreen.so  libqvnc.so  libqxcb.so

# No Wayland plugin - would need libqwayland-*.so
```

### Why QT_SCALE_FACTOR Doesn't Work

Tried `QT_SCALE_FACTOR=2` - this makes OnlyOffice render at 2x scale but the window geometry is still 1920x1080, so it just renders larger content in the same quarter of the screen.

## Solution Options

### Option 1: Build OnlyOffice with Qt Wayland Support (Best)

Build OnlyOffice from source with modern Qt that includes Wayland plugins:
- Requires Qt 5.15+ with qtwayland5
- Significant build effort (multiple repos: desktop-apps, desktop-sdk, core, sdkjs, web-apps)
- Would provide native Wayland rendering at correct resolution

**Complexity**: High (multi-day effort)
**Effectiveness**: Would fully solve both issues

### Option 2: Enable xwayland-native-scaling (Mutter Feature)

GNOME/Mutter has experimental `xwayland-native-scaling` feature that makes XWayland report physical resolution instead of logical:

```bash
gsettings set org.gnome.mutter experimental-features "['scale-monitor-framebuffer', 'xwayland-native-scaling']"
```

**Issue**: Requires Mutter restart and may not work in headless mode.

**Complexity**: Low
**Effectiveness**: Uncertain in headless mode

### Option 3: Don't Use GNOME Scaling for 4K

Instead of `scaling-factor=2`, use 4K at 1x scale and let individual apps handle scaling via `GDK_SCALE`/`QT_SCALE_FACTOR` for native Wayland apps only.

This would make XWayland report 3840x2160 and OnlyOffice would work correctly, but UI would be tiny unless apps scale themselves.

**Complexity**: Medium (changes to startup-app.sh)
**Effectiveness**: Would fix OnlyOffice but may break other things

### Option 4: Use Flatpak OnlyOffice (May Have Wayland)

The Flatpak version might be built with Wayland support:

```bash
flatpak install flathub org.onlyoffice.desktopeditors
```

**Complexity**: Low
**Effectiveness**: Unknown - needs testing

## Recommended Approach

1. **Short term**: Test Flatpak version to see if it has Wayland support
2. **Medium term**: Investigate building OnlyOffice with Qt Wayland
3. **Long term**: File upstream issue requesting native Wayland support (already exists: #2105)

## Files That Would Need Changes

| File | Change |
|------|--------|
| `Dockerfile.ubuntu-helix` | Install flatpak OnlyOffice OR build from source with Qt Wayland |
| `desktop/ubuntu-config/startup-app.sh` | Possibly adjust scaling logic |

## Cursor Issue

Secondary to the rendering issue. Once OnlyOffice renders correctly, we can address cursor theme with:

```bash
export XCURSOR_THEME=Helix-Invisible
export XCURSOR_SIZE=48
```

However, OnlyOffice (like many apps) may render its own cursors for text editing regardless of system theme.

## References

- OnlyOffice Wayland issue: https://github.com/ONLYOFFICE/DesktopEditors/issues/2105
- OnlyOffice source: https://github.com/ONLYOFFICE/DesktopEditors
- OnlyOffice build tools: https://github.com/nicotine-plus/nicotine-plus/issues/2105
- Qt Wayland: https://doc.qt.io/qt-5/qtwaylandcompositor-index.html