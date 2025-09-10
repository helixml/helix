#!/bin/bash
set -e

# Desktop startup script for Ubuntu user
echo "Starting desktop environment as Ubuntu user..."

# Weston configuration for VNC backend with 4K support
mkdir -p /home/ubuntu/.config
cat > /home/ubuntu/.config/weston.ini << 'WESTONCONF'
[core]
backend=vnc-backend.so
renderer=gl

[vnc]
refresh-rate=60

[output]
name=vnc
mode=3840x2160
resizeable=false

[libinput]
enable-tap=true
tap-and-drag=true
disable-while-typing=false

[keyboard]
keymap_rules=evdev
keymap_model=pc105
keymap_layout=us

[shell]
background-color=0xff1b1b26
panel-color=0xff241c2e
locking=false
binding-modifier=ctrl

[launcher]
icon=/usr/share/icons/hicolor/32x32/apps/com.mitchellh.ghostty.png
path=/usr/bin/ghostty
displayname=Ghostty

[launcher]
icon=/usr/share/icons/hicolor/32x32/apps/google-chrome.png
path=/usr/bin/google-chrome --enable-features=UseOzonePlatform --ozone-platform=wayland --no-sandbox --disable-dev-shm-usage
displayname=Chrome

WESTONCONF

# Start dbus session
if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
    eval $(dbus-launch --sh-syntax)
    export DBUS_SESSION_BUS_ADDRESS
fi

# Set up runtime directory
export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu
mkdir -p $XDG_RUNTIME_DIR
chmod 700 $XDG_RUNTIME_DIR

# Set up GPU environment
export DISPLAY=:1
export VGL_DISPLAY=:0
export NVIDIA_VISIBLE_DEVICES=all
export NVIDIA_DRIVER_CAPABILITIES=all
export GBM_BACKEND=nvidia-drm
export __GLX_VENDOR_LIBRARY_NAME=nvidia

# Start Weston with VNC backend for 4K@60Hz using TLS
weston --backend=vnc --address=0.0.0.0 --port=5900 --width=3840 --height=2160 --disable-transport-layer-security &
COMPOSITOR_PID=$!

# Wait for Weston to start
sleep 8

echo "Weston started, launching applications..."

# Start input reset daemon to handle reconnection issues
cat > /tmp/input-reset.sh << 'INPUT_RESET'
#!/bin/bash
while true; do
    sleep 30
    # Check if weston is running and reset input focus
    if pgrep weston > /dev/null; then
        # Send a key event to refresh input state
        export WAYLAND_DISPLAY=wayland-1
        # Use weston-simple-touch to refresh input if available
        timeout 1s weston-info 2>/dev/null >/dev/null || true
    fi
done
INPUT_RESET

chmod +x /tmp/input-reset.sh
/tmp/input-reset.sh &

# Create manual input reset script for emergency use
cat > /tmp/fix-input.sh << 'FIX_INPUT'
#!/bin/bash
echo "Attempting to fix input issues..."
export WAYLAND_DISPLAY=wayland-1

# Kill and restart weston-keyboard if it exists
pkill weston-keyboard 2>/dev/null || true
sleep 1
/usr/libexec/weston-keyboard &

# Reset input devices via weston control
echo "Input reset attempted. Try typing now."
FIX_INPUT

chmod +x /tmp/fix-input.sh

# Start Helix agent runner in background
echo "Starting Helix External Agent Runner..."
export API_HOST=http://api:8080
export API_TOKEN=${RUNNER_TOKEN:-oh-hallo-insecure-token}
export LOG_LEVEL=debug
export CONCURRENCY=1
export MAX_TASKS=0
export SESSION_TIMEOUT=3600
export WORKSPACE_DIR=/tmp/workspace

# Start the Helix agent runner
/usr/local/bin/helix external-agent-runner &
AGENT_PID=$!

echo "Helix agent runner started with PID $AGENT_PID"
echo "VNC server started on port 5900"

# Wait for compositor (this keeps the container running)
wait $COMPOSITOR_PID
