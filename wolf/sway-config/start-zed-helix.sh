#!/bin/bash
# Startup script for Zed editor connected to Helix controlplane (Sway version)
set -e

# Check if Zed binary exists (directory mounted to survive inode changes on rebuild)
if [ ! -f "/zed-build/zed" ]; then
    echo "Zed binary not found at /zed-build/zed - cannot start Zed agent"
    exit 1
fi

# Environment variables are passed from Wolf executor via container env
# HELIX_API_URL, HELIX_API_TOKEN, ANTHROPIC_API_KEY should be available

# Set workspace to mounted work directory
cd /home/retro
sudo chown retro:retro work
cd /home/retro/work

# Trap signals to prevent script exit when Zed is closed
# Using signal numbers for compatibility: 15=TERM, 2=INT, 1=HUP
trap 'echo "Caught signal, continuing restart loop..."' 15 2 1

# Launch Zed in a restart loop for development
# When you close Zed (click X), it auto-restarts with the latest binary
# Perfect for testing rebuilds without recreating the entire container
echo "Starting Zed with auto-restart loop (close window to reload updated binary)"
while true; do
    echo "Launching Zed..."
    /zed-build/zed . || true
    echo "Zed exited, restarting in 2 seconds..."
    sleep 2
done
