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

# D-Bus session should already be set by dbus-run-session wrapper
# If not set, try to source from file as fallback
if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
    DBUS_ENV_FILE="${XDG_RUNTIME_DIR:-/run/user/1000}/dbus-session.env"
    if [ -f "$DBUS_ENV_FILE" ]; then
        # shellcheck source=/dev/null
        source "$DBUS_ENV_FILE"
        log "D-Bus session sourced from file"
    else
        log "WARNING: DBUS_SESSION_BUS_ADDRESS not set and no env file found"
    fi
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
