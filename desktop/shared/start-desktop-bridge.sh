#!/bin/bash
# Shared init script for desktop-bridge
# Used by both Ubuntu and Sway startup scripts
# Only waits for Wayland socket, then starts desktop-bridge

set -e

SERVICE_NAME="desktop-bridge"

log() {
    echo "[${SERVICE_NAME}] $*"
}

log "Starting..."

# D-Bus session is inherited from dbus-run-session (both Sway and GNOME use this pattern)
if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
    log "WARNING: DBUS_SESSION_BUS_ADDRESS not set - should be launched from dbus-run-session"
fi

# Wait for Wayland socket (aggressive polling - 50ms intervals)
# Sway uses wayland-1, GNOME uses wayland-0
WAYLAND_SOCKET=""
for i in $(seq 1 600); do
    if [ -S "${XDG_RUNTIME_DIR}/wayland-1" ]; then
        WAYLAND_SOCKET="wayland-1"
        break
    elif [ -S "${XDG_RUNTIME_DIR}/wayland-0" ]; then
        WAYLAND_SOCKET="wayland-0"
        break
    fi
    sleep 0.05
done

if [ -z "$WAYLAND_SOCKET" ]; then
    log "ERROR: No Wayland socket found after 30 seconds"
    exit 1
fi
log "Wayland socket ready: ${WAYLAND_SOCKET}"

# Export environment for desktop-bridge
export WAYLAND_DISPLAY="$WAYLAND_SOCKET"

# Set XDG_CURRENT_DESKTOP if not already set
if [ -z "$XDG_CURRENT_DESKTOP" ]; then
    if [ "$WAYLAND_SOCKET" = "wayland-1" ]; then
        export XDG_CURRENT_DESKTOP="sway"
    else
        export XDG_CURRENT_DESKTOP="GNOME"
    fi
fi

# Start desktop-bridge with log prefix
log "Starting (WAYLAND_DISPLAY=${WAYLAND_DISPLAY}, DBUS=${DBUS_SESSION_BUS_ADDRESS:+set})"
exec /usr/local/bin/desktop-bridge 2>&1 | sed -u "s/^/[${SERVICE_NAME}] /"
