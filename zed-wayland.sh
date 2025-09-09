#!/bin/bash
# Zed launcher script for Wayland with GPU acceleration

# Debug info
echo "Starting Zed with Wayland GPU acceleration..."
echo "WAYLAND_DISPLAY: $WAYLAND_DISPLAY"
echo "XDG_RUNTIME_DIR: $XDG_RUNTIME_DIR"
echo "WLR_RENDERER: $WLR_RENDERER"

# Check if mounted Zed binary exists
if [ ! -f "/zed-build/zed" ]; then
    echo "ERROR: Zed binary not found at /zed-build/zed"
    echo "Run './stack build-zed' on the host first"
    exit 1
fi

# Set up GPU environment for Wayland
export LIBGL_ALWAYS_SOFTWARE=0
export WLR_RENDERER=gles2
export GBM_BACKEND=nvidia-drm

# Enable GPU acceleration for applications
export __GL_SYNC_TO_VBLANK=1
export __GL_THREADED_OPTIMIZATIONS=1

# Run Zed with native Wayland support
echo "Launching Zed with native Wayland GPU acceleration..."
exec /usr/local/bin/zed "$@"