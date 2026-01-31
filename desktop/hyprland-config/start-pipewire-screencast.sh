#!/bin/bash
# Start PipeWire ScreenCast session for Wolf pipewiresrc video source (Hyprland)
#
# This script creates a persistent ScreenCast session and reports the
# PipeWire node ID to Wolf so Wolf can start its pipewiresrc producer.
#
# For Hyprland, we use xdg-desktop-portal-hyprland which provides the
# standard org.freedesktop.portal.ScreenCast D-Bus interface.
#
# Required environment variables:
#   WOLF_SESSION_ID - The lobby ID to report the node ID to
#   XDG_RUNTIME_DIR - Where wolf.sock is located

set -e

WOLF_SOCKET="${XDG_RUNTIME_DIR:-/run/user/1000}/wolf.sock"
SESSION_HANDLE=""
STREAM_NODE_ID=""

log() {
    echo "[pipewire-screencast] $(date '+%H:%M:%S') $*" >&2
}

cleanup() {
    local exit_code=$?
    log "Cleanup triggered (exit code: $exit_code)"
    exit $exit_code
}

trap cleanup EXIT INT TERM

# Validate required environment variables
if [ -z "$WOLF_SESSION_ID" ]; then
    log "ERROR: WOLF_SESSION_ID not set (required for pipewiresrc mode)"
    exit 1
fi

log "Starting PipeWire ScreenCast for Hyprland lobby: $WOLF_SESSION_ID"
log "Wolf socket: $WOLF_SOCKET"

# Wait for xdg-desktop-portal and Hyprland portal backend
log "Waiting for xdg-desktop-portal-hyprland..."
sleep 2  # Give Hyprland time to start

# Start xdg-desktop-portal-hyprland in background if not running
if ! pgrep -x xdg-desktop-portal-hyprland > /dev/null; then
    log "Starting xdg-desktop-portal-hyprland..."
    /usr/libexec/xdg-desktop-portal-hyprland &
    sleep 1
fi

# Start xdg-desktop-portal if not running
if ! pgrep -x xdg-desktop-portal > /dev/null; then
    log "Starting xdg-desktop-portal..."
    /usr/libexec/xdg-desktop-portal &
    sleep 1
fi

# Wait for ScreenCast portal
log "Waiting for ScreenCast portal..."
if ! gdbus wait --session --timeout 30 org.freedesktop.portal.Desktop 2>/dev/null; then
    log "ERROR: xdg-desktop-portal not available after 30s"
    exit 1
fi
log "Portal service available"

# Create a unique token for this session
TOKEN="helix_$(date +%s)"

# Create ScreenCast session via portal
log "Creating ScreenCast session..."
SESSION_RESULT=$(gdbus call --session \
    --dest org.freedesktop.portal.Desktop \
    --object-path /org/freedesktop/portal/desktop \
    --method org.freedesktop.portal.ScreenCast.CreateSession \
    "{'handle_token': <'$TOKEN'>, 'session_handle_token': <'session_$TOKEN'>}")

# The portal returns a request handle, and we need to wait for the response
# For simplicity in a headless environment, we'll use a different approach:
# Hyprland's portal allows automatic screen selection for virtual displays

log "Session creation result: $SESSION_RESULT"

# Wait a bit for session to be created
sleep 0.5

# The session path follows a pattern
SESSION_PATH="/org/freedesktop/portal/desktop/session/${USER:-retro}/session_$TOKEN"

# Select sources (entire screen)
log "Selecting screen source..."
SELECT_RESULT=$(gdbus call --session \
    --dest org.freedesktop.portal.Desktop \
    --object-path /org/freedesktop/portal/desktop \
    --method org.freedesktop.portal.ScreenCast.SelectSources \
    "$SESSION_PATH" \
    "{'types': <uint32 1>, 'cursor_mode': <uint32 1>, 'persist_mode': <uint32 0>}" 2>/dev/null || true)

log "Source selection result: $SELECT_RESULT"
sleep 0.5

# Start the stream
log "Starting ScreenCast stream..."
START_RESULT=$(gdbus call --session \
    --dest org.freedesktop.portal.Desktop \
    --object-path /org/freedesktop/portal/desktop \
    --method org.freedesktop.portal.ScreenCast.Start \
    "$SESSION_PATH" \
    "" \
    "{}" 2>/dev/null || true)

log "Stream start result: $START_RESULT"

# Extract node_id from the result
# The Start method returns streams with node_id in the response
# Format: (uint32 0, @a{sv} {'streams': <[(uint32 NODE_ID, {...})]>})
NODE_ID=$(echo "$START_RESULT" | grep -oP 'uint32 \K\d+' | head -1)

if [ -z "$NODE_ID" ] || [ "$NODE_ID" = "0" ]; then
    # Try alternative: list PipeWire nodes to find the screen capture
    log "Trying to find node ID via pw-cli..."
    sleep 1

    # Look for a node that looks like a screen capture source
    NODE_ID=$(pw-cli list-objects 2>/dev/null | \
        grep -B5 "screen" | \
        grep "id:" | \
        head -1 | \
        grep -oP '\d+' || true)

    if [ -z "$NODE_ID" ]; then
        # Last resort: get the most recent video source node
        NODE_ID=$(pw-cli list-objects 2>/dev/null | \
            grep -B2 "media.class.*Video/Source" | \
            grep "id:" | \
            tail -1 | \
            grep -oP '\d+' || true)
    fi
fi

if [ -z "$NODE_ID" ]; then
    log "ERROR: Could not determine PipeWire node ID"
    log "ScreenCast may not be working correctly"
    # Don't exit - keep running and let Wolf figure it out
    NODE_ID=0
fi

log "PipeWire node ID: $NODE_ID"

# Give PipeWire a moment to set up the stream
sleep 0.5

# Report the node ID to Wolf via the API
log "Reporting node ID to Wolf..."
if [ -S "$WOLF_SOCKET" ]; then
    RESPONSE=$(curl -s --unix-socket "$WOLF_SOCKET" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "{\"lobby_id\": \"$WOLF_SESSION_ID\", \"node_id\": $NODE_ID}" \
        "http://localhost/api/v1/lobbies/set-pipewire-node-id")

    log "Wolf API response: $RESPONSE"

    if echo "$RESPONSE" | grep -q '"success":true'; then
        log "Successfully reported node ID to Wolf"
    else
        log "WARNING: Failed to report node ID to Wolf (response: $RESPONSE)"
    fi
else
    log "WARNING: Wolf socket not found at $WOLF_SOCKET"
fi

# Keep the session alive
log "Keeping ScreenCast session alive (PID: $$)..."
log "Press Ctrl+C or send SIGTERM to stop"

while true; do
    sleep 60
done
