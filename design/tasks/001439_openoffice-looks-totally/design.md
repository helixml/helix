# Design: LibreOffice Rendering Fix for GNOME Headless Wayland

## Summary

Fix LibreOffice rendering issues (partial screen, duplicate cursors) in the Helix ARM64 desktop environment running GNOME headless mode with pure Wayland.

## Root Cause Analysis

### Issue 1: Partial Screen Rendering (Top Quarter Only)

LibreOffice uses VCL (Visual Component Library) for rendering. On Linux, VCL supports multiple backends:
- `gtk3` / `gtk4` - GTK toolkit
- `kf5` / `kf6` - KDE/Qt toolkit  
- `gen` - Generic X11

**Problem**: Without explicit configuration, LibreOffice auto-detects the backend. In GNOME headless mode with `GDK_BACKEND=wayland`, the GTK backend may fail to properly negotiate the virtual monitor geometry, causing it to render only a portion of the window.

**Evidence**: GNOME headless creates a virtual monitor via `--virtual-monitor WxH@60`. Apps that don't properly query Wayland surface geometry may use incorrect dimensions.

### Issue 2: Duplicate Mouse Cursors

**Problem**: LibreOffice renders its own cursor sprites for text editing (I-beam), resize handles, etc. These conflict with:
1. Helix-Invisible cursor theme (transparent at compositor level)
2. Client-side cursor rendering in the browser

The result is LibreOffice's internal cursor + Helix's cursor overlay = duplicate cursors.

### Issue 3: General Visual Corruption

Likely a cascade effect from the geometry mismatch - incorrect buffer sizes, misaligned damage regions, or DMA-BUF format issues.

## Solution Design

### Approach: Force LibreOffice GTK4/Wayland Backend

Set environment variables to force LibreOffice to use the GTK4 Wayland backend correctly:

```bash
# In startup-app.sh or Dockerfile ENV
export SAL_USE_VCLPLUGIN=gtk4
export SAL_DISABLE_CURSOR_ANIMATION=1
```

**Rationale**:
- `SAL_USE_VCLPLUGIN=gtk4` forces the GTK4 backend which has better Wayland support
- `SAL_DISABLE_CURSOR_ANIMATION=1` may reduce cursor conflicts

### Alternative: Wrapper Script

Create `/usr/local/bin/libreoffice-wrapper.sh`:

```bash
#!/bin/bash
export SAL_USE_VCLPLUGIN=gtk4
export SAL_DISABLE_CURSOR_ANIMATION=1
exec /usr/bin/soffice "$@"
```

Update `.desktop` files to use the wrapper.

### Cursor Conflict Mitigation

LibreOffice's internal cursor rendering cannot be fully disabled, but we can:

1. **Accept dual cursors** - Document that LibreOffice shows its own cursor for text editing
2. **Use GTK4 backend** - Better integration with Wayland cursor protocol
3. **Test visibility** - Verify Helix-Invisible theme doesn't cause LibreOffice cursor to also be invisible

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
│  │  LibreOffice    │     │  PipeWire ScreenCast         │  │
│  │  (GTK4/Wayland) │     │  (captures full screen)      │  │
│  │                 │     └──────────────────────────────┘  │
│  │ SAL_USE_VCLPLUGIN=gtk4                                 │ │
│  └─────────────────┘                                       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Files to Modify

| File | Change |
|------|--------|
| `Dockerfile.ubuntu-helix` | Add LibreOffice environment variables to ENV block |
| `desktop/ubuntu-config/startup-app.sh` | Export SAL_* variables before app launch |

## Testing

1. Build image: `./stack build-ubuntu`
2. Start ARM64 session with LibreOffice
3. Verify:
   - Full window renders (not just top quarter)
   - Window resize works
   - Menus open correctly
   - Only expected cursors visible
   - Document editing functional

## Risks

- **GTK4 not installed**: May need to add `libreoffice-gtk4` package
- **Backend unavailable**: Fallback to `gtk3` if `gtk4` fails
- **Performance**: GTK4 Wayland may have different performance characteristics

## References

- [LibreOffice VCL Documentation](https://wiki.documentfoundation.org/Development/VCL)
- `SAL_USE_VCLPLUGIN` environment variable
- GNOME headless mode: `design/2025-01-18-cursor-go-pipewire.md`
- Client-side cursor: `design/2026-01-16-client-side-cursor-rendering.md`
