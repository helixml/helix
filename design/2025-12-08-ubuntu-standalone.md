# Ubuntu GNOME Desktop: Build from Ubuntu 24.04 from Scratch

**Date:** 2025-12-08
**Status:** Implementation Guide
**Branch:** `feature/ubuntu-desktop`

## Executive Summary

Build a clean Ubuntu GNOME desktop container from `ubuntu:24.04` base, replicating only the essential GOW (Games on Whales) infrastructure. This avoids the GNOME 48/GTK 4.16 Vulkan rendering issues present in Ubuntu 25.04.

**Why Ubuntu 24.04?**
- Has GNOME 46 / Mutter 14 (stable, OpenGL-based rendering)
- Ubuntu 25.04's GNOME 48 / GTK 4.16 defaults to Vulkan which breaks on NVIDIA
- Zorin OS 18 (which works) is based on Ubuntu 24.04

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Wolf Streaming Server                    │
│                  (Moonlight-compatible)                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                        Gamescope                             │
│              (GPU-accelerated Wayland compositor)            │
│                    WAYLAND_DISPLAY=wayland-1                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                        Xwayland                              │
│                (X11 server on Wayland)                       │
│                      DISPLAY=:9                              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    GNOME Shell / Mutter                      │
│                   (Desktop environment)                      │
└─────────────────────────────────────────────────────────────┘
```

## GOW Infrastructure Components Required

### 1. s6-overlay Init System

s6-overlay is the init system used by GOW containers. It:
- Runs scripts in `/etc/cont-init.d/` on startup (numbered for ordering)
- Manages long-running services in `/etc/services.d/`
- Handles container lifecycle

**Installation:**
```dockerfile
ARG S6_OVERLAY_VERSION=3.1.6.2
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz /tmp
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-x86_64.tar.xz /tmp
RUN tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz && \
    tar -C / -Jxpf /tmp/s6-overlay-x86_64.tar.xz && \
    rm /tmp/s6-overlay-*.tar.xz

ENTRYPOINT ["/init"]
```

### 2. Init Scripts (from `/etc/cont-init.d/`)

These run in order on container startup:

#### `/etc/cont-init.d/10-setup_user.sh`
Creates the `retro` user with correct UID/GID from environment variables.

```bash
#!/usr/bin/with-contenv bash
# Create retro user with UID/GID from environment

USER_NAME="${USER_NAME:-retro}"
USER_UID="${PUID:-1000}"
USER_GID="${PGID:-1000}"

# Create group if it doesn't exist
if ! getent group "$USER_GID" > /dev/null 2>&1; then
    groupadd -g "$USER_GID" "$USER_NAME"
fi

# Create user if it doesn't exist
if ! id "$USER_NAME" > /dev/null 2>&1; then
    useradd -m -u "$USER_UID" -g "$USER_GID" -s /bin/bash "$USER_NAME"
fi

# Ensure home directory exists and has correct ownership
mkdir -p "/home/$USER_NAME"
chown "$USER_UID:$USER_GID" "/home/$USER_NAME"

# Add to video and render groups for GPU access
usermod -aG video,render "$USER_NAME" 2>/dev/null || true

echo "[10-setup_user] User $USER_NAME created with UID=$USER_UID GID=$USER_GID"
```

#### `/etc/cont-init.d/15-setup_devices.sh`
Sets up device permissions for GPU access.

```bash
#!/usr/bin/with-contenv bash
# Setup device permissions for GPU access

USER_NAME="${USER_NAME:-retro}"

# Ensure /dev/dri devices are accessible
if [ -d /dev/dri ]; then
    for device in /dev/dri/*; do
        if [ -e "$device" ]; then
            # Get the group that owns the device
            DEVICE_GID=$(stat -c '%g' "$device")
            # Add user to that group
            if [ "$DEVICE_GID" != "0" ]; then
                groupadd -g "$DEVICE_GID" "device_$DEVICE_GID" 2>/dev/null || true
                usermod -aG "device_$DEVICE_GID" "$USER_NAME" 2>/dev/null || true
            fi
            chmod 666 "$device" 2>/dev/null || true
        fi
    done
    echo "[15-setup_devices] DRI devices configured"
fi

# Ensure /dev/nvidia* devices are accessible
for device in /dev/nvidia*; do
    if [ -e "$device" ]; then
        chmod 666 "$device" 2>/dev/null || true
    fi
done

# Ensure /dev/input devices are accessible (for keyboard/mouse)
if [ -d /dev/input ]; then
    chmod -R 777 /dev/input 2>/dev/null || true
fi

echo "[15-setup_devices] Device permissions configured"
```

#### `/etc/cont-init.d/30-nvidia.sh`
Sets up NVIDIA driver symlinks and configuration.

```bash
#!/usr/bin/with-contenv bash
# Setup NVIDIA driver symlinks and configuration

# Check if NVIDIA is available
if ! command -v nvidia-smi &> /dev/null; then
    echo "[30-nvidia] NVIDIA not detected, skipping"
    exit 0
fi

# Create necessary directories
mkdir -p /usr/share/glvnd/egl_vendor.d
mkdir -p /usr/share/vulkan/icd.d
mkdir -p /etc/vulkan/icd.d

# NVIDIA EGL vendor configuration
cat > /usr/share/glvnd/egl_vendor.d/10_nvidia.json << 'EOF'
{
    "file_format_version" : "1.0.0",
    "ICD" : {
        "library_path" : "libEGL_nvidia.so.0"
    }
}
EOF

# NVIDIA Vulkan ICD configuration (for applications that need Vulkan)
NVIDIA_VERSION=$(nvidia-smi --query-gpu=driver_version --format=csv,noheader | head -1)
if [ -n "$NVIDIA_VERSION" ]; then
    cat > /usr/share/vulkan/icd.d/nvidia_icd.json << EOF
{
    "file_format_version" : "1.0.0",
    "ICD": {
        "library_path": "libGLX_nvidia.so.0",
        "api_version" : "1.3"
    }
}
EOF
fi

# Set GBM backend for NVIDIA
export __NV_PRIME_RENDER_OFFLOAD=1
export __GLX_VENDOR_LIBRARY_NAME=nvidia
export GBM_BACKEND=nvidia-drm

echo "[30-nvidia] NVIDIA configuration complete (driver version: ${NVIDIA_VERSION:-unknown})"
```

### 3. GOW Runtime Scripts (from `/opt/gow/`)

#### `/opt/gow/bash-lib/utils.sh`
Logging and utility functions.

```bash
#!/bin/bash
# GOW utility functions

# Logging helpers
log_info() {
    echo "[INFO] $*"
}

log_warn() {
    echo "[WARN] $*"
}

log_error() {
    echo "[ERROR] $*" >&2
}

# Wait for a process to be ready
wait_for_process() {
    local process_name="$1"
    local max_attempts="${2:-100}"
    local attempt=0

    while ! pgrep -x "$process_name" > /dev/null 2>&1; do
        if (( attempt++ >= max_attempts )); then
            log_error "$process_name failed to start"
            return 1
        fi
        sleep 0.1
    done
    log_info "$process_name is running"
    return 0
}
```

#### `/opt/gow/xorg.sh`
Launches Xwayland and waits for it to be ready.

```bash
#!/bin/bash
# Launch Xwayland on display :9 and wait for it to be ready
# Using :9 to avoid conflicts with Gamescope on :0

export DISPLAY=:9

function wait_for_x_display() {
    local max_attempts=100
    local attempt=0
    while ! xdpyinfo -display ":9" >/dev/null 2>&1; do
        if (( attempt++ >= max_attempts )); then
            echo "Xwayland failed to start on display :9"
            exit 1
        fi
        sleep 0.1
    done
    echo "Xwayland is ready on display :9"
}

function wait_for_dbus() {
    local max_attempts=100
    local attempt=0
    while ! dbus-send --system --dest=org.freedesktop.DBus --type=method_call --print-reply \
        /org/freedesktop/DBus org.freedesktop.DBus.ListNames >/dev/null 2>&1; do
        if (( attempt++ >= max_attempts )); then
            echo "D-Bus failed to start"
            exit 1
        fi
        sleep 0.1
    done
    echo "D-Bus is ready"
}

# Start Xwayland on display :9
echo "Starting Xwayland on display :9..."
Xwayland :9 &
wait_for_x_display

# Set screen resolution
XWAYLAND_OUTPUT=$(xrandr --display :9 | grep " connected" | awk '{print $1}')
if [ -n "$XWAYLAND_OUTPUT" ]; then
    xrandr --output "$XWAYLAND_OUTPUT" --mode "${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}"
    echo "Set resolution to ${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}"
fi

# Start D-Bus
sudo /opt/gow/dbus.sh
wait_for_dbus

# Launch desktop
exec /opt/gow/desktop.sh
```

#### `/opt/gow/dbus.sh`
Starts the system D-Bus daemon.

```bash
#!/bin/bash
# Start system D-Bus daemon
echo "Starting system D-Bus..."
service dbus start
```

#### `/opt/gow/desktop.sh`
Configures environment and launches GNOME session.

```bash
#!/bin/bash
# Configure environment and launch GNOME session
echo "=== GOW desktop.sh starting ==="

# Set up environment for GNOME on X11
export XDG_DATA_DIRS=/var/lib/flatpak/exports/share:/home/retro/.local/share/flatpak/exports/share:/usr/local/share/:/usr/share/
export XDG_CURRENT_DESKTOP=ubuntu:GNOME
export DE=ubuntu
export DESKTOP_SESSION=ubuntu
export XDG_SESSION_TYPE=x11
export XDG_SESSION_CLASS="user"
export _JAVA_AWT_WM_NONREPARENTING=1
export GDK_BACKEND=x11
export MOZ_ENABLE_WAYLAND=0
export QT_QPA_PLATFORM="xcb"
export QT_AUTO_SCREEN_SCALE_FACTOR=1
export QT_ENABLE_HIGHDPI_SCALING=1
export LC_ALL="en_US.UTF-8"

# Use display :9 (set up by xorg.sh)
export DISPLAY=:9
unset WAYLAND_DISPLAY

# Start a new D-Bus session for GNOME
export $(dbus-launch)

# Set vanilla Ubuntu scaling (no HiDPI)
gsettings set org.gnome.desktop.interface scaling-factor 1 2>/dev/null || true

echo "Launching GNOME session..."
exec /usr/bin/dbus-launch /usr/bin/gnome-session
```

#### `/opt/gow/launch-comp.sh`
Launcher function that handles Sway vs non-Sway modes.

```bash
#!/bin/bash
# GOW launcher function
# If RUN_SWAY=1, starts Sway compositor
# Otherwise, directly executes the given command

source /opt/gow/bash-lib/utils.sh

launcher() {
    if [ "$RUN_SWAY" = "1" ]; then
        log_info "Starting Sway compositor..."
        exec sway
    else
        log_info "Executing: $*"
        exec "$@"
    fi
}
```

#### `/opt/gow/startup.sh`
Entry point called by s6-overlay after init scripts complete.

```bash
#!/bin/bash
# GOW startup entry point
# This is called after all /etc/cont-init.d/ scripts have run

echo "=== GOW Startup ==="

# Switch to retro user and run the launcher
USER_NAME="${USER_NAME:-retro}"
USER_HOME="/home/$USER_NAME"

# Ensure XDG_RUNTIME_DIR exists
export XDG_RUNTIME_DIR="/run/user/$(id -u $USER_NAME)"
mkdir -p "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"
chown "$USER_NAME:$USER_NAME" "$XDG_RUNTIME_DIR"

# Run the Helix startup script as the user
exec s6-setuidgid "$USER_NAME" /opt/gow/helix-startup.sh
```

## Complete Dockerfile

```dockerfile
# ====================================================================
# Ubuntu 24.04 GNOME Desktop for Helix Personal Dev Environment
# ====================================================================
# Built from scratch with minimal GOW infrastructure
# Avoids GNOME 48/GTK 4.16 Vulkan rendering issues

# ====================================================================
# Go build stage for settings-sync-daemon, screenshot-server, revdial-client
# ====================================================================
FROM golang:1.24 AS go-build-env
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY api ./api
WORKDIR /app/api
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -buildvcs=false -ldflags "-s -w" -o /settings-sync-daemon ./cmd/settings-sync-daemon && \
    CGO_ENABLED=0 go build -buildvcs=false -ldflags "-s -w" -o /screenshot-server ./cmd/screenshot-server && \
    CGO_ENABLED=0 go build -buildvcs=false -ldflags "-s -w" -o /revdial-client ./cmd/revdial-client

# ====================================================================
# Main image: Ubuntu 24.04 (Noble) with GNOME 46
# ====================================================================
FROM ubuntu:24.04

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# ====================================================================
# Install s6-overlay init system
# ====================================================================
ARG S6_OVERLAY_VERSION=3.1.6.2
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz /tmp
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-x86_64.tar.xz /tmp
RUN tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz && \
    tar -C / -Jxpf /tmp/s6-overlay-x86_64.tar.xz && \
    rm /tmp/s6-overlay-*.tar.xz

# ====================================================================
# Install base system packages
# ====================================================================
RUN apt-get update && apt-get install -y \
    sudo \
    locales \
    ca-certificates \
    curl \
    wget \
    gnupg \
    && rm -rf /var/lib/apt/lists/*

# Configure locale
RUN locale-gen en_US.UTF-8
ENV LANG=en_US.UTF-8 \
    LANGUAGE=en_US:en \
    LC_ALL=en_US.UTF-8

# ====================================================================
# Install GPU packages (OpenGL, EGL, Mesa, Xwayland)
# ====================================================================
RUN apt-get update && apt-get install -y \
    libgl1 \
    libglx-mesa0 \
    libglx0 \
    libglvnd0 \
    libegl1 \
    libegl-mesa0 \
    libgles2 \
    libgl1-mesa-dri \
    mesa-vulkan-drivers \
    xwayland \
    x11-utils \
    x11-xserver-utils \
    && rm -rf /var/lib/apt/lists/*

# ====================================================================
# Install GNOME 46 (Ubuntu 24.04's version)
# ====================================================================
RUN apt-get update && apt-get install -y \
    gnome-session \
    gnome-shell \
    gnome-terminal \
    gnome-control-center \
    nautilus \
    dbus-x11 \
    && rm -rf /var/lib/apt/lists/*

# ====================================================================
# Install Ubuntu theming (Yaru)
# ====================================================================
RUN apt-get update && apt-get install -y \
    yaru-theme-gtk \
    yaru-theme-icon \
    yaru-theme-sound \
    ubuntu-wallpapers \
    fonts-ubuntu \
    && rm -rf /var/lib/apt/lists/*

# ====================================================================
# Install additional tools
# ====================================================================
RUN apt-get update && apt-get install -y \
    git \
    openssh-client \
    scrot \
    wmctrl \
    devilspie2 \
    xclip \
    xsel \
    && rm -rf /var/lib/apt/lists/*

# ====================================================================
# Install Firefox from Mozilla repository
# ====================================================================
RUN apt-get update && \
    mkdir -p /etc/apt/keyrings && \
    wget -q https://packages.mozilla.org/apt/repo-signing-key.gpg -O /etc/apt/keyrings/packages.mozilla.org.asc && \
    echo "deb [signed-by=/etc/apt/keyrings/packages.mozilla.org.asc] https://packages.mozilla.org/apt mozilla main" > /etc/apt/sources.list.d/mozilla.list && \
    echo "Package: firefox*\nPin: origin packages.mozilla.org\nPin-Priority: 1000" > /etc/apt/preferences.d/mozilla-firefox && \
    apt-get update && \
    apt-get install -y firefox && \
    rm -rf /var/lib/apt/lists/*

# ====================================================================
# Install Docker CLI
# ====================================================================
RUN apt-get update \
    && apt-get install -y --no-install-recommends gnupg ca-certificates curl \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && chmod a+r /etc/apt/keyrings/docker.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu noble stable" > /etc/apt/sources.list.d/docker.list \
    && apt-get update \
    && apt-get install -y docker-ce-cli docker-compose-plugin \
    && rm -rf /var/lib/apt/lists/*

# ====================================================================
# Disable systemd-logind for GNOME in containers
# ====================================================================
# GNOME session manager tries to connect to systemd-logind for session
# management. Containers don't have systemd, so this causes GNOME to fail.
RUN for file in $(find /usr -type f -iname "*login1*" 2>/dev/null); do \
    mv -v "$file" "$file.back" 2>/dev/null || true; \
    done

# Enable bubblewrap SUID for GNOME sandboxing
RUN chmod u+s /usr/bin/bwrap 2>/dev/null || true

# ====================================================================
# Set up XDG_RUNTIME_DIR
# ====================================================================
ENV XDG_RUNTIME_DIR=/run/user/1000
RUN mkdir -p /run/user/1000 && chmod 700 /run/user/1000

# ====================================================================
# Configure sudo
# ====================================================================
RUN echo "%sudo ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers && \
    echo "retro ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# ====================================================================
# Copy GOW init scripts
# ====================================================================
COPY wolf/ubuntu-config/gow-scripts/10-setup_user.sh /etc/cont-init.d/
COPY wolf/ubuntu-config/gow-scripts/15-setup_devices.sh /etc/cont-init.d/
COPY wolf/ubuntu-config/gow-scripts/30-nvidia.sh /etc/cont-init.d/
RUN chmod +x /etc/cont-init.d/*.sh

# ====================================================================
# Copy GOW runtime scripts
# ====================================================================
RUN mkdir -p /opt/gow/bash-lib
COPY wolf/ubuntu-config/gow-scripts/bash-lib/utils.sh /opt/gow/bash-lib/
COPY wolf/ubuntu-config/gow-scripts/xorg.sh /opt/gow/
COPY wolf/ubuntu-config/gow-scripts/dbus.sh /opt/gow/
COPY wolf/ubuntu-config/gow-scripts/desktop.sh /opt/gow/
COPY wolf/ubuntu-config/gow-scripts/launch-comp.sh /opt/gow/
COPY wolf/ubuntu-config/gow-scripts/startup.sh /opt/gow/
RUN chmod +x /opt/gow/*.sh /opt/gow/bash-lib/*.sh

# ====================================================================
# Copy Helix-specific binaries
# ====================================================================
COPY --from=go-build-env /settings-sync-daemon /usr/local/bin/settings-sync-daemon
COPY --from=go-build-env /screenshot-server /usr/local/bin/screenshot-server
COPY --from=go-build-env /revdial-client /usr/local/bin/revdial-client

# Copy Zed binary
RUN mkdir -p /zed-build
COPY zed-build/zed /zed-build/zed
RUN chmod +x /zed-build/zed

# Copy Helix startup scripts
ADD wolf/ubuntu-config/start-zed-helix.sh /usr/local/bin/start-zed-helix.sh
ADD wolf/ubuntu-config/helix-specs-create.sh /usr/local/bin/helix-specs-create.sh
RUN chmod +x /usr/local/bin/start-zed-helix.sh /usr/local/bin/helix-specs-create.sh

# Copy Helix startup script (renamed to helix-startup.sh)
ADD wolf/ubuntu-config/startup-app.sh /opt/gow/helix-startup.sh
RUN chmod +x /opt/gow/helix-startup.sh

# Copy dconf settings
COPY wolf/ubuntu-config/dconf-settings.ini /opt/gow/dconf-settings.ini

# Copy Docker wrappers for Hydra compatibility
COPY wolf/ubuntu-config/docker-wrapper.sh /usr/local/bin/docker-wrapper.sh
COPY wolf/ubuntu-config/docker-compose-wrapper.sh /usr/local/bin/docker-compose-wrapper.sh
RUN mv /usr/bin/docker /usr/bin/docker.real \
    && mv /usr/local/bin/docker-wrapper.sh /usr/bin/docker \
    && chmod 755 /usr/bin/docker /usr/bin/docker.real \
    && mv /usr/libexec/docker/cli-plugins/docker-compose /usr/libexec/docker/cli-plugins/docker-compose.real \
    && mv /usr/local/bin/docker-compose-wrapper.sh /usr/libexec/docker/cli-plugins/docker-compose \
    && chmod 755 /usr/libexec/docker/cli-plugins/docker-compose /usr/libexec/docker/cli-plugins/docker-compose.real

# Copy devilspie2 config
RUN mkdir -p /etc/skel/.config/devilspie2
COPY wolf/ubuntu-config/devilspie2/helix-tiling.lua /etc/skel/.config/devilspie2/helix-tiling.lua
RUN chmod 644 /etc/skel/.config/devilspie2/helix-tiling.lua

# ====================================================================
# Environment configuration
# ====================================================================
# Disable Sway (we use GNOME)
ENV RUN_SWAY=""

# ====================================================================
# Entry point
# ====================================================================
ENTRYPOINT ["/init"]
```

## Files to Create

### Directory Structure

```
wolf/ubuntu-config/
├── gow-scripts/
│   ├── bash-lib/
│   │   └── utils.sh
│   ├── 10-setup_user.sh
│   ├── 15-setup_devices.sh
│   ├── 30-nvidia.sh
│   ├── xorg.sh
│   ├── dbus.sh
│   ├── desktop.sh
│   ├── launch-comp.sh
│   └── startup.sh
├── startup-app.sh        (existing - becomes helix-startup.sh)
├── start-zed-helix.sh    (existing)
├── helix-specs-create.sh (existing)
├── dconf-settings.ini    (existing)
├── docker-wrapper.sh     (existing)
├── docker-compose-wrapper.sh (existing)
└── devilspie2/
    └── helix-tiling.lua  (existing)
```

### Modified `startup-app.sh`

The existing `startup-app.sh` needs to be simplified at the end. Replace the GNOME startup section with:

```bash
# ============================================================================
# GNOME Session Startup (Zorin-compatible pattern)
# ============================================================================
# Use GOW scripts which properly initialize Xwayland on :9,
# D-Bus, and GNOME session with hardware GPU rendering.

echo "Launching GNOME via GOW xorg.sh..."
exec /opt/gow/xorg.sh
```

**Remove** the following from `startup-app.sh`:
- All software rendering flags (`LIBGL_ALWAYS_SOFTWARE`, `MUTTER_DEBUG_*`, etc.)
- The `source /opt/gow/bash-lib/utils.sh` line
- The environment variable setup section at the end
- The complex `exec dbus-run-session -- bash -E -c "..."` line

## Implementation Steps

### Step 1: Create GOW Scripts Directory

```bash
mkdir -p wolf/ubuntu-config/gow-scripts/bash-lib
```

### Step 2: Create Init Scripts

Create each file in `wolf/ubuntu-config/gow-scripts/`:
- `10-setup_user.sh` (content above)
- `15-setup_devices.sh` (content above)
- `30-nvidia.sh` (content above)

### Step 3: Create Runtime Scripts

Create each file in `wolf/ubuntu-config/gow-scripts/`:
- `bash-lib/utils.sh` (content above)
- `xorg.sh` (content above)
- `dbus.sh` (content above)
- `desktop.sh` (content above)
- `launch-comp.sh` (content above)
- `startup.sh` (content above)

### Step 4: Simplify `startup-app.sh`

Edit `wolf/ubuntu-config/startup-app.sh` to end with the simple `exec /opt/gow/xorg.sh` call as shown above.

### Step 5: Replace Dockerfile

Replace `Dockerfile.ubuntu-helix` with the complete Dockerfile above.

### Step 6: Build and Test

```bash
# Commit changes first (required for image tag to update)
git add -A && git commit -m "feat(ubuntu): rebuild from Ubuntu 24.04 with minimal GOW"

# Build the Ubuntu image
./stack build-ubuntu

# Launch a new Ubuntu sandbox session and test
```

## Verification Checklist

After implementation, verify:

- [ ] Container starts without errors
- [ ] GNOME desktop fully renders (not just loading animation)
- [ ] No EGL/GLX errors in container logs
- [ ] Hardware GPU acceleration working: `glxinfo | grep "direct rendering"` → should show "Yes"
- [ ] Ubuntu theming correct (Yaru theme, Ubuntu wallpaper)
- [ ] Zed launches and connects to settings-sync-daemon
- [ ] Terminal launches
- [ ] Firefox launches
- [ ] Window tiling works (Terminal, Zed, Firefox in thirds)
- [ ] Screenshot server responds
- [ ] Settings sync daemon works

## Debugging Commands

**Check container logs:**
```bash
docker compose -f docker-compose.dev.yaml logs -f sandbox
```

**Get into a running container:**
```bash
docker exec -it <container_id> bash
```

**Check GPU rendering:**
```bash
# Inside container
glxinfo | grep "direct rendering"
# Should output: direct rendering: Yes
```

**View startup debug log:**
```bash
# Inside container
cat /tmp/ubuntu-startup-debug.log
```

**Check GNOME version:**
```bash
gnome-shell --version
# Should output: GNOME Shell 46.x (not 48.x)
```

**Check Mutter version:**
```bash
mutter --version
# Should output: mutter 46.x (not 48.x)
```

## Alternative Quick Fix (If Needed)

If you need to try Ubuntu 25.04 first before the full rebuild, you can try forcing OpenGL rendering:

```bash
# Add to environment in desktop.sh
export GSK_RENDERER=ngl
```

This forces GTK 4.16 to use OpenGL instead of Vulkan. However, the Ubuntu 24.04 approach is more reliable and tested.

## Reference Links

- [s6-overlay GitHub](https://github.com/just-containers/s6-overlay)
- [Games on Whales](https://github.com/games-on-whales)
- [GTK 4.16 Vulkan Issue](https://gitlab.gnome.org/GNOME/gtk/-/issues/6589)
- [GNOME on Xwayland](https://wiki.gnome.org/Projects/Mutter/Wayland)
