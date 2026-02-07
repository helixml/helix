#!/bin/bash
# Test Mutter with DRM lease via logind stub
# Run this on the VM (helix-vm)
set -e

echo "=== Testing Mutter with DRM Lease ==="

# Step 1: Ensure helix-drm-manager is running
if ! pgrep -x helix-drm-manager > /dev/null; then
    echo "Starting helix-drm-manager..."
    sudo /usr/local/bin/helix-drm-manager &
    sleep 2
fi

# Step 2: Get a DRM lease
echo "Requesting DRM lease..."
LEASE_OUTPUT=$(sudo /usr/local/bin/drm-test-client request 2>&1)
echo "$LEASE_OUTPUT"

SCANOUT_ID=$(echo "$LEASE_OUTPUT" | grep "Scanout ID" | awk '{print $NF}')
echo "Got scanout ID: $SCANOUT_ID"

# The lease FD is received by drm-test-client but immediately closed.
# For the real flow, we need a process that:
# 1. Gets the lease FD from helix-drm-manager
# 2. Keeps it open
# 3. Passes it to logind-stub
# 4. Starts gnome-shell

# For now, let's just test the logind stub standalone
echo ""
echo "=== Testing logind stub ==="

# Stop real logind
echo "Stopping real logind..."
sudo systemctl stop systemd-logind 2>/dev/null || true
sleep 1

# Start logind-stub with a dummy FD (just to test D-Bus registration)
echo "Starting logind-stub..."

# We need an actual DRM lease FD. Let's use a simple approach:
# Open /dev/dri/card0 and pass it as FD 7
exec 7</dev/dri/card0
sudo /usr/local/bin/logind-stub --lease-fd=7 &
STUB_PID=$!
sleep 2

# Test: can we query the stub?
echo "Testing D-Bus query..."
gdbus call --system \
    --dest org.freedesktop.login1 \
    --object-path /org/freedesktop/login1/session/auto \
    --method org.freedesktop.DBus.Properties.Get \
    org.freedesktop.login1.Session Active 2>&1 || echo "D-Bus query failed"

# Cleanup
echo "Cleaning up..."
sudo kill $STUB_PID 2>/dev/null || true
exec 7<&-

# Restart real logind
echo "Restarting real logind..."
sudo systemctl start systemd-logind 2>/dev/null || true

echo "Done."
