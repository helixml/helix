#!/bin/bash
# Shared init script for settings-sync-daemon
# Used by both Ubuntu and Sway startup scripts
# Waits for D-Bus and Wayland, then starts settings-sync-daemon with log prefix

set -e

SERVICE_NAME="settings-sync"
DBUS_ENV_FILE="${XDG_RUNTIME_DIR:-/run/user/1000}/dbus-session.env"

log() {
    echo "[${SERVICE_NAME}] $*"
}

log "Starting..."

# Check required Helix environment variables
if [ -z "$HELIX_SESSION_ID" ] || [ -z "$HELIX_API_URL" ]; then
    log "WARNING: HELIX_SESSION_ID or HELIX_API_URL not set, settings sync disabled"
    log "HELIX_SESSION_ID=${HELIX_SESSION_ID:-NOT SET}"
    log "HELIX_API_URL=${HELIX_API_URL:-NOT SET}"
    exit 0
fi

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

# Wait for Wayland socket (settings-sync-daemon may need display access)
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

# Export environment for settings-sync-daemon
export WAYLAND_DISPLAY="$WAYLAND_SOCKET"
export DBUS_SESSION_BUS_ADDRESS

# Set XDG_CURRENT_DESKTOP if not already set
if [ -z "$XDG_CURRENT_DESKTOP" ]; then
    if [ "$WAYLAND_SOCKET" = "wayland-1" ]; then
        export XDG_CURRENT_DESKTOP="sway"
    else
        export XDG_CURRENT_DESKTOP="GNOME"
    fi
    log "Set XDG_CURRENT_DESKTOP=${XDG_CURRENT_DESKTOP}"
fi

log "Environment: HELIX_SESSION_ID=${HELIX_SESSION_ID}"
log "Environment: HELIX_API_URL=${HELIX_API_URL}"

# Start settings-sync-daemon with log prefix (stdout goes to docker logs)
log "Starting settings-sync-daemon"
# Use sed for log prefixing - exec replaces this script with the pipeline
exec /usr/local/bin/settings-sync-daemon 2>&1 | sed -u "s/^/[${SERVICE_NAME}] /"
