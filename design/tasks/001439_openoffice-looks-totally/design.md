# Design: OnlyOffice 4K Rendering + Cursor Theme Fix

## Summary

Fix two OnlyOffice issues in GNOME headless Wayland:
1. **4K rendering broken** - Only top quarter renders at high resolutions (works at 1080p)
2. **Ignores system cursor theme** - Renders its own cursor instead of using Helix-Invisible

## Root Cause Analysis (Verified via Testing)

### Issue 1: 4K Resolution Renders Only Top Quarter

**OnlyOffice is NOT Electron-based. It's Qt5 + CEF (Chromium Embedded Framework).**

OnlyOffice bundles its own Qt5 libraries WITHOUT the Wayland plugin. Available plugins:
- `libqxcb.so` (X11) ✅
- `libqlinuxfb.so`, `libqminimal.so`, `libqoffscreen.so`, `libqvnc.so`
- NO `libqwayland.so` ❌

**This means OnlyOffice REQUIRES XWayland to run - it cannot use native Wayland.**

The 4K issue is caused by GNOME's 2x scaling:
1. GNOME virtual monitor: 3840x2160 @ 2x scale
2. XWayland reports **logical resolution** (1920x1080) to X11 apps
3. OnlyOffice thinks screen is 1920x1080, renders at that size
4. On 4K physical display, this fills only the top-left quarter

**Evidence from `xdpyinfo`:**
```
dimensions:    1920x1080 pixels (508x286 millimeters)
```

### Issue 2: OnlyOffice Cursor

**The cursor theme DOES work!** When `XCURSOR_THEME=Helix-Invisible` is set, the OnlyOffice cursor becomes invisible as expected. The duplicate cursor issue was the Helix client-side cursor rendering on top.

## Solution Design

### Fix 1: XWayland Environment Setup

OnlyOffice needs these environment variables to run via XWayland:

```bash
export DISPLAY=:0
export XAUTHORITY=/run/user/1000/.mutter-Xwaylandauth.*  # Dynamic file
export LD_LIBRARY_PATH=/opt/onlyoffice/desktopeditors
```

### Fix 2: Cursor Theme

```bash
export XCURSOR_THEME=Helix-Invisible
export XCURSOR_SIZE=48
```

### Fix 3: 4K Scaling Issue

This is NOT fixable at the OnlyOffice level. Options:
1. **Run at 1x scale** - Set GNOME scaling to 1x for full 4K resolution
2. **Accept the limitation** - X11 apps see logical resolution when scaling > 1x
3. **Use GDK_SCALE** - May help some GTK parts but not the CEF webview

### Implementation: Wrapper Script

Update `/usr/bin/onlyoffice-desktopeditors` or create wrapper in Dockerfile:

```bash
#!/bin/bash
# OnlyOffice requires X11 via XWayland (Qt5 without Wayland plugin)
export DISPLAY=:0

# Find the Mutter XWayland auth file (name changes on each session)
XAUTH_FILE=$(ls /run/user/1000/.mutter-Xwaylandauth.* 2>/dev/null | tail -1)
if [ -n "$XAUTH_FILE" ]; then
    export XAUTHORITY="$XAUTH_FILE"
fi

# Cursor theme for X11 apps
export XCURSOR_THEME=Helix-Invisible
export XCURSOR_SIZE=48

# Required library path
APP_PATH=/opt/onlyoffice/desktopeditors
export LD_LIBRARY_PATH=$APP_PATH${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}

exec $APP_PATH/DesktopEditors "$@"
```

## Files to Modify

| File | Change |
|------|--------|
| `Dockerfile.ubuntu-helix` | Add wrapper script after OnlyOffice install (~line 365) |

## Testing Results

### Working Configuration
- OnlyOffice launches successfully with XWayland
- Cursor theme (Helix-Invisible) is respected
- Full UI renders correctly

### 4K Scaling Limitation
- At 2x GNOME scaling, OnlyOffice sees 1920x1080 logical resolution
- This is XWayland behavior, not an OnlyOffice bug
- **Recommendation**: Document as known limitation or run at 1x scale for 4K

## Implementation Notes

1. OnlyOffice bundles Qt5 at `/opt/onlyoffice/desktopeditors/libQt5*.so`
2. Platform plugins at `/opt/onlyoffice/desktopeditors/platforms/`
3. The existing `/usr/bin/onlyoffice-desktopeditors` script sets LD_LIBRARY_PATH but not DISPLAY/XAUTHORITY
4. XWayland auth file has dynamic suffix (e.g., `.NZGDL3`) that changes per session
5. Created `~/.icons/default` symlink to help X11 apps find cursor theme