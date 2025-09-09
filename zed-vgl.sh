#!/bin/bash
# Zed launcher script with VirtualGL GPU acceleration

# Set up VirtualGL environment
export DISPLAY=:1
export VGL_DISPLAY=:0

# Debug info
echo "Starting Zed with VirtualGL GPU acceleration..."
echo "DISPLAY: $DISPLAY"
echo "VGL_DISPLAY: $VGL_DISPLAY"

# Check if mounted Zed binary exists
if [ ! -f "/zed-build/zed" ]; then
    echo "ERROR: Zed binary not found at /zed-build/zed"
    echo "Run './stack build-zed' on the host first"
    exit 1
fi

# Check VirtualGL status
echo "VirtualGL status:"
/opt/VirtualGL/bin/vglconnect -s 2>/dev/null || echo "VirtualGL connection check failed"

# Run Zed with VirtualGL acceleration
echo "Launching Zed with vglrun..."
exec /opt/VirtualGL/bin/vglrun /usr/local/bin/zed "$@"