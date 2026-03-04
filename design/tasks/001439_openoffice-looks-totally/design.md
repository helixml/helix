# Design: OnlyOffice Rendering Fix for GNOME Headless Wayland

## Summary

Fix OnlyOffice Desktop Editors rendering issues (partial screen, duplicate cursors) in the Helix AMD64 desktop environment running GNOME headless mode with pure Wayland.

## Root Cause Analysis

### Issue 1: Partial Screen Rendering (Top Quarter Only)

OnlyOffice is an **Electron-based application**. Electron apps on Linux can use either X11 or Wayland backends via the Ozone platform abstraction layer.

**Problem**: Without explicit configuration, Electron apps may:
- Default to X11 backend but find no X server (no XWayland in GNOME headless)
- Attempt Wayland but misconfigure the surface geometry
- Use software rendering fallback with incorrect buffer sizes

**Evidence**: Chrome (also Electron-based) already has a wrapper script with special flags. OnlyOffice has no such wrapper.

### Issue 2: Duplicate Mouse Cursors

**Problem**: OnlyOffice renders its own cursor sprites for:
- Text editing (I-beam cursor)
- Resize handles
- Cell selection in spreadsheets

These conflict with:
1. Helix-Invisible cursor theme (transparent at compositor level)
2. Client-side cursor rendering in the browser

Result: OnlyOffice's internal cursor + Helix's overlay cursor = duplicate cursors.

### Issue 3: General Visual Corruption

Likely caused by:
- Incorrect Ozone platform selection
- Missing GPU acceleration flags
- Buffer format mismatch with PipeWire capture

## Solution Design

### Approach: OnlyOffice Wrapper Script (like Chrome)

Create a wrapper script that forces correct Electron/Wayland configuration:

```bash
#!/bin/bash
# /usr/bin/onlyoffice-wrapper.sh
exec /usr/bin/desktopeditors \
    --ozone-platform=wayland \
    --enable-features=UseOzonePlatform,WaylandWindowDecorations \
    --disable-gpu-sandbox \
    "$@"
```

### Implementation in Dockerfile

```dockerfile
# After OnlyOffice installation, create wrapper
RUN if [ "${TARGETARCH}" != "arm64" ]; then \
    mv /usr/bin/desktopeditors /usr/bin/desktopeditors.real && \
    printf '#!/bin/bash\nexec /usr/bin/desktopeditors.real --ozone-platform=wayland --enable-features=UseOzonePlatform,WaylandWindowDecorations "$@"\n' > /usr/bin/desktopeditors && \
    chmod +x /usr/bin/desktopeditors && \
    # Also patch .desktop file
    sed -i 's|Exec=/usr/bin/desktopeditors|Exec=/usr/bin/desktopeditors --ozone-platform=wayland --enable-features=UseOzonePlatform,WaylandWindowDecorations|g' /usr/share/applications/onlyoffice-desktopeditors.desktop; \
fi
```

### Key Electron Flags

| Flag | Purpose |
|------|---------|
| `--ozone-platform=wayland` | Force Wayland backend instead of X11 |
| `--enable-features=UseOzonePlatform` | Enable Ozone platform abstraction |
| `--enable-features=WaylandWindowDecorations` | Use Wayland-native window decorations |
| `--disable-gpu-sandbox` | May help with GPU access in containers |

### Cursor Conflict Mitigation

OnlyOffice's internal cursor rendering cannot be fully disabled. Options:

1. **Accept dual cursors** - Document that OnlyOffice shows its own cursor for text editing
2. **Test with Wayland backend** - Wayland-native Electron may integrate better with cursor protocol
3. **Hide Helix cursor over OnlyOffice** - Frontend could detect OnlyOffice window and hide overlay

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Helix Desktop Container                  │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────┐     ┌──────────────────────────────┐  │
│  │  GNOME Shell    │────▶│  Virtual Monitor             │  │
│  │  (headless)     │     │  1920x1080@60                 │  │
│  └─────────────────┘     └──────────────────────────────┘  │
│           │                         │                       │
│           ▼                         ▼                       │
│  ┌─────────────────┐     ┌──────────────────────────────┐  │
│  │  OnlyOffice     │     │  PipeWire ScreenCast         │  │
│  │  (Electron)     │     │  (captures full screen)      │  │
│  │                 │     └──────────────────────────────┘  │
│  │ --ozone-platform=wayland                               │ │
│  │ --enable-features=UseOzonePlatform                     │ │
│  └─────────────────┘                                       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Files to Modify

| File | Change |
|------|--------|
| `Dockerfile.ubuntu-helix` | Add OnlyOffice wrapper script after installation |
| `Dockerfile.sway-helix` | Same wrapper pattern (if Sway also affected) |

## Testing

1. Build image: `./stack build-ubuntu`
2. Start new AMD64 session
3. Launch OnlyOffice from app menu or command line
4. Verify:
   - Full window renders (not just top quarter)
   - Window resize works
   - Menus open correctly
   - Document editing works
   - Note cursor behavior (document any expected dual-cursor situation)
5. Screenshot working state

## Risks

- **Flag compatibility**: OnlyOffice's Electron version may not support all flags
- **GPU acceleration**: May need additional flags for hardware rendering
- **Older Electron**: OnlyOffice may use older Electron without full Wayland support

## Fallback Options

If Wayland flags don't work:

1. **Add XWayland**: Install and configure XWayland for OnlyOffice specifically
2. **Environment variable**: Try `ELECTRON_OZONE_PLATFORM_HINT=wayland`
3. **Disable GPU**: `--disable-gpu` for software rendering (slower but may work)

## References

- [Electron Wayland Support](https://www.electronjs.org/docs/latest/api/environment-variables#electron_ozone_platform_hint-linux)
- [Chromium Ozone Platform](https://chromium.googlesource.com/chromium/src/+/HEAD/docs/ozone_overview.md)
- Chrome wrapper in `Dockerfile.ubuntu-helix` (lines 783-786)
- GNOME headless mode: `design/2025-01-18-cursor-go-pipewire.md`
