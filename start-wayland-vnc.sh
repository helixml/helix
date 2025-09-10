#!/bin/bash
set -e

# Hyprland-only startup script for 4K@60Hz with NVIDIA GPU acceleration

echo "Starting Hyprland compositor with NVIDIA GPU acceleration..."

# Set up environment for Wayland GPU acceleration
export WLR_RENDERER=${WLR_RENDERER:-gles2}
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

# Wolf-style render node configuration for GPU acceleration
export WOLF_RENDER_NODE=/dev/dri/renderD128
export GST_GL_DRM_DEVICE=${GST_GL_DRM_DEVICE:-$WOLF_RENDER_NODE}

# Wolf's GST GPU configuration
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

# Start Hyprland as Ubuntu user
su ubuntu -c "
export USER=ubuntu
export HOME=/home/ubuntu
export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu
mkdir -p /tmp/runtime-ubuntu
chmod 700 /tmp/runtime-ubuntu

# Export GPU environment variables
export WLR_RENDERER=\"$WLR_RENDERER\"
export WLR_BACKENDS=\"$WLR_BACKENDS\"
export WLR_NO_HARDWARE_CURSORS=\"$WLR_NO_HARDWARE_CURSORS\"
export WLR_HEADLESS_OUTPUTS=\"$WLR_HEADLESS_OUTPUTS\"
export __GL_THREADED_OPTIMIZATIONS=\"$__GL_THREADED_OPTIMIZATIONS\"
export __GL_SYNC_TO_VBLANK=\"$__GL_SYNC_TO_VBLANK\"
export LIBGL_ALWAYS_SOFTWARE=\"$LIBGL_ALWAYS_SOFTWARE\"
export GBM_BACKEND=\"$GBM_BACKEND\"
export __GLX_VENDOR_LIBRARY_NAME=\"$__GLX_VENDOR_LIBRARY_NAME\"
export WLR_DRM_NO_ATOMIC=\"$WLR_DRM_NO_ATOMIC\"
export NVIDIA_VISIBLE_DEVICES=\"$NVIDIA_VISIBLE_DEVICES\"
export NVIDIA_DRIVER_CAPABILITIES=\"$NVIDIA_DRIVER_CAPABILITIES\"
export EGL_PLATFORM=\"$EGL_PLATFORM\"
export WLR_DRM_DEVICES=\"$WLR_DRM_DEVICES\"
export WLR_RENDERER_ALLOW_SOFTWARE=\"$WLR_RENDERER_ALLOW_SOFTWARE\"
export LIBGL_DRIVERS_PATH=\"$LIBGL_DRIVERS_PATH\"
export LIBVA_DRIVERS_PATH=\"$LIBVA_DRIVERS_PATH\"
export WOLF_RENDER_NODE=\"$WOLF_RENDER_NODE\"
export GST_GL_DRM_DEVICE=\"$GST_GL_DRM_DEVICE\"

# Start dbus session
if [ -z \"\$DBUS_SESSION_BUS_ADDRESS\" ]; then
    eval \$(dbus-launch --sh-syntax)
fi

# Create crash report directory for Hyprland
mkdir -p /home/ubuntu/.cache/hyprland
chown ubuntu:ubuntu /home/ubuntu/.cache/hyprland

# Enhanced Hyprland startup with NVIDIA GPU acceleration
echo \"Starting Hyprland with NVIDIA RTX 4090 GPU acceleration...\"

# Set up comprehensive environment for Hyprland NVIDIA support
export HYPRLAND_LOG_WLR=1
export HYPRLAND_NO_RT=1
export WLR_RENDERER_ALLOW_SOFTWARE=1

# Pre-flight GPU check
echo \"Pre-flight GPU check:\"
echo \"DRI devices: \$(ls /dev/dri/)\"
echo \"NVIDIA_VISIBLE_DEVICES: \$NVIDIA_VISIBLE_DEVICES\"
echo \"GBM_BACKEND: \$GBM_BACKEND\"

# Start Hyprland with comprehensive error capture
echo \"Starting Hyprland...\"
(Hyprland 2>&1 | while read line; do echo \"HYPRLAND: \$line\"; done) &
COMPOSITOR_PID=\$!

# Give Hyprland time to initialize with GPU
sleep 5

# Check if Hyprland is still running
if ! kill -0 \$COMPOSITOR_PID 2>/dev/null; then
    echo \"❌ Hyprland failed to start with NVIDIA GPU acceleration\"
    exit 1
else
    echo \"✅ Hyprland started successfully with NVIDIA GPU acceleration!\"
fi

# Start wayvnc VNC server on the Wayland display
echo \"Starting wayvnc VNC server...\"
# Wait for Wayland display to be ready
sleep 1

# Find the actual Wayland display socket
if [ -S \"\$XDG_RUNTIME_DIR/wayland-1\" ]; then
    export WAYLAND_DISPLAY=wayland-1
elif [ -S \"\$XDG_RUNTIME_DIR/wayland-0\" ]; then
    export WAYLAND_DISPLAY=wayland-0
fi

echo \"Using Wayland display: \$WAYLAND_DISPLAY\"

# Start wayvnc with input enabled and cursor optimizations for VNC
wayvnc --max-fps 120 --show-performance --disable-resizing 0.0.0.0 5901 &
WAYVNC_PID=\$!

echo \"VNC server started on port 5901\"

# Start Wolf Moonlight server
echo \"Starting Wolf Moonlight server...\"
/start-moonlight.sh &
MOONLIGHT_PID=\$!

echo \"Moonlight server started on ports 47984 (HTTPS), 47989 (HTTP), 47999 (Control), 48010 (RTSP), 48100 (Video), 48200 (Audio)\"

# Set up environment for AGS
echo \"Setting up AGS environment...\"
export DISPLAY=:1
export HYPRLAND_INSTANCE_SIGNATURE=\$(ls \$XDG_RUNTIME_DIR/hypr/ | tail -1)

# Create additional workspaces for better desktop experience
echo \"Creating additional workspaces...\"
for i in 2 3 4 5; do
    hyprctl dispatch workspace \$i 2>/dev/null || true
done
hyprctl dispatch workspace 1 2>/dev/null || true

# Wait a bit more for Hyprland to fully initialize
sleep 3

# Start AGS with proper environment and unique bus name
echo \"Starting AGS bar and dock...\"
(agsv1 --bus-name \"ags-\$(date +%s)\" 2>&1 | while read line; do echo \"AGS: \$line\"; done) &
AGS_PID=\$!

# Wait for all processes
wait \$COMPOSITOR_PID \$WAYVNC_PID \$MOONLIGHT_PID
" &

# Start Helix agent in background
# /start-helix-agent.sh &

# Keep container running
tail -f /dev/null
