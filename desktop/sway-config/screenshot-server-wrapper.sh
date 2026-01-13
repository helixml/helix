#!/bin/bash

# Screenshot server wrapper that ensures correct Wayland environment
# Replaces /usr/local/bin/desktop-bridge with env-aware version

# Ensure we have correct Wayland display
export WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-wayland-1}
export XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-/tmp/sockets}

echo "Screenshot server wrapper starting with WAYLAND_DISPLAY=$WAYLAND_DISPLAY" >&2

# Start simple HTTP server on port 9876
PORT=9876

# Use nc (netcat) and a loop to serve screenshots
while true; do
    {
        # Wait for connection
        echo "HTTP/1.1 200 OK"
        echo "Content-Type: image/png"
        echo ""

        # Capture screenshot with grim
        grim - 2>/tmp/grim-error.log || {
            echo "HTTP/1.1 500 Internal Server Error"
            echo "Content-Type: text/plain"
            echo ""
            echo "Failed to capture screenshot"
            cat /tmp/grim-error.log >&2
        }
    } | nc -l -p $PORT -q 1
done
