#!/bin/bash
# GOW base-app startup script for Helix Personal Dev Environment
set -e

echo "Starting Helix Personal Dev Environment with Sway..."

# Wait a moment for the system to stabilize
sleep 2

# Start Sway window manager
echo "Starting Sway window manager..."
exec sway