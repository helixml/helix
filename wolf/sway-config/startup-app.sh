#!/bin/bash
# GOW base-app startup script for Helix Personal Dev Environment
set -e

echo "Starting Helix Personal Dev Environment with Sway..."

# Wait a moment for the system to stabilize
sleep 2

# Source GOW's launch-comp.sh and launch Zed with the launcher function
echo "Starting Sway and launching Zed via GOW launcher..."
source /opt/gow/launch-comp.sh
launcher /usr/local/bin/zed .