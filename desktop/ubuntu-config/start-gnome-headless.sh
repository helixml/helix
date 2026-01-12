#!/bin/bash
# Start GNOME Shell in headless mode with PipeWire bridge to Wolf
#
# This is an alternative to the XWayland approach. Instead of running
# GNOME as an X11 session on XWayland, we run GNOME in headless mode
# and bridge the screen-cast to Wolf via PipeWire.
#
# Architecture:
#   GNOME Shell (headless) → PipeWire → gnome-wolf-bridge → Wolf (Wayland)
#
# Advantages:
#   - Zero-copy GPU frames via DMA-BUF
#   - Pure Wayland, no X11 overhead
#   - Uses GNOME's existing screen-cast infrastructure
#
# Requirements:
#   - gnome-wolf-bridge binary installed
#   - GNOME Shell with headless support
#   - PipeWire running

set -e

echo "[gnome-headless] Starting GNOME headless with PipeWire bridge..."

# Get Wolf's Wayland display
WOLF_DISPLAY=${WAYLAND_DISPLAY:-wayland-1}
echo "[gnome-headless] Wolf display: $WOLF_DISPLAY"

# Start D-Bus session if not running
if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
    echo "[gnome-headless] Starting D-Bus session..."
    eval $(dbus-launch --sh-syntax)
    export DBUS_SESSION_BUS_ADDRESS
fi

# Start PipeWire
echo "[gnome-headless] Starting PipeWire..."
pipewire &
PIPEWIRE_PID=$!
sleep 1

# Start WirePlumber (PipeWire session manager)
echo "[gnome-headless] Starting WirePlumber..."
wireplumber &
WIREPLUMBER_PID=$!
sleep 1

# Start GNOME Shell in headless mode
echo "[gnome-headless] Starting GNOME Shell (headless)..."
gnome-shell --headless --wayland &
GNOME_SHELL_PID=$!

# Wait for GNOME to be ready
echo "[gnome-headless] Waiting for GNOME screen-cast service..."
for i in {1..30}; do
    if gdbus call --session \
        --dest org.gnome.Mutter.ScreenCast \
        --object-path /org/gnome/Mutter/ScreenCast \
        --method org.freedesktop.DBus.Peer.Ping >/dev/null 2>&1; then
        echo "[gnome-headless] GNOME screen-cast ready"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "[gnome-headless] Timeout waiting for GNOME"
        exit 1
    fi
    sleep 1
done

# Start the bridge
echo "[gnome-headless] Starting gnome-wolf-bridge..."
WAYLAND_DISPLAY="$WOLF_DISPLAY" \
    gnome-wolf-bridge \
    --width "${GAMESCOPE_WIDTH:-1920}" \
    --height "${GAMESCOPE_HEIGHT:-1080}" &
BRIDGE_PID=$!

# Cleanup function
cleanup() {
    echo "[gnome-headless] Shutting down..."
    kill $BRIDGE_PID 2>/dev/null || true
    kill $GNOME_SHELL_PID 2>/dev/null || true
    kill $WIREPLUMBER_PID 2>/dev/null || true
    kill $PIPEWIRE_PID 2>/dev/null || true
}
trap cleanup EXIT

echo "[gnome-headless] All components started"
echo "[gnome-headless] Bridge PID: $BRIDGE_PID"
echo "[gnome-headless] GNOME Shell PID: $GNOME_SHELL_PID"

# Wait for bridge to exit
wait $BRIDGE_PID
