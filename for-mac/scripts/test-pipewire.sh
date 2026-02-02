#!/bin/bash
# Test PipeWire ScreenCast capture in the VM
# Run this inside the Ubuntu VM after installation

set -e

echo "=== Testing PipeWire ScreenCast ==="

# Install dependencies
echo "Installing dependencies..."
sudo apt update
sudo apt install -y \
    pipewire \
    pipewire-pulse \
    wireplumber \
    gstreamer1.0-pipewire \
    gstreamer1.0-tools \
    xdg-desktop-portal-gnome \
    libpipewire-0.3-dev

# Check PipeWire status
echo ""
echo "=== PipeWire Status ==="
systemctl --user status pipewire pipewire-pulse wireplumber 2>&1 | head -20 || true

# List PipeWire nodes
echo ""
echo "=== PipeWire Nodes ==="
pw-cli list-objects 2>&1 | head -30 || echo "pw-cli not available"

# Test ScreenCast portal
echo ""
echo "=== Testing ScreenCast Portal ==="
echo "This will request screen capture permission..."

# Create a simple GStreamer test pipeline
echo ""
echo "=== GStreamer PipeWire Test ==="
echo "Running: gst-launch-1.0 pipewiresrc ! videoconvert ! autovideosink"
echo "(Press Ctrl+C to stop)"

# This should open a permission dialog and then show the screen
timeout 15 gst-launch-1.0 pipewiresrc ! videoconvert ! autovideosink 2>&1 || true

echo ""
echo "=== PipeWire Test Complete ==="
