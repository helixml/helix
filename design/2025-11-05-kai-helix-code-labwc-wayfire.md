# labwc and Wayfire Desktop Support for Helix Code

**Author**: Kai (with Claude Code assistance)
**Date**: November 5, 2025
**Status**: Implemented

## Problem Statement

Helix Code currently supports three desktop environments for Personal Dev Environments (PDEs) and External Agents:

1. **Sway** - Lightweight tiling compositor (~150MB memory)
   - ✅ Wayland-native, lightweight, efficient
   - ❌ Tiling window management confuses users expecting traditional desktops
   - ❌ Not mouse-friendly (requires keyboard shortcuts)

2. **XFCE** - Traditional desktop (~250MB memory)
   - ✅ Familiar floating window management
   - ❌ X11-only (missing Wayland benefits)

3. **Zorin/GNOME** - Full-featured desktop (~500MB memory)
   - ✅ Modern, polished, full-featured
   - ❌ Heavy resource usage
   - ❌ Forced to X11 mode by base image

**User requirement**: We need Wayland-native floating desktop options that provide traditional mouse-driven UX without the resource overhead of GNOME.

## Research Findings

### Games-on-Whales Desktop Ecosystem

From GitHub Container Registry (`ghcr.io/games-on-whales`):
- **xfce** image available (but likely X11-based)
- **No official GNOME Wayland** images
- **Sway** is the primary Wayland option (tiling)

### Wolf Wayland Architecture

Wolf uses a custom headless Wayland compositor (`gst-wayland-display`) based on:
- **Smithay** framework (Rust)
- **wlroots** compatibility layer
- Designed to work with any wlroots-based compositor

**Key insight**: Since Sway (wlroots-based) works with Wolf, other wlroots compositors should work too.

### Lightweight Wayland Floating Compositor Options

Research identified two excellent candidates:

#### 1. labwc (Openbox-inspired)
- ✅ **Wayland-native** (wlroots-based)
- ✅ **Floating/stacking compositor** (not tiling)
- ✅ **Lightweight** (similar footprint to Sway)
- ✅ **Traditional UX** (Openbox-style)
- ✅ **Recommended by XFCE** for Wayland sessions
- ✅ **Proven containerizable**
- ✅ **Mature and stable**

#### 2. Wayfire (Compiz-inspired)
- ✅ **Wayland-native** (wlroots-based)
- ✅ **3D compositor with effects**
- ✅ **Floating primary, tiling optional**
- ✅ **Customizable and extensible**
- ✅ **Visual effects** (wobbly windows, cube, animations)
- ✅ **Feature-rich** without being heavy

Both use **wlroots**, ensuring Wolf compatibility.

## Implementation

### Architecture

```
User Request (HELIX_DESKTOP=labwc or wayfire)
           ↓
API reads via getDesktopTypeFromEnv()
           ↓
Returns DesktopLabwc or DesktopWayfire enum
           ↓
getDesktopImage() maps to helix-labwc:latest or helix-wayfire:latest
           ↓
Container starts with NO RUN_SWAY set
           ↓
GOW launcher sees RUN_SWAY unset, executes /opt/gow/startup-app.sh
           ↓
startup-app.sh launches labwc or wayfire compositor
           ↓
Wayland session active, Zed auto-starts, streaming ready
```

### Files Created

#### 1. labwc Configuration

**wolf/labwc-config/**
- `startup-app.sh` - Main initialization script
- `start-zed-helix.sh` - Zed launcher with auto-restart
- `labwc-config/rc.xml` - Keybindings and window rules
- `labwc-config/menu.xml` - Context menu definitions
- `labwc-config/autostart` - Auto-start applications (waybar)
- `waybar/config.json` - Status bar configuration
- `waybar/style.css` - Status bar styling

**Key features**:
- Super+Enter → Terminal (Ghostty)
- Super+D → Application launcher (rofi)
- Super+Q → Close window
- Super+M → Maximize
- Super+F → Fullscreen
- Super+Arrow → Window snapping
- Alt+Tab → Window switching
- Mouse drag on titlebar → Move window
- Super+Left/Right mouse → Move/Resize windows

#### 2. Wayfire Configuration

**wolf/wayfire-config/**
- `startup-app.sh` - Main initialization script
- `start-zed-helix.sh` - Zed launcher with auto-restart
- `wayfire.ini` - Comprehensive Wayfire configuration
- `waybar/config.json` - Status bar configuration
- `waybar/style.css` - Status bar styling

**Key features**:
- Same keybindings as labwc (for consistency)
- **3D effects enabled**:
  - Wobbly windows (physics-based)
  - Cube desktop switcher (Super+Ctrl+Left/Right)
  - Zoom effect (Super+Scroll)
  - Fade animations
  - Expo workspace overview (Super+S)
- **Plugins loaded**: animate, cube, expo, grid, wobbly, zoom, etc.

#### 3. Dockerfiles

**Dockerfile.labwc-helix**
- Base: Ubuntu 25.04 (Plucky Puffin)
- Compositor: labwc + wlroots
- Panel: waybar
- Launcher: rofi
- Screenshot: grim (Wayland-native)
- Same applications as other desktops (Firefox, Ghostty, OnlyOffice, Zed)

**Dockerfile.wayfire-helix**
- Base: Ubuntu 25.04 (Plucky Puffin)
- Compositor: Wayfire + wlroots
- Panel: waybar
- Launcher: rofi
- Screenshot: grim (Wayland-native)
- Additional: wcm (Wayfire Config Manager)
- Same applications as other desktops

#### 4. Go Code Updates

**api/pkg/external-agent/wolf_executor.go**

Added desktop types:
```go
const (
    DesktopSway    DesktopType = "sway"
    DesktopXFCE    DesktopType = "xfce"
    DesktopZorin   DesktopType = "zorin"
    DesktopLabwc   DesktopType = "labwc"   // NEW
    DesktopWayfire DesktopType = "wayfire" // NEW
)
```

Updated functions:
- `getDesktopImage()` - Maps desktop type to Docker image
- `parseDesktopType()` - Recognizes "labwc" and "wayfire"
- `createSwayWolfApp()` - Mounts labwc/wayfire config files
- `recreateWolfAppForInstance()` - Supports labwc/wayfire in dev mode

**api/pkg/external-agent/wolf_executor_apps.go**

Updated:
- `createSwayWolfAppForAppsMode()` - Mounts labwc/wayfire configs

#### 5. Stack Script Updates

**./stack**

Added build functions:
- `build-labwc` - Builds helix-labwc:latest image
- `build-wayfire` - Builds helix-wayfire:latest image

Updated help text:
- Added labwc and Wayfire to build command list
- Added to "Next steps" output after building Zed

## Technical Comparison

### Resource Usage (Estimated)

| Desktop   | Memory | Disk Space | Compositor |
|-----------|--------|------------|------------|
| Sway      | ~150MB | ~500MB     | Tiling     |
| labwc     | ~150MB | ~550MB     | Floating   |
| Wayfire   | ~200MB | ~600MB     | 3D Effects |
| XFCE      | ~250MB | ~700MB     | Floating (X11) |
| Zorin     | ~500MB | ~1.2GB     | Floating (X11) |

### Feature Comparison

| Feature | Sway | labwc | Wayfire | XFCE | Zorin |
|---------|------|-------|---------|------|-------|
| Wayland native | ✅ | ✅ | ✅ | ❌ | ❌* |
| Floating windows | ❌ | ✅ | ✅ | ✅ | ✅ |
| Mouse-friendly | ❌ | ✅ | ✅ | ✅ | ✅ |
| 3D effects | ❌ | ❌ | ✅ | ❌ | ✅ |
| Lightweight | ✅ | ✅ | ✅ | ❌ | ❌ |
| wlroots-based | ✅ | ✅ | ✅ | ❌ | ❌ |

*Zorin image forces X11 mode via environment variables

### Startup Flow

**Common elements** (all desktops):
1. startup-app.sh sets environment variables
2. Symlinks Zed state directory
3. Copies desktop config files to user home
4. Starts settings-sync-daemon (background)
5. Starts screenshot-server (background)
6. Starts start-zed-helix.sh (background, 5s delay)
7. Sources /opt/gow/launch-comp.sh
8. Executes compositor via GOW launcher

**labwc-specific**:
- Exports Wayland environment variables
- Copies labwc XML configs to ~/.config/labwc
- Copies waybar JSON/CSS to ~/.config/waybar
- Executes: `launcher labwc`

**Wayfire-specific**:
- Exports Wayland environment variables
- Copies wayfire.ini to ~/.config/wayfire
- Copies waybar JSON/CSS to ~/.config/waybar
- Executes: `launcher wayfire`

**Critical**: Both unset `RUN_SWAY` to prevent GOW launcher from starting Sway.

## Configuration Details

### labwc Configuration

**Keybinding philosophy**: Sensible defaults matching modern desktop expectations

- **Super key** (Windows key) as primary modifier
- **Alt key** for window switching
- **Mouse integration** for drag/resize
- **Snapping** to edges with keyboard

**rc.xml highlights**:
- Server-side decorations (window borders/titles)
- 8px corner radius for modern look
- 10px gap between windows
- Adwaita-dark theme

### Wayfire Configuration

**Plugin architecture**: Modular effects system

Enabled plugins:
- `animate` - Window open/close animations
- `cube` - 3D desktop cube switcher
- `expo` - Workspace overview
- `wobbly` - Physics-based window wobble
- `grid` - Window tiling/snapping
- `zoom` - Magnification
- `fast-switcher` - Same-app window cycling

**wayfire.ini highlights**:
- Floating-first (traditional desktop)
- 2x2 workspace grid
- Cube desktop switcher (Super+Ctrl+Arrows)
- Wobbly windows on drag
- Custom animation speeds

### waybar Configuration

**Same for both desktops** (consistency):

**Left**: Workspace switcher (wlr/workspaces module)
**Center**: Clock with date tooltip
**Right**: Audio, network, CPU, memory, system tray

**Styling**: Catppuccin-inspired color scheme
- Background: Semi-transparent dark blue
- Active workspace: Highlighted in lavender
- Module backgrounds: Translucent with rounded corners

## Environment Variables

Both desktops export critical Wayland variables:

```bash
export XDG_SESSION_TYPE=wayland
export WAYLAND_DISPLAY=wayland-0
export GDK_BACKEND=wayland
export QT_QPA_PLATFORM=wayland
export MOZ_ENABLE_WAYLAND=1
export CLUTTER_BACKEND=wayland

# HiDPI support (200% scaling)
export GDK_SCALE=2
export GDK_DPI_SCALE=1
```

**Critical**: `unset RUN_SWAY` ensures GOW launcher doesn't override compositor choice.

## Testing

### Build Commands

```bash
# Build labwc desktop
./stack build-labwc

# Build Wayfire desktop
./stack build-wayfire
```

### Usage

Set environment variable before starting Helix:

```bash
# For labwc
export HELIX_DESKTOP=labwc

# For Wayfire
export HELIX_DESKTOP=wayfire
```

Then create PDE or external agent session as usual.

### Verification Checklist

After creating session:

1. **Container starts without RUN_SWAY errors**
   ```bash
   docker logs <container-id> | grep -i sway
   # Should NOT see "[Sway] - Starting"
   ```

2. **Compositor launches successfully**
   ```bash
   docker logs <container-id> | grep "Starting.*compositor"
   # Should see "Starting labwc compositor" or "Starting Wayfire compositor"
   ```

3. **Wayland display available**
   ```bash
   docker exec <container-id> echo $WAYLAND_DISPLAY
   # Should output: wayland-0
   ```

4. **Zed auto-starts**
   ```bash
   docker logs <container-id> | grep "Launching Zed"
   # Should see Zed startup messages
   ```

5. **Connect via Moonlight and verify**:
   - Floating windows work
   - Mouse drag/resize works
   - Keyboard shortcuts work
   - waybar shows at top
   - rofi launcher (Super+D) opens
   - Zed is running and responsive

## Key Learnings

### 1. Wolf Works with Any wlroots Compositor

Wolf's custom Wayland compositor (gst-wayland-display) is designed to work with wlroots-based compositors. Since Sway works, labwc and Wayfire work too.

**Takeaway**: Any wlroots compositor should be Wolf-compatible out of the box.

### 2. Ubuntu 25.04 Has Excellent Wayland Support

Ubuntu 25.04 (Plucky Puffin) ships with:
- labwc available in apt repositories
- Wayfire available in apt repositories
- Modern wlroots libraries
- Wayland-first philosophy

**Takeaway**: Ubuntu 25.04 is an excellent base for Wayland desktops.

### 3. Consistency Improves UX

Both labwc and Wayfire use:
- Same keybindings (Super+D, Super+Enter, etc.)
- Same waybar configuration
- Same application launcher (rofi)
- Same color scheme

**Takeaway**: Consistent keybindings across desktops reduce learning curve.

### 4. GOW Mount Path Matters

Different base images expect different paths:
- **Official GOW images**: `/opt/gow/startup-app.sh`
- **Community Zorin image**: `/opt/gow/startup.sh`
- **Our custom images**: `/opt/gow/startup-app.sh` (GOW standard)

**Takeaway**: Always verify mount paths when integrating with GOW launcher.

### 5. Wayland Screenshot Tools Differ from X11

- **Wayland**: `grim` (what we use)
- **X11**: `scrot` (what Zorin uses)

**Takeaway**: screenshot-server handles both (tries grim first, falls back to scrot).

## Future Enhancements

### 1. Per-User Desktop Preference

Store preferred desktop in user profile:
```go
type User struct {
    PreferredDesktop string `json:"preferred_desktop"`
}
```

### 2. Per-Session Desktop Selection

Allow choosing desktop when creating PDE:
```typescript
createPersonalDevEnvironment({
    name: "My PDE",
    desktop: "labwc",  // or "wayfire", "sway", "xfce", "zorin"
})
```

### 3. Desktop Metrics

Track which desktops users prefer:
- Usage statistics
- Performance metrics
- Crash rates
- User satisfaction

### 4. Additional Desktop Options

Potential future additions:
- **LXQt** - Full DE, Wayland-ready
- **Weston** - Reference Wayland compositor
- **Hyprland** - Dynamic tiling with animations
- **River** - Flexible tiling Wayland compositor

## Summary

Adding labwc and Wayfire desktops provides Helix Code users with:

✅ **Wayland-native** floating desktops
✅ **Lightweight** resource usage (similar to Sway)
✅ **Traditional UX** (mouse-driven, familiar)
✅ **Wolf-compatible** (wlroots-based)
✅ **Full feature parity** with existing desktops

Users now have **five desktop choices**:
1. **Sway** - Tiling, minimal, keyboard-driven
2. **XFCE** - Traditional, X11, familiar
3. **Zorin** - Full GNOME, polished, X11
4. **labwc** - Floating, Wayland, lightweight ← NEW
5. **Wayfire** - 3D effects, Wayland, modern ← NEW

The implementation maintains consistency with existing patterns while expanding choice for users who want Wayland benefits with traditional desktop UX.
