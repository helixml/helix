# Design: OnlyOffice 4K Rendering + Cursor Theme Fix

## Summary

Fix two OnlyOffice issues in GNOME headless Wayland:
1. **4K rendering broken** - Only top quarter renders at high resolutions (works at 1080p)
2. **Ignores system cursor theme** - Renders its own cursor instead of using Helix-Invisible

## Root Cause Analysis

### Issue 1: 4K Resolution Renders Only Top Quarter

OnlyOffice is Electron-based. At 4K (3840x2160), Electron/Chromium's HiDPI scaling logic may:
- Misdetect the scale factor in headless Wayland
- Create a 1080p-sized buffer but render to a 4K surface
- Result: content appears in top-left quarter only

**Evidence**: Works at 1080p, breaks at 4K. Classic DPI scaling mismatch.

### Issue 2: OnlyOffice Renders Its Own Cursor

OnlyOffice doesn't respect the system cursor theme (`Helix-Invisible`). It renders:
- I-beam for text editing
- Resize handles
- Custom spreadsheet cursors

This conflicts with Helix's client-side cursor rendering, causing duplicate cursors.

## Solution Design

### Fix 1: Force Correct Scaling for 4K

Add Electron flags to disable automatic DPI scaling and force correct geometry:

```bash
--force-device-scale-factor=1
--high-dpi-support=1
```

### Fix 2: Force System Cursor Theme

Set cursor-related environment variables before launching:

```bash
export XCURSOR_THEME=Helix-Invisible
export XCURSOR_SIZE=48
export GTK_CURSOR_THEME_NAME=Helix-Invisible
```

### Implementation: Wrapper Script

Add to `Dockerfile.ubuntu-helix` after OnlyOffice installation:

```dockerfile
# OnlyOffice wrapper for 4K support + system cursor theme
RUN if [ "${TARGETARCH}" != "arm64" ]; then \
    mv /usr/bin/desktopeditors /usr/bin/desktopeditors.real && \
    printf '#!/bin/bash\n\
export XCURSOR_THEME=Helix-Invisible\n\
export XCURSOR_SIZE=48\n\
export GTK_CURSOR_THEME_NAME=Helix-Invisible\n\
exec /usr/bin/desktopeditors.real \
--force-device-scale-factor=1 \
--high-dpi-support=1 \
--ozone-platform=wayland \
--enable-features=UseOzonePlatform \
"$@"\n' > /usr/bin/desktopeditors && \
    chmod +x /usr/bin/desktopeditors; \
fi
```

### Key Flags

| Flag | Purpose |
|------|---------|
| `--force-device-scale-factor=1` | Prevent DPI auto-detection issues at 4K |
| `--high-dpi-support=1` | Enable HiDPI but with explicit scale |
| `--ozone-platform=wayland` | Use native Wayland (already needed) |
| `XCURSOR_THEME=Helix-Invisible` | Force system cursor theme |
| `XCURSOR_SIZE=48` | Match Helix cursor size for hotspot fingerprinting |

## Files to Modify

| File | Change |
|------|--------|
| `Dockerfile.ubuntu-helix` | Add wrapper script after OnlyOffice install (~line 365) |

## Testing

1. `./stack build-ubuntu`
2. Start session at 4K resolution (3840x2160)
3. Launch OnlyOffice
4. Verify:
   - [ ] Full window renders at 4K
   - [ ] No partial/quarter-screen rendering
   - [ ] OnlyOffice cursor is invisible (using Helix-Invisible theme)
   - [ ] Client-side cursor renders correctly over OnlyOffice

## Risks

- **Cursor theme may not work**: Electron apps sometimes ignore GTK cursor settings
- **Scale factor=1 at 4K**: UI elements may be small; may need `=2` instead

## Fallback

If cursor theme still ignored:
- Accept dual cursors for OnlyOffice (document as known limitation)
- Or investigate Electron `--cursor-theme` flag if it exists