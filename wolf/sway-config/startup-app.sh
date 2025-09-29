#!/bin/bash
# GOW base-app startup script for Helix Personal Dev Environment
set -e

echo "Starting Helix Personal Dev Environment with Sway..."

# Wait a moment for the system to stabilize
sleep 2

# Use GOW's launch-comp.sh which handles NVIDIA compatibility
echo "Starting Sway window manager via GOW launch-comp.sh..."
exec /opt/gow/launch-comp.sh