#!/bin/bash
# Shared init script for desktop-bridge
# Used by both Ubuntu and Sway startup scripts
# Waits for D-Bus and Wayland, then starts desktop-bridge with log prefix

set -e

SERVICE_NAME="desktop-bridge"
DBUS_ENV_FILE="${XDG_RUNTIME_DIR:-/run/user/1000}/dbus-session.env"

log() {
    echo "[${SERVICE_NAME}] $*"
}

log "Starting..."

# Wait for D-Bus session env file
log "Waiting for D-Bus session env at ${DBUS_ENV_FILE}..."
for i in $(seq 1 30); do
    if [ -f "$DBUS_ENV_FILE" ]; then
        log "D-Bus session env found"
        # shellcheck source=/dev/null
        source "$DBUS_ENV_FILE"
        break
    fi
    sleep 1
done

if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
    log "ERROR: D-Bus session not available after 30 seconds"
    exit 1
fi
log "D-Bus session: ${DBUS_SESSION_BUS_ADDRESS}"

# Wait for Wayland socket
# Sway uses wayland-1, GNOME uses wayland-0
WAYLAND_SOCKET=""
log "Waiting for Wayland socket..."
for i in $(seq 1 60); do
    if [ -S "${XDG_RUNTIME_DIR}/wayland-1" ]; then
        WAYLAND_SOCKET="wayland-1"
        break
    elif [ -S "${XDG_RUNTIME_DIR}/wayland-0" ]; then
        WAYLAND_SOCKET="wayland-0"
        break
    fi
    sleep 0.5
done

if [ -z "$WAYLAND_SOCKET" ]; then
    log "ERROR: No Wayland socket found after 30 seconds"
    exit 1
fi
log "Wayland socket: ${WAYLAND_SOCKET}"

# Wait for XDG portal to be ready on D-Bus
# Sway starts xdg-desktop-portal-wlr asynchronously, so we need to wait for it
log "Waiting for XDG Desktop Portal (ScreenCast interface)..."
for i in $(seq 1 60); do
    if gdbus introspect --session --dest org.freedesktop.portal.Desktop --object-path /org/freedesktop/portal/desktop 2>/dev/null | grep -q "org.freedesktop.portal.ScreenCast"; then
        log "Portal ScreenCast interface ready"
        break
    fi
    sleep 0.5
done

# Verify portal is available
if ! gdbus introspect --session --dest org.freedesktop.portal.Desktop --object-path /org/freedesktop/portal/desktop 2>/dev/null | grep -q "org.freedesktop.portal.ScreenCast"; then
    log "WARNING: Portal ScreenCast interface not available - video streaming may fail"
fi

# Export environment for desktop-bridge
export WAYLAND_DISPLAY="$WAYLAND_SOCKET"
export DBUS_SESSION_BUS_ADDRESS

# Set XDG_CURRENT_DESKTOP if not already set (helps desktop-bridge detect compositor)
if [ -z "$XDG_CURRENT_DESKTOP" ]; then
    # Detect based on Wayland socket name
    if [ "$WAYLAND_SOCKET" = "wayland-1" ]; then
        export XDG_CURRENT_DESKTOP="sway"
    else
        export XDG_CURRENT_DESKTOP="GNOME"
    fi
    log "Set XDG_CURRENT_DESKTOP=${XDG_CURRENT_DESKTOP}"
fi

# Start desktop-bridge with log prefix (stdout goes to docker logs)
log "Starting desktop-bridge (WAYLAND_DISPLAY=${WAYLAND_DISPLAY})"
# Use sed for log prefixing - exec replaces this script with the pipeline
exec /usr/local/bin/desktop-bridge 2>&1 | sed -u "s/^/[${SERVICE_NAME}] /"
