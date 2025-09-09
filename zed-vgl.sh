#!/bin/bash
# Zed launcher script with VirtualGL GPU acceleration

# Set up VirtualGL environment with safe display isolation
export DISPLAY=:1
export VGL_DISPLAY=:0

# Debug info
echo "Starting Zed with VirtualGL GPU acceleration..."
echo "DISPLAY: $DISPLAY"
echo "VGL_DISPLAY: $VGL_DISPLAY"

# Check if mounted Zed binary exists
if [ ! -f "/zed-build/zed" ]; then
    echo "ERROR: Zed binary not found at /zed-build/zed"
    echo "Run './stack build-zed' on the host first"
    exit 1
fi

# Check VirtualGL status (works great with TigerVNC)
echo "VirtualGL status:"
/opt/VirtualGL/bin/vglconnect -s 2>/dev/null || echo "VirtualGL connection check failed (normal in containers)"

# Allow Zed to use software GPU rendering (recommended for VNC/remote desktop)
export ZED_ALLOW_EMULATED_GPU=1

# Set up Vulkan software rendering (works better in VirtualGL containers)
export VK_ICD_FILENAMES="/usr/share/vulkan/icd.d/lvp_icd.x86_64.json"
export MESA_LOADER_DRIVER_OVERRIDE=llvmpipe

# Run Zed with software Vulkan (which still uses VirtualGL for OpenGL acceleration)
echo "Launching Zed with software Vulkan rendering (optimized for remote desktop)..."
exec /opt/VirtualGL/bin/vglrun -d :0 /usr/local/bin/zed "$@"