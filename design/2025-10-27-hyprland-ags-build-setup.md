# Hyprland + AGS Build Setup Documentation

**Date:** 2025-10-27
**Context:** AGS (Aylur's GTK Shell) desktop environment with Hyprland compositor
**Repository:** helixml/helix

## Overview

This document describes the complete build and development setup for running AGS (Aylur's GTK Shell) with Hyprland in containerized environments for Helix external agents and personal dev environments.

## Key Repositories

### 1. AGS Configuration: helixml/dots-hyperland (Fork)

**Repository:** https://github.com/helixml/dots-hyperland
**Branch:** `ii-ags`
**Upstream:** https://github.com/end-4/dots-hyprland (branch: `ii-ags`)

**Custom Modifications:**
- `f1b80060` - Disable audio controls in AGS (removed 200+ lines from audio module)
- `030c1012` - Disable xwayland in Hyprland config
- Additional commits for container compatibility

**Local Development Path:** `/home/luke/pm/dots-hyprland` (on mind.lukemarsden.net)

### 2. AGS Binary: end-4/ii-agsv1

**Repository:** https://github.com/end-4/ii-agsv1
**Purpose:** AGS v1 binary implementation
**Build Method:** Meson + npm in Dockerfile

## Complete Build Setup

### Dockerfile Structure

The complete Dockerfile includes three key components:

#### 1. SCSS Compilation Dependencies

```dockerfile
# Add SCSS compilation dependencies for dots-hyprland AGS
RUN apt-get update && apt-get install -y \
    # SCSS compiler and build tools
    sass \
    # Node.js build tools for material design colors
    build-essential \
    # Notification support for AGS
    libnotify-bin libnotify-dev gir1.2-notify-0.7 \
    # System tray support for AGS
    libdbusmenu-gtk3-dev gir1.2-dbusmenu-glib-0.4 gir1.2-dbusmenu-gtk3-0.4 \
    # GtkSource library for AGS
    libgtksourceview-3.0-dev gir1.2-gtksource-3.0 \
    && rm -rf /var/lib/apt/lists/*
```

#### 2. AGS v1 Build

```dockerfile
# AGS (Aylur's GTK Shell) build stage - cached separately for efficiency
RUN apt-get update && apt-get install -y \
    # AGS build dependencies
    git meson npm \
    libgjs-dev libgtk-3-dev libgtk-layer-shell-dev \
    libpulse-dev libpam0g-dev libglib2.0-dev \
    gir1.2-gtk-3.0 gir1.2-glib-2.0 gobject-introspection \
    libgirepository1.0-dev \
    # AGS runtime dependencies
    gvfs gjs glib2.0-bin libglib2.0-0 libgtk-3-0 \
    libgtk-layer-shell0 libpulse0 libpam0g \
    && rm -rf /var/lib/apt/lists/*

# Build AGS v1 (ii-agsv1) - cached separately
RUN npm install -g typescript && \
    cd /tmp && \
    git clone https://github.com/end-4/ii-agsv1.git && \
    cd ii-agsv1 && \
    npm install && \
    meson setup build --libdir "lib/ii-agsv1" -Dbuild_types=true && \
    meson compile -C build && \
    meson install -C build && \
    # Create agsv1 symlink (following the PKGBUILD pattern)
    rm -f /usr/bin/ags && \
    ln -sf /usr/local/share/com.github.Aylur.ags/com.github.Aylur.ags /usr/bin/agsv1 && \
    # Cleanup build files
    rm -rf /tmp/ii-agsv1
```

#### 3. Runtime SCSS Pre-compilation

In the startup script (e.g., `start-wayland-vnc.sh` or `start-ubuntu-session.sh`):

```bash
# Pre-compile AGS SCSS styles
echo "Pre-compiling AGS SCSS styles..."
mkdir -p /home/ubuntu/.cache/ags/user/generated
cd /home/ubuntu/.config/ags

if command -v sass >/dev/null 2>&1; then
    sass scss/main.scss /home/ubuntu/.cache/ags/user/generated/style.css 2>&1 || \
        echo "SCSS compilation failed, AGS will try at runtime"
    echo "SCSS compilation completed"
else
    echo "Sass not available, AGS will compile at runtime"
fi
```

**Purpose:** Pre-compiling SCSS significantly speeds up AGS startup since it doesn't have to compile styles on first run.

## Development Workflow

### Fast Iteration Setup (docker-compose.dev.yaml)

The development workflow uses volume mounts to enable rapid iteration without container rebuilds:

```yaml
volumes:
    # Mount Hyprland config for fast iteration
    - ./hypr-config:/home/ubuntu/.config/hypr

    # Mount AGS config from dots-hyprland fork for fast iteration
    - /home/luke/pm/dots-hyprland/.config/ags:/home/ubuntu/.config/ags

    # Mount Zed config for persistence
    - ./zed-config:/home/ubuntu/.config/zed
```

### How the Development Process Works

1. **Build Time:**
   - Dockerfile builds AGS v1 binary from `github.com/end-4/ii-agsv1`
   - Installs SCSS compiler (`sass` package)
   - May clone upstream `github.com/end-4/dots-hyprland` (optional, gets overridden)

2. **Runtime:**
   - docker-compose mounts **helixml fork** from `/home/luke/pm/dots-hyprland/.config/ags`
   - This overrides any built-in AGS config
   - Custom modifications (disabled audio, no xwayland) take effect
   - Changes to local files reflect immediately in running container

3. **SCSS Compilation:**
   - Startup script pre-compiles `scss/main.scss` → `~/.cache/ags/user/generated/style.css`
   - AGS loads pre-compiled CSS on startup (much faster)
   - AGS can also recompile at runtime via `handleStyles()` function in `init.js`

## AGS SCSS Compilation Details

### AGS init.js Compilation Function

Located in `.config/ags/init.js`:

```javascript
export const COMPILED_STYLE_DIR = `${GLib.get_user_cache_dir()}/ags/user/generated`

globalThis['handleStyles'] = (resetMusic) => {
    // Reset music styles if requested
    Utils.exec(`mkdir -p "${GLib.get_user_state_dir()}/ags/scss"`);
    if (resetMusic) {
        Utils.exec(`bash -c 'echo "" > ${GLib.get_user_state_dir()}/ags/scss/_musicwal.scss'`);
        Utils.exec(`bash -c 'echo "" > ${GLib.get_user_state_dir()}/ags/scss/_musicmaterial.scss'`);
    }

    // Generate overrides for icon theme
    let lightdark = darkMode.value ? "dark" : "light";
    Utils.writeFileSync(`
@mixin symbolic-icon {
    -gtk-icon-theme: '${userOptions.icons.symbolicIconTheme[lightdark]}';
}
`, `${GLib.get_user_state_dir()}/ags/scss/_lib_mixins_overrides.scss`)

    // Compile and apply SCSS
    async function applyStyle() {
        Utils.exec(`mkdir -p ${COMPILED_STYLE_DIR}`);
        Utils.exec(`sass -I "${GLib.get_user_state_dir()}/ags/scss" -I "${App.configDir}/scss/fallback" "${App.configDir}/scss/main.scss" "${COMPILED_STYLE_DIR}/style.css"`);
        App.resetCss();
        App.applyCss(`${COMPILED_STYLE_DIR}/style.css`);
        console.log('[LOG] Styles loaded')
    }
    applyStyle().then(() => {
        loadSourceViewColorScheme(CUSTOM_SOURCEVIEW_SCHEME_PATH);
    }).catch(print);
}
```

### Color Generation with SCSS

The dots-hyprland config includes Material Design 3 color generation:

**Script:** `.config/ags/scripts/color_generation/colorgen.sh`

```bash
# Generate colors from image or hex color
sass -I "$STATE_DIR/scss" -I "$CONFIG_DIR/scss/fallback" \
    "$CACHE_DIR/user/generated/material_colors.scss" \
    "$CACHE_DIR/user/generated/colors_classes.scss" \
    --style compressed
```

**Script:** `.config/ags/scripts/color_generation/applycolor.sh`

Reads generated colors from `$STATE_DIR/scss/_material.scss` and applies them to:
- AGS styles (triggers `handleStyles()`)
- Hyprland colors
- GTK themes
- Terminal colors
- Fuzzel launcher

## Dockerfiles Available

### 1. Dockerfile.hyprland-wolf (Basic)

**File:** `Dockerfile.hyprland-wolf`
**Size:** ~7KB, 177 lines
**Base:** `ghcr.io/games-on-whales/base-app:edge`

**Features:**
- Hyprland v0.51.1 built from source
- All Hyprland dependencies (hyprutils, hyprlang, aquamarine, etc.)
- Basic desktop (kitty, waybar, xwayland)
- Wolf integration for streaming

**Use Case:** Minimal Hyprland with Wolf streaming, no AGS or development tools

### 2. Dockerfile.hyprland-wolf-zed (Full Development)

**File:** `Dockerfile.hyprland-wolf-zed`
**Size:** ~18KB, 433 lines
**Base:** `ghcr.io/games-on-whales/base-app:edge`

**Features:**
- Everything from hyprland-wolf
- AGS v1 (ii-agsv1) built from source
- Complete Zed integration with WebSocket sync
- Go toolchain + Helix API binary
- Development tools (Node.js, npm, Python)
- Google Chrome with Wayland support
- Ghostty terminal
- NVIDIA CUDA runtime
- Material Symbols Rounded font
- SCSS compiler (`sass`)

**Use Case:** Full development environment for Helix external agents

### 3. Dockerfile.zed-agent-vnc (Remote/Mind Version)

**Location:** `luke@mind.lukemarsden.net:pm/helix.2/Dockerfile.zed-agent-vnc`
**Size:** 366 lines
**Base:** `ubuntu:25.04`

**Features:**
- Uses pre-built HyprMoon .deb package (not building from source)
- AGS v1 with **explicit SCSS compilation setup**
- Includes detailed comments about SCSS dependencies
- VNC + GPU acceleration
- Lighter weight than full Hyprland build

**Use Case:** The version that was actually used for development on mind with volume mounts

## Key Differences Between Approaches

### Build from Source (Dockerfile.hyprland-wolf-zed)
- **Pros:** Full control, latest Hyprland version
- **Cons:** Slow builds (~30+ minutes), complex dependencies
- **Size:** Large image (~2GB+)

### Pre-built Package (mind version)
- **Pros:** Fast builds, simpler Dockerfile
- **Cons:** Depends on HyprMoon .deb package availability
- **Size:** Smaller image

### Runtime Mount (Development)
- **Pros:** Instant iteration, no rebuilds needed
- **Cons:** Requires local checkout of dots-hyprland fork
- **Perfect for:** Active AGS configuration development

## Reproducing the Setup

### Option 1: Full Build from Source

1. Use `Dockerfile.hyprland-wolf-zed` as base
2. Add SCSS compilation dependencies (from Option 3)
3. Build with Docker BuildKit for caching:
   ```bash
   DOCKER_BUILDKIT=1 docker build -f Dockerfile.hyprland-wolf-zed -t helix-hyprland-ags:latest .
   ```

### Option 2: Pre-built Package (Faster)

1. Obtain HyprMoon .deb package (or build it separately)
2. Use the mind version Dockerfile pattern
3. Install package instead of building from source

### Option 3: Development with Volume Mounts (Recommended)

1. Clone helixml fork:
   ```bash
   git clone -b ii-ags https://github.com/helixml/dots-hyperland.git ~/dots-hyprland
   ```

2. Use either Dockerfile as base image

3. Add volume mounts to docker-compose.dev.yaml:
   ```yaml
   volumes:
       - ./hypr-config:/home/ubuntu/.config/hypr
       - ~/dots-hyprland/.config/ags:/home/ubuntu/.config/ags
   ```

4. Startup script pre-compiles SCSS (see "Runtime SCSS Pre-compilation" section)

5. Iterate on AGS config locally, changes reflect immediately

## Files Recovered

The following files were restored from git history to the helix repo:

- `Dockerfile.hyprland-wolf` - Basic Hyprland + Wolf
- `Dockerfile.hyprland-wolf-zed` - Full dev environment with AGS
- `hypr-config/` - Complete Hyprland configuration from helix history
- `hypr-config/ags/` - AGS configuration copied from dots-hyprland ii-ags branch

**Note:** The complete helixml fork of dots-hyprland remains in its own repository at https://github.com/helixml/dots-hyperland (note spelling: "hyperland" not "hyprland").

## Related Repositories

- **Wolf (Moonlight Server):** https://github.com/games-on-whales/wolf
- **HyprMoon (Custom Hyprland):** https://github.com/helixml/hyprmoon
- **Zed (Helix Fork):** https://github.com/helixml/zed
- **Helix (Main Project):** https://github.com/helixml/helix

## Summary

The complete AGS setup requires:
1. AGS v1 binary built from ii-agsv1
2. SCSS compiler (`sass` package) installed
3. AGS configuration from helixml/dots-hyperland fork
4. Runtime SCSS pre-compilation for fast startup
5. (Development) Volume mounts for instant iteration

The key insight is that the Dockerfiles build the *binary* from upstream, but the *configuration* can come from either:
- Built into the image (cloned during build)
- Mounted at runtime (development workflow)
- Copied into hypr-config/ directory (snapshot approach)

All three approaches work, with runtime mounts providing the fastest iteration cycle for AGS configuration development.
