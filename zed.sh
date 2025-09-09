#!/bin/bash
# Zed launcher script with software rendering for RDP/headless environments

# Force software rendering and try to bypass Vulkan validation
export ZED_ALLOW_EMULATED_GPU=1
export ZED_ALLOW_ROOT=1
export LIBGL_ALWAYS_SOFTWARE=1
export GALLIUM_DRIVER=llvmpipe
export VK_ICD_FILENAMES=/usr/share/vulkan/icd.d/lvp_icd.x86_64.json
export MESA_LOADER_DRIVER_OVERRIDE=llvmpipe
export VK_LOADER_DEBUG=none
export VK_INSTANCE_LAYERS=""
export RUST_BACKTRACE=1

# Debug info
echo "Starting Zed with software rendering..."
echo "DISPLAY: $DISPLAY"
echo "Vulkan ICD: $VK_ICD_FILENAMES"

# Check what Vulkan drivers are available
echo "Available Vulkan ICDs:"
ls -la /usr/share/vulkan/icd.d/ 2>/dev/null || echo "No Vulkan ICDs found"
vulkan-tools.vulkaninfo 2>/dev/null | head -10 || echo "vulkaninfo failed"

# Check if mounted Zed binary exists
if [ ! -f "/zed-build/zed" ]; then
    echo "ERROR: Zed binary not found at /zed-build/zed"
    echo "Run './stack build-zed' on the host first"
    exit 1
fi

# Run Zed with all arguments passed through
exec /usr/local/bin/zed "$@"
