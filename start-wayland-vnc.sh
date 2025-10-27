#!/bin/bash
set -e

# Hyprland-only startup script for 4K@60Hz with NVIDIA GPU acceleration

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

echo "Starting Hyprland compositor with NVIDIA GPU acceleration..."

# Set up environment for Wayland GPU acceleration
# Force OpenGL instead of Vulkan to fix Sunshine wlroots capture (GitHub issue #4050)
export WLR_RENDERER=gles2
export WLR_BACKENDS=${WLR_BACKENDS:-headless}
export WLR_NO_HARDWARE_CURSORS=${WLR_NO_HARDWARE_CURSORS:-1}
export WLR_HEADLESS_OUTPUTS=${WLR_HEADLESS_OUTPUTS:-1}

# Prevent Vulkan zink driver which breaks Sunshine capture - use NVIDIA OpenGL instead
export VK_ICD_FILENAMES=""
export VK_DRIVER_FILES=""
export DISABLE_VK_LAYER_NV_optimus_1=1

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

# Start Ubuntu user session
echo "Starting Ubuntu user session..."
su ubuntu -c "/start-ubuntu-session.sh" &

# Start Helix agent in background
/start-helix-agent.sh &

# Keep container running
tail -f /dev/null