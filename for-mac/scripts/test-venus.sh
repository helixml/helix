#!/bin/bash
# Test Venus/Vulkan support in the VM
# Run this inside the Ubuntu VM after installation

set -e

echo "=== Testing Venus/Vulkan Support ==="

# Check if Vulkan is available
if ! command -v vulkaninfo &> /dev/null; then
    echo "Installing vulkan-tools..."
    sudo apt update
    sudo apt install -y vulkan-tools mesa-vulkan-drivers
fi

echo ""
echo "=== Vulkan Info ==="
vulkaninfo --summary 2>&1 | head -30

echo ""
echo "=== Checking for Venus driver ==="
vulkaninfo 2>&1 | grep -i "venus\|virtio" || echo "Venus not detected in driver name"

echo ""
echo "=== GPU Device ==="
vulkaninfo 2>&1 | grep -A5 "GPU id"

echo ""
echo "=== Testing vkcube (press Ctrl+C to stop) ==="
echo "If this works, Venus is functioning correctly!"
timeout 10 vkcube 2>&1 || true

echo ""
echo "=== Venus Test Complete ==="
