# AGS Build Setup Documentation (Sway/Hyprland)

**Date:** 2025-10-27
**Context:** AGS (Aylur's GTK Shell) desktop environment with Sway or Hyprland compositor
**Repository:** helixml/helix
**Target Compositor:** Sway (with Hyprland compatibility)

## Overview

This document describes the complete build and development setup for running AGS (Aylur's GTK Shell) with **Sway compositor** in containerized environments for Helix external agents and personal dev environments.

**Important:** While this documentation was recovered from Hyprland-based development work, AGS has **full native Sway support** and the configuration works with both compositors. The current Helix implementation uses Sway, not Hyprland.

## Quick Links - All GitHub Repositories

**🔧 Build Infrastructure:**
- **Main Helix Repo**: https://github.com/helixml/helix
- **Sway Dockerfile**: https://github.com/helixml/helix/blob/feature/helix-code/Dockerfile.sway-helix
- **Hyprland Dockerfiles** (reference): https://github.com/helixml/helix/tree/feature/helix-code (Dockerfile.hyprland-wolf, Dockerfile.hyprland-wolf-zed)

**🎨 AGS Configuration:**
- **helixml Fork**: https://github.com/helixml/dots-hyperland/tree/ii-ags (ii-ags branch)
- **Sway Service**: https://github.com/helixml/dots-hyperland/blob/ii-ags/.config/ags/services/sway.js
- **AGS Config Snapshot in Helix**: https://github.com/helixml/helix/tree/feature/helix-code/hypr-config/ags

**📦 AGS Binary:**
- **ii-agsv1 (upstream)**: https://github.com/end-4/ii-agsv1

**🎮 Related:**
- **Wolf (Moonlight Streaming)**: https://github.com/games-on-whales/wolf
- **Zed (Helix Fork)**: https://github.com/helixml/zed

## Compositor Compatibility

### AGS Works with Both Sway and Hyprland

The dots-hyprland AGS configuration includes **automatic compositor detection** and supports both:

- **Sway** (i3-compatible Wayland compositor) - **Current Helix implementation**
- **Hyprland** (Dynamic tiling Wayland compositor) - Original development environment

**How it works:**
1. AGS detects which compositor is running via environment variables (`SWAYSOCK` or `HYPRLAND_INSTANCE_SIGNATURE`)
2. Automatically loads the appropriate service module (`services/sway.js` or Hyprland's built-in service)
3. Falls back gracefully if one isn't available

**Key Evidence from Code:**

```javascript
// From modules/bar/main.js - Automatic compositor detection
const NormalOptionalWorkspaces = async () => {
    try {
        return (await import('./normal/workspaces_hyprland.js')).default();
    } catch {
        try {
            return (await import('./normal/workspaces_sway.js')).default();
        } catch {
            return null;
        }
    }
};
```

**Sway IPC Protocol Support:**
AGS includes a complete Sway service (`services/sway.js`) that implements the i3/Sway IPC protocol:
- Workspace management
- Window tracking
- Monitor detection
- Event subscriptions (window focus, workspace changes, etc.)

**Fallback Commands:**
The configuration includes fallback commands for compositor-agnostic operations:
```bash
hyprctl reload || swaymsg reload  # Reload compositor
pkill Hyprland || pkill sway      # Kill compositor
```

### What Needs to Change for Sway

**Nothing critical!** The AGS configuration is designed to work with both. However, for a Sway-focused deployment:

1. **Remove Hyprland-specific features** (optional cleanup):
   - Quick toggles for Hyprland-specific settings (animations, screen shaders)
   - Hyprland-specific keybind registration
   - These will simply be ignored on Sway

2. **Sway-specific scripts already included**:
   - `scripts/sway/swayToRelativeWs.sh` - Workspace navigation
   - Workspace widgets: `workspaces_sway.js` (already implemented)

3. **Compositor Build Differences**:
   - **Hyprland**: Requires building from source (complex, slow ~30+ min)
   - **Sway**: Available as standard Ubuntu package (`apt install sway`)
   - **Result**: Sway builds are **significantly faster and simpler**

### Why Helix Uses Sway Instead of Hyprland

**CRITICAL: Wolf streaming doesn't work with Hyprland!**

This is the **primary reason** Helix uses Sway:
- 🚫 **Wolf + Hyprland = Incompatible** - Wolf's Moonlight streaming server does not work with Hyprland
- ✅ **Wolf + Sway = Works** - Wolf streaming works perfectly with Sway

**Secondary benefits of Sway:**

✅ **Faster builds** - No source compilation needed, just `apt install sway`
✅ **Smaller images** - No build dependencies required
✅ **Stable package** - Ubuntu-maintained, tested releases
✅ **Simpler Dockerfile** - Remove all Hyprland build stages
✅ **Faster iteration** - Quick container recreation
✅ **Same AGS experience** - Full desktop shell functionality

**Bottom line:** The Hyprland work was exploratory/development only. For production Helix with Wolf streaming, Sway is required.

## Key Repositories and Build Infrastructure on GitHub

### 1. AGS Configuration: helixml/dots-hyperland (Fork)

**Repository:** https://github.com/helixml/dots-hyperland
**Branch:** `ii-ags`
**Direct Link:** https://github.com/helixml/dots-hyperland/tree/ii-ags
**Upstream:** https://github.com/end-4/dots-hyprland (branch: `ii-ags`)

**Custom Modifications:**
- `f1b80060` - Disable audio controls in AGS (removed 200+ lines from audio module)
- `030c1012` - Disable xwayland in Hyprland config
- Additional commits for container compatibility

**Key Files:**
- AGS Config: `.config/ags/` - Complete AGS desktop shell configuration
- SCSS Styles: `.config/ags/scss/` - Material Design theming
- Sway Integration: `.config/ags/services/sway.js` - Sway IPC protocol implementation

**Local Development Path:** `/home/luke/pm/dots-hyprland` (on mind.lukemarsden.net)

### 2. AGS Binary: end-4/ii-agsv1

**Repository:** https://github.com/end-4/ii-agsv1
**Purpose:** AGS v1 binary implementation
**Build Method:** Meson + npm in Dockerfile

### 3. Helix Build Infrastructure (This Repository)

**Repository:** https://github.com/helixml/helix

**Dockerfiles (Historical Reference):**
- **Dockerfile.hyprland-wolf**: https://github.com/helixml/helix/blob/feature/helix-code/Dockerfile.hyprland-wolf
  - Basic Hyprland v0.51.1 with Wolf streaming
  - ~177 lines, builds all Hyprland dependencies from source

- **Dockerfile.hyprland-wolf-zed**: https://github.com/helixml/helix/blob/feature/helix-code/Dockerfile.hyprland-wolf-zed
  - Full dev environment with AGS, Zed, Helix integration
  - ~433 lines, includes SCSS compilation and complete toolchain

**Current Sway Implementation:**
- **Dockerfile.sway-helix**: https://github.com/helixml/helix/blob/feature/helix-code/Dockerfile.sway-helix
  - Production Sway + Wolf + Zed environment
  - Uses standard Ubuntu Sway package (not building from source)

**AGS Configuration Snapshot:**
- **hypr-config/**: https://github.com/helixml/helix/tree/feature/helix-code/hypr-config
  - Complete Hyprland configuration from development era
  - **hypr-config/ags/**: AGS configuration copied from dots-hyperland ii-ags branch
  - Can be used as a reference or mounted into containers

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

## Simplified Sway Build (Recommended for New Work)

### Dockerfile Pattern for Sway + AGS

This is the **recommended approach for your colleague** working on Sway:

```dockerfile
FROM ubuntu:25.04

# Install Sway compositor (simple package install)
RUN apt-get update && apt-get install -y \
    sway \
    swayidle \
    swaylock \
    swaybg \
    && rm -rf /var/lib/apt/lists/*

# Install AGS build dependencies
RUN apt-get update && apt-get install -y \
    git meson npm \
    libgjs-dev libgtk-3-dev libgtk-layer-shell-dev \
    libpulse-dev libpam0g-dev libglib2.0-dev \
    gir1.2-gtk-3.0 gir1.2-glib-2.0 gobject-introspection \
    libgirepository1.0-dev \
    gvfs gjs glib2.0-bin libglib2.0-0 libgtk-3-0 \
    libgtk-layer-shell0 libpulse0 libpam0g \
    && rm -rf /var/lib/apt/lists/*

# Install SCSS compilation dependencies
RUN apt-get update && apt-get install -y \
    sass \
    build-essential \
    libnotify-bin libnotify-dev gir1.2-notify-0.7 \
    libdbusmenu-gtk3-dev gir1.2-dbusmenu-glib-0.4 gir1.2-dbusmenu-gtk3-0.4 \
    libgtksourceview-3.0-dev gir1.2-gtksource-3.0 \
    && rm -rf /var/lib/apt/lists/*

# Build AGS v1 binary
RUN npm install -g typescript && \
    cd /tmp && \
    git clone https://github.com/end-4/ii-agsv1.git && \
    cd ii-agsv1 && \
    npm install && \
    meson setup build --libdir "lib/ii-agsv1" -Dbuild_types=true && \
    meson compile -C build && \
    meson install -C build && \
    ln -sf /usr/local/share/com.github.Aylur.ags/com.github.Aylur.ags /usr/bin/agsv1 && \
    rm -rf /tmp/ii-agsv1

# Copy startup script that pre-compiles SCSS
COPY start-sway-session.sh /start-sway-session.sh
RUN chmod +x /start-sway-session.sh

ENTRYPOINT ["/start-sway-session.sh"]
```

**Startup Script Pattern** (`start-sway-session.sh`):

```bash
#!/bin/bash
set -e

export USER=ubuntu
export HOME=/home/ubuntu
export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu
mkdir -p $XDG_RUNTIME_DIR
chmod 700 $XDG_RUNTIME_DIR

# Start dbus
eval $(dbus-launch --sh-syntax)

# Start Sway
sway &
SWAY_PID=$!

sleep 3  # Wait for Sway to initialize

# Pre-compile AGS SCSS
echo "Pre-compiling AGS SCSS styles..."
mkdir -p /home/ubuntu/.cache/ags/user/generated
cd /home/ubuntu/.config/ags

if command -v sass >/dev/null 2>&1; then
    sass scss/main.scss /home/ubuntu/.cache/ags/user/generated/style.css 2>&1 || \
        echo "SCSS compilation failed, AGS will try at runtime"
    echo "SCSS compilation completed"
fi

# Start AGS
echo "Starting AGS bar and dock..."
agsv1 &
AGS_PID=$!

# Wait for processes
wait $SWAY_PID $AGS_PID
```

**Benefits of this approach:**
- ⚡ **Fast builds** (~5 min vs 30+ min for Hyprland from source)
- 📦 **Small images** (~1GB vs 2GB+ for Hyprland)
- 🔧 **Simple maintenance** - Standard Ubuntu packages
- ✅ **Tested in production** - Current Helix Sway implementation

## Reproducing the Setup

### Option 1: Sway Build (Recommended for New Work)

Use the simplified Sway Dockerfile pattern above. This is the **fastest and simplest** approach.

**Steps:**
1. Create Dockerfile with Sway + AGS (see "Simplified Sway Build" section)
2. Mount or copy AGS config from helixml/dots-hyperland fork
3. Build: `docker build -t helix-sway-ags:latest .`
4. Total build time: ~5 minutes

### Option 2: Hyprland Build from Source (Historical Reference)

1. Use `Dockerfile.hyprland-wolf-zed` as base
2. Add SCSS compilation dependencies
3. Build with Docker BuildKit for caching:
   ```bash
   DOCKER_BUILDKIT=1 docker build -f Dockerfile.hyprland-wolf-zed -t helix-hyprland-ags:latest .
   ```
4. Total build time: ~30-45 minutes

### Option 3: Development with Volume Mounts (Best for AGS Config Development)

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

## Summary for Your Colleague

### Quick Start Checklist

To get AGS running with Sway for Helix:

✅ **Use Sway, not Hyprland**
   - **CRITICAL**: Wolf streaming doesn't work with Hyprland
   - AGS has full native Sway support
   - Much faster builds (5 min vs 30+ min)
   - Simpler Dockerfile

✅ **Three required components:**
   1. **AGS v1 binary** - Build from `github.com/end-4/ii-agsv1` (meson + npm)
   2. **SCSS compiler** - Install `sass` package
   3. **AGS config** - Clone from `github.com/helixml/dots-hyperland` (ii-ags branch)

✅ **SCSS pre-compilation is critical:**
   - Pre-compile in startup script before launching AGS
   - Significantly speeds up AGS startup
   - Command: `sass scss/main.scss ~/.cache/ags/user/generated/style.css`

✅ **Use volume mounts for development:**
   ```yaml
   volumes:
     - ~/dots-hyprland/.config/ags:/home/ubuntu/.config/ags
   ```
   - Edit config files locally
   - Changes reflect immediately in container
   - No rebuilds needed

### What Works Out of the Box

The AGS configuration is **already compatible with Sway**:

- ✅ Workspace management (via Sway IPC)
- ✅ Window tracking and focus
- ✅ Bar and dock
- ✅ System indicators
- ✅ Application launcher
- ✅ SCSS theming and Material Design colors

### What to Ignore (Hyprland-specific)

These features will be silently ignored on Sway (no errors):

- ⚠️ Hyprland animation toggles
- ⚠️ Hyprland screen shader toggles
- ⚠️ Hyprland-specific keybind registration

**You don't need to remove them** - they'll just do nothing on Sway.

### Debugging Tips

1. **Check compositor detection:**
   ```bash
   # Inside container
   echo $SWAYSOCK  # Should be set for Sway
   ```

2. **Watch AGS logs:**
   ```bash
   agsv1 2>&1 | grep -E "(sway|workspace|window)"
   ```

3. **Test SCSS compilation manually:**
   ```bash
   cd ~/.config/ags
   sass scss/main.scss /tmp/test.css
   ```

4. **Verify Sway IPC:**
   ```bash
   swaymsg -t get_workspaces
   ```

### Reference Implementation

The current Helix Sway implementation (`Dockerfile.sway-helix`) is a working reference. Key patterns:

- Sway installed via `apt install sway`
- AGS v1 built from source during image build
- Startup script launches Sway → pre-compiles SCSS → starts AGS
- Configuration mounted from helixml fork (or copied into image)

### Complete Build Recipe

The complete AGS setup requires:
1. AGS v1 binary built from ii-agsv1
2. SCSS compiler (`sass` package) installed
3. AGS configuration from helixml/dots-hyperland fork
4. Runtime SCSS pre-compilation for fast startup
5. (Development) Volume mounts for instant iteration

**Key insight:** The Dockerfiles build the *binary* from upstream, but the *configuration* can come from either:
- Built into the image (cloned during build)
- Mounted at runtime (development workflow) ← **Recommended**
- Copied into hypr-config/ directory (snapshot approach)

All three approaches work, with runtime mounts providing the fastest iteration cycle for AGS configuration development.

### Next Steps for Implementation

1. **Start with simplified Sway Dockerfile** (see "Simplified Sway Build" section)
2. **Test basic Sway + AGS functionality**
3. **Mount helixml/dots-hyperland config** for customization
4. **Iterate on AGS config** using volume mounts
5. **Optimize SCSS pre-compilation** if needed

The hardest part (AGS Sway compatibility) is already done! You're building on a solid foundation.
