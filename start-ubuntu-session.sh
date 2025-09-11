#!/bin/bash
set -e

export USER=ubuntu
export HOME=/home/ubuntu
export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu
mkdir -p /tmp/runtime-ubuntu
chmod 700 /tmp/runtime-ubuntu

# Export GPU environment variables
export WLR_RENDERER="$WLR_RENDERER"
export WLR_BACKENDS="$WLR_BACKENDS"
export WLR_NO_HARDWARE_CURSORS="$WLR_NO_HARDWARE_CURSORS"
export WLR_HEADLESS_OUTPUTS="$WLR_HEADLESS_OUTPUTS"
export __GL_THREADED_OPTIMIZATIONS="$__GL_THREADED_OPTIMIZATIONS"
export __GL_SYNC_TO_VBLANK="$__GL_SYNC_TO_VBLANK"
export LIBGL_ALWAYS_SOFTWARE="$LIBGL_ALWAYS_SOFTWARE"
export GBM_BACKEND="$GBM_BACKEND"
export __GLX_VENDOR_LIBRARY_NAME="$__GLX_VENDOR_LIBRARY_NAME"
export WLR_DRM_NO_ATOMIC="$WLR_DRM_NO_ATOMIC"
export NVIDIA_VISIBLE_DEVICES="$NVIDIA_VISIBLE_DEVICES"
export NVIDIA_DRIVER_CAPABILITIES="$NVIDIA_DRIVER_CAPABILITIES"
export EGL_PLATFORM="$EGL_PLATFORM"
export WLR_DRM_DEVICES="$WLR_DRM_DEVICES"
export WLR_RENDERER_ALLOW_SOFTWARE="$WLR_RENDERER_ALLOW_SOFTWARE"
export LIBGL_DRIVERS_PATH="$LIBGL_DRIVERS_PATH"
export LIBVA_DRIVERS_PATH="$LIBVA_DRIVERS_PATH"
export GPU_RENDER_NODE="$GPU_RENDER_NODE"
export GST_GL_DRM_DEVICE="$GST_GL_DRM_DEVICE"

# Start dbus session
if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
    eval $(dbus-launch --sh-syntax)
fi

# Create crash report directory for Hyprland
mkdir -p /home/ubuntu/.cache/hyprland
chown ubuntu:ubuntu /home/ubuntu/.cache/hyprland

# Enhanced Hyprland startup with NVIDIA GPU acceleration
echo "Starting Hyprland with NVIDIA RTX 4090 GPU acceleration..."

# Set up comprehensive environment for Hyprland NVIDIA support
export HYPRLAND_LOG_WLR=1
export HYPRLAND_NO_RT=1
export WLR_RENDERER_ALLOW_SOFTWARE=1

# Pre-flight GPU check
echo "Pre-flight GPU check:"
echo "DRI devices: $(ls /dev/dri/)"
echo "NVIDIA_VISIBLE_DEVICES: $NVIDIA_VISIBLE_DEVICES"
echo "GBM_BACKEND: $GBM_BACKEND"

# Start Hyprland with comprehensive error capture and debug config
echo "Starting Hyprland with debug logging..."
export PATH="/usr/bin:/usr/local/bin:$PATH"

echo "Hyprland command: /usr/bin/Hyprland"
/usr/bin/Hyprland &
COMPOSITOR_PID=$!
echo "Hyprland started with PID: $COMPOSITOR_PID"

# Give Hyprland time to initialize with GPU
sleep 5

# Check if Hyprland is running (either our PID or any Hyprland process)
if pgrep -x "Hyprland" >/dev/null 2>&1; then
    echo "✅ Hyprland is running successfully with NVIDIA GPU acceleration!"
else
    echo "❌ Hyprland failed to start with NVIDIA GPU acceleration"
    exit 1
fi

# Start wayvnc VNC server on the Wayland display
echo "Starting wayvnc VNC server..."
# Wait for Wayland display to be ready
sleep 1

# Find the actual Wayland display socket
if [ -S "$XDG_RUNTIME_DIR/wayland-1" ]; then
    export WAYLAND_DISPLAY=wayland-1
elif [ -S "$XDG_RUNTIME_DIR/wayland-0" ]; then
    export WAYLAND_DISPLAY=wayland-0
fi

echo "Using Wayland display: $WAYLAND_DISPLAY"

# Disable Wayland security features that would require user permission prompts in containerized environment
export SUNSHINE_DISABLE_WAYLAND_SECURITY=1
export WLR_ALLOW_ALL_CLIENTS=1

# Start wayvnc with input enabled and cursor optimizations for VNC
echo "Starting wayvnc on port 5902..."
wayvnc --max-fps 120 --show-performance --disable-resizing 0.0.0.0 5902 &
WAYVNC_PID=$!

# Wait a moment and verify wayvnc is running on the correct port
sleep 2
if lsof -i :5902 >/dev/null 2>&1; then
    echo "✅ VNC server started successfully on port 5902"
else
    echo "❌ VNC server failed to start on port 5902"
    echo "Exiting container due to VNC startup failure"
    exit 1
fi

# Start Sunshine Moonlight server
echo "Starting Sunshine Moonlight server..."
/start-moonlight.sh &
MOONLIGHT_PID=$!

echo "Sunshine Moonlight server started on port 47989 (HTTP/HTTPS), will use standard Moonlight protocol ports"

# Set up environment for AGS
echo "Setting up AGS environment..."
export DISPLAY=:1
export HYPRLAND_INSTANCE_SIGNATURE=$(ls $XDG_RUNTIME_DIR/hypr/ | tail -1)

# Create additional workspaces for better desktop experience
echo "Creating additional workspaces..."
for i in 2 3 4 5; do
    hyprctl dispatch workspace $i 2>/dev/null || true
done
hyprctl dispatch workspace 1 2>/dev/null || true

# Wait a bit more for Hyprland to fully initialize
sleep 3

# Start AGS with proper environment and unique bus name
echo "Pre-compiling AGS SCSS styles..."
mkdir -p /home/ubuntu/.cache/ags/user/generated
cd /home/ubuntu/.config/ags

if command -v sass >/dev/null 2>&1; then
    sass scss/main.scss /home/ubuntu/.cache/ags/user/generated/style.css 2>&1 || echo "SCSS compilation failed, AGS will try at runtime"
    echo "SCSS compilation completed"
else
    echo "Sass not available, AGS will compile at runtime"
fi

# Kill any existing AGS processes to prevent duplicates during restarts/debugging
echo "Cleaning up any existing AGS processes..."
# pkill -f agsv1 2>/dev/null || true

echo "Starting AGS bar and dock..."
(agsv1 --bus-name "ags-$(date +%s)" 2>&1 | while read line; do echo "AGS: $line"; done) &
AGS_PID=$!

# Wait for all processes
wait $COMPOSITOR_PID $WAYVNC_PID $MOONLIGHT_PID