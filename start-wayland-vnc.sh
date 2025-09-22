#!/bin/bash
set -e

# Stock Hyprland + HyprMoon external screencopy startup script for 4K@60Hz with NVIDIA GPU acceleration

# Lock file to prevent multiple concurrent executions
LOCK_FILE="/tmp/start-wayland-vnc.lock"

# Check if already running
if [ -f "$LOCK_FILE" ]; then
    echo "start-wayland-vnc.sh is already running (lock file exists: $LOCK_FILE)"
    echo "If this is incorrect, remove $LOCK_FILE and try again"
    exit 1
fi

# Create lock file with PID
echo $$ > "$LOCK_FILE"

# Cleanup lock file on exit
trap 'rm -f "$LOCK_FILE"' EXIT

# === HYPRMOON VERSION INFORMATION ===
echo "================================================================"
echo "HYPRMOON VERSION CHECK - CRITICAL DEBUGGING INFORMATION"
echo "================================================================"
echo "Checking what version of HyprMoon/Hyprland is actually installed..."

# Check for stock Hyprland and HyprMoon external screencopy
if dpkg -l hyprland >/dev/null 2>&1; then
    echo "✅ STOCK HYPRLAND FOUND:"
    dpkg -l hyprland | grep "^ii" || echo "Package info not available"
    echo ""
    echo "Hyprland binary location:"
    which Hyprland || echo "❌ Hyprland binary not found in PATH"
    ls -la /usr/bin/Hyprland 2>/dev/null || echo "❌ /usr/bin/Hyprland not found"
else
    echo "❌ STOCK HYPRLAND PACKAGE NOT FOUND!"
    echo "⚠️ Continuing anyway - checking for any Hyprland binary..."
    if which Hyprland >/dev/null 2>&1; then
        echo "✅ Found Hyprland binary in PATH"
    else
        echo "❌ No Hyprland binary found anywhere"
        echo "⚠️ Will continue for debugging purposes"
    fi
fi

# Check for HyprMoon with integrated screencopy
if dpkg -l hyprmoon >/dev/null 2>&1; then
    echo "✅ HYPRMOON WITH INTEGRATED SCREENCOPY FOUND:"
    dpkg -l hyprmoon | grep "^ii" || echo "Package info not available"
    echo ""
    echo "HyprMoon binary location:"
    which Hyprland || echo "❌ Hyprland binary not found in PATH"
    ls -la /usr/bin/Hyprland 2>/dev/null || echo "❌ /usr/bin/Hyprland not found"
    echo "🔧 FORCING STOCK HYPRLAND MODE FOR VNC TESTING"
    HYPRMOON_MODE=false
else
    echo "❌ HyprMoon not found - falling back to stock Hyprland"
    HYPRMOON_MODE=false
fi

echo ""
echo "All hypr-related packages installed:"
dpkg -l | grep hypr || echo "❌ No hypr packages found"
echo "================================================================"
echo ""

if [ "$HYPRMOON_MODE" = "true" ]; then
    echo "Starting HyprMoon with integrated screencopy backend and NVIDIA GPU acceleration..."
else
    echo "Starting stock Hyprland compositor with NVIDIA GPU acceleration..."
fi

# Set up environment for Wayland GPU acceleration
# Force OpenGL instead of Vulkan to fix Sunshine wlroots capture (GitHub issue #4050)
export WLR_RENDERER=gles2
export WLR_BACKENDS=${WLR_BACKENDS:-headless}
export WLR_NO_HARDWARE_CURSORS=${WLR_NO_HARDWARE_CURSORS:-1}
export WLR_HEADLESS_OUTPUTS=${WLR_HEADLESS_OUTPUTS:-1}


# Set GPU acceleration settings
export __GL_THREADED_OPTIMIZATIONS=1
export __GL_SYNC_TO_VBLANK=1
export LIBGL_ALWAYS_SOFTWARE=0

# Core NVIDIA environment variables for container runtime
export NVIDIA_VISIBLE_DEVICES=all
export NVIDIA_DRIVER_CAPABILITIES=all

# Standard DRI/Mesa configuration for container GPU access
export LIBGL_DRIVERS_PATH="/usr/lib/x86_64-linux-gnu/dri"
export LIBVA_DRIVERS_PATH="/usr/lib/x86_64-linux-gnu/dri"

# Minimal EGL configuration for container environments
export EGL_PLATFORM=drm
export GBM_BACKEND=nvidia-drm
export __GLX_VENDOR_LIBRARY_NAME=nvidia

# wlroots GPU configuration
export WLR_DRM_DEVICES=/dev/dri/card0
export WLR_RENDERER_ALLOW_SOFTWARE=1
export WLR_DRM_NO_ATOMIC=1

# GPU render node configuration for hardware acceleration
export GPU_RENDER_NODE=/dev/dri/renderD128
export GST_GL_DRM_DEVICE=${GST_GL_DRM_DEVICE:-$GPU_RENDER_NODE}

# GStreamer GPU configuration for hardware encoding
export GST_GL_API=gles2
export GST_GL_WINDOW=surfaceless

# GPU detection and configuration
echo "Detecting GPU configuration..."

# Check for DRI devices
if ls /dev/dri/card* >/dev/null 2>&1; then
    DRI_CARD=$(ls /dev/dri/card* | head -1)
    echo "DRI device $DRI_CARD detected, attempting hardware acceleration"
    export WLR_DRM_DEVICES=$DRI_CARD

    # Check if lspci works and detects GPU
    if command -v lspci >/dev/null 2>&1 && lspci | grep -i nvidia >/dev/null 2>&1; then
        echo "NVIDIA GPU detected:"
        lspci | grep -i nvidia | head -1
        echo "Configured for NVIDIA RTX 4090 GPU acceleration"
    fi
else
    echo "Warning: No DRI devices detected"
fi

# Ensure proper ownership of config directories
mkdir -p /home/ubuntu/.config
chown -R ubuntu:ubuntu /home/ubuntu/.config

# Start Ubuntu user session with VNC failure tolerance for Moonlight testing
echo "Starting Ubuntu user session with VNC failure tolerance..."
su ubuntu -c "
export USER=ubuntu
export HOME=/home/ubuntu
export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu
mkdir -p /tmp/runtime-ubuntu
chmod 700 /tmp/runtime-ubuntu

# Export GPU environment variables
export WLR_RENDERER=\"\$WLR_RENDERER\"
export WLR_BACKENDS=\"\$WLR_BACKENDS\"
export WLR_NO_HARDWARE_CURSORS=\"\$WLR_NO_HARDWARE_CURSORS\"
export WLR_HEADLESS_OUTPUTS=\"\$WLR_HEADLESS_OUTPUTS\"
export __GL_THREADED_OPTIMIZATIONS=\"\$__GL_THREADED_OPTIMIZATIONS\"
export __GL_SYNC_TO_VBLANK=\"\$__GL_SYNC_TO_VBLANK\"
export LIBGL_ALWAYS_SOFTWARE=\"\$LIBGL_ALWAYS_SOFTWARE\"
export GBM_BACKEND=\"\$GBM_BACKEND\"
export __GLX_VENDOR_LIBRARY_NAME=\"\$__GLX_VENDOR_LIBRARY_NAME\"
export WLR_DRM_NO_ATOMIC=\"\$WLR_DRM_NO_ATOMIC\"
export NVIDIA_VISIBLE_DEVICES=\"\$NVIDIA_VISIBLE_DEVICES\"
export NVIDIA_DRIVER_CAPABILITIES=\"\$NVIDIA_DRIVER_CAPABILITIES\"
export EGL_PLATFORM=\"\$EGL_PLATFORM\"
export WLR_DRM_DEVICES=\"\$WLR_DRM_DEVICES\"
export WLR_RENDERER_ALLOW_SOFTWARE=\"\$WLR_RENDERER_ALLOW_SOFTWARE\"
export LIBGL_DRIVERS_PATH=\"\$LIBGL_DRIVERS_PATH\"
export LIBVA_DRIVERS_PATH=\"\$LIBVA_DRIVERS_PATH\"
export GPU_RENDER_NODE=\"\$GPU_RENDER_NODE\"
export GST_GL_DRM_DEVICE=\"\$GST_GL_DRM_DEVICE\"

# Export HyprMoon mode selection and configuration
export HYPRMOON_MODE=\"$HYPRMOON_MODE\"
export HYPRMOON_FRAME_SOURCE=\"$HYPRMOON_FRAME_SOURCE\"
export HYPRMOON_WAYLAND_DISPLAY=\"$HYPRMOON_WAYLAND_DISPLAY\"
export HYPRMOON_DEBUG_SAVE_FRAMES=\"$HYPRMOON_DEBUG_SAVE_FRAMES\"

# Start dbus session and capture environment properly
if [ -z \"\$DBUS_SESSION_BUS_ADDRESS\" ]; then
    # Start D-Bus session and capture the environment
    DBUS_LAUNCH_OUTPUT=\$(dbus-launch --sh-syntax)
    eval \"\$DBUS_LAUNCH_OUTPUT\"
    echo \"D-Bus session started: \$DBUS_SESSION_BUS_ADDRESS\"
fi

# Ensure D-Bus environment is properly exported
export DBUS_SESSION_BUS_ADDRESS
export DBUS_SESSION_BUS_PID

# Create crash report directory for Hyprland
mkdir -p /home/ubuntu/.cache/hyprland
chown ubuntu:ubuntu /home/ubuntu/.cache/hyprland

if [ \"\$HYPRMOON_MODE\" = \"true\" ]; then
    # HyprMoon with integrated screencopy
    echo \"Starting HyprMoon with integrated screencopy backend...\"

    # Set up comprehensive environment for HyprMoon NVIDIA support
    export HYPRLAND_LOG_WLR=1
    export HYPRLAND_NO_RT=1
    export WLR_RENDERER_ALLOW_SOFTWARE=1

    # HyprMoon screencopy configuration
    export HYPRMOON_FRAME_SOURCE=screencopy
    export HYPRMOON_WAYLAND_DISPLAY=wayland-0
    export HYPRMOON_DEBUG_SAVE_FRAMES=1

    echo \"HyprMoon screencopy environment:\"
    echo \"  HYPRMOON_FRAME_SOURCE: \$HYPRMOON_FRAME_SOURCE\"
    echo \"  HYPRMOON_WAYLAND_DISPLAY: \$HYPRMOON_WAYLAND_DISPLAY\"
    echo \"  HYPRMOON_DEBUG_SAVE_FRAMES: \$HYPRMOON_DEBUG_SAVE_FRAMES\"

    # Pre-flight GPU check
    echo \"Pre-flight GPU check:\"
    echo \"DRI devices: \$(ls /dev/dri/)\"
    echo \"NVIDIA_VISIBLE_DEVICES: \$NVIDIA_VISIBLE_DEVICES\"
    echo \"GBM_BACKEND: \$GBM_BACKEND\"

    # Start HyprMoon with comprehensive error capture and debug config
    echo \"Starting HyprMoon with integrated screencopy...\"
    export PATH=\"/usr/bin:/usr/local/bin:\$PATH\"

    echo \"HyprMoon command: /usr/bin/Hyprland\"
    /usr/bin/Hyprland &
    COMPOSITOR_PID=\$!
    echo \"HyprMoon started with PID: \$COMPOSITOR_PID\"
else
    # Enhanced Hyprland startup with NVIDIA GPU acceleration
    echo \"Starting stock Hyprland with NVIDIA GPU acceleration...\"

    # Set up comprehensive environment for Hyprland NVIDIA support
    export HYPRLAND_LOG_WLR=1
    export HYPRLAND_NO_RT=1
    export WLR_RENDERER_ALLOW_SOFTWARE=1

    # Pre-flight GPU check
    echo \"Pre-flight GPU check:\"
    echo \"DRI devices: \$(ls /dev/dri/)\"
    echo \"NVIDIA_VISIBLE_DEVICES: \$NVIDIA_VISIBLE_DEVICES\"
    echo \"GBM_BACKEND: \$GBM_BACKEND\"

    # Start Hyprland with comprehensive error capture and debug config
    echo \"Starting Hyprland with debug logging...\"
    export PATH=\"/usr/bin:/usr/local/bin:\$PATH\"

    echo \"Hyprland command: /usr/bin/Hyprland\"
    /usr/bin/Hyprland &
    COMPOSITOR_PID=\$!
    echo \"Hyprland started with PID: \$COMPOSITOR_PID\"
fi

# Give Hyprland time to initialize with GPU
sleep 5

# Check if Hyprland is running (either our PID or any Hyprland process)
if pgrep -x \"Hyprland\" >/dev/null 2>&1; then
    if [ \"\$HYPRMOON_MODE\" = \"true\" ]; then
        echo \"✅ HyprMoon is running successfully with integrated screencopy backend!\"
        echo \"🎯 Moonlight streaming with screencopy available on port 47989 (HTTP)\"
        echo \"📸 Screencopy frame capture with debug saving to /tmp/hyprmoon_frame_dumps\"
        echo \"🌙 HyprMoon integrated server active with Wolf streaming engine\"
    else
        echo \"✅ Stock Hyprland is running successfully with NVIDIA GPU acceleration!\"
        echo \"⚠️ No Moonlight streaming available (HyprMoon not installed)\"
    fi
else
    echo \"❌ Hyprland failed to start with NVIDIA GPU acceleration\"
    # DO NOT EXIT - continue for testing
    echo \"⚠️ Continuing anyway\"
fi

# Try to start VNC but don't fail if it doesn't work
echo \"Attempting to start wayvnc VNC server...\"
sleep 1

# Find the actual Wayland display socket and create symlinks
# Hyprland creates sockets in subdirectories, try to find them
HYPR_SOCKET_DIR=\$(ls -t \"\$XDG_RUNTIME_DIR/hypr/\"*/.socket.sock 2>/dev/null | head -1 | xargs dirname 2>/dev/null)
if [ -n \"\$HYPR_SOCKET_DIR\" ] && [ -S \"\$HYPR_SOCKET_DIR/.socket.sock\" ]; then
    echo \"Found Hyprland socket: \$HYPR_SOCKET_DIR/.socket.sock\"

    # Create symlinks for standard wayland socket names for VNC and screencopy
    ln -sf \"\$HYPR_SOCKET_DIR/.socket.sock\" \"\$XDG_RUNTIME_DIR/wayland-0\" 2>/dev/null || true
    ln -sf \"\$HYPR_SOCKET_DIR/.socket.sock\" \"\$XDG_RUNTIME_DIR/wayland-1\" 2>/dev/null || true

    export WAYLAND_DISPLAY=wayland-1
    echo \"Created wayland symlinks, using WAYLAND_DISPLAY=wayland-1\"
    echo \"Debug: Socket points to \$HYPR_SOCKET_DIR/.socket.sock\"
    ls -la \"\$XDG_RUNTIME_DIR/wayland-1\" 2>/dev/null || echo \"Symlink creation failed\"
elif [ -S \"\$XDG_RUNTIME_DIR/wayland-1\" ]; then
    export WAYLAND_DISPLAY=wayland-1
elif [ -S \"\$XDG_RUNTIME_DIR/wayland-0\" ]; then
    export WAYLAND_DISPLAY=wayland-0
else
    echo \"⚠️ No Wayland display socket found, VNC will not work\"
fi

echo \"Using Wayland display: \$WAYLAND_DISPLAY\"

# Disable Wayland security features that would require user permission prompts in containerized environment
export SUNSHINE_DISABLE_WAYLAND_SECURITY=1
export WLR_ALLOW_ALL_CLIENTS=1

# Start wayvnc with input enabled and cursor optimizations for VNC
echo \"Starting wayvnc on port 5901...\"
wayvnc --max-fps 120 --show-performance --disable-resizing 0.0.0.0 5901 &
WAYVNC_PID=\$!

# Wait a moment and verify wayvnc is running on the correct port
sleep 2
if lsof -i :5901 >/dev/null 2>&1; then
    echo \"✅ VNC server started successfully on port 5901\"
else
    echo \"❌ VNC server failed to start on port 5901\"
    echo \"⚠️ VNC failure - continuing anyway to allow Moonlight server initialization\"
    # DO NOT EXIT - allow Moonlight to work without VNC
fi

if [ \"\$HYPRMOON_MODE\" = \"true\" ]; then
    echo \"🚀 HyprMoon integrated server active with screencopy backend\"
else
    echo \"Stock Hyprland active (no Moonlight streaming)\"
fi

# Keep running to allow Moonlight server to complete initialization
echo \"Keeping session alive for Moonlight server initialization...\"
tail -f /dev/null
" &

# Start Helix agent in background
/start-helix-agent.sh &

# Keep container running
tail -f /dev/null
