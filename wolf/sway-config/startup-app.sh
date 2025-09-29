#!/bin/bash
# GOW base-app startup script for Helix Personal Dev Environment
set -e

echo "Starting Helix Personal Dev Environment with Sway..."

# Wait a moment for the system to stabilize
sleep 2

# Function to start wayvnc VNC server after Sway is ready
start_wayvnc() {
    echo "Starting wayvnc VNC server..."
    # Wait for Sway/Wayland to be ready
    sleep 5

    # Find the Wayland display socket
    if [ -S "$XDG_RUNTIME_DIR/wayland-1" ]; then
        export WAYLAND_DISPLAY=wayland-1
    elif [ -S "$XDG_RUNTIME_DIR/wayland-0" ]; then
        export WAYLAND_DISPLAY=wayland-0
    fi

    echo "Using Wayland display: $WAYLAND_DISPLAY"

    # Disable Wayland security features for containerized environment
    export WLR_ALLOW_ALL_CLIENTS=1

    # Start wayvnc on port 5901
    echo "Starting wayvnc on port 5901..."
    wayvnc --max-fps 120 --show-performance --disable-resizing 0.0.0.0 5901 &
    WAYVNC_PID=$!

    # Verify wayvnc started successfully
    sleep 2
    if lsof -i :5901 >/dev/null 2>&1; then
        echo "✅ VNC server started successfully on port 5901"
    else
        echo "❌ VNC server failed to start on port 5901"
    fi
}

# Start wayvnc in background
start_wayvnc &

# Source GOW's launch-comp.sh and launch Zed with the launcher function
echo "Starting Sway and launching Zed via GOW launcher..."
source /opt/gow/launch-comp.sh
launcher /usr/local/bin/zed .