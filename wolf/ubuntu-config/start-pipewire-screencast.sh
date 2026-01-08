#!/bin/bash
# Start PipeWire ScreenCast session for Wolf pipewiresrc video source
#
# This script creates a persistent ScreenCast session and reports the
# PipeWire node ID to Wolf so Wolf can start its pipewiresrc producer.
#
# Usage: start-pipewire-screencast.sh
#
# Required environment variables:
#   WOLF_SESSION_ID - The lobby ID to report the node ID to
#   XDG_RUNTIME_DIR - Where wolf.sock is located
#
# The script:
# 1. Waits for GNOME Mutter ScreenCast D-Bus service
# 2. Creates a ScreenCast session
# 3. Records the virtual monitor
# 4. Gets the PipeWire node ID
# 5. Starts the session
# 6. Reports the node ID to Wolf via API
# 7. Keeps running to maintain the session

set -e

WOLF_SOCKET="${XDG_RUNTIME_DIR:-/run/user/1000}/wolf.sock"
SESSION_PATH=""
STREAM_PATH=""

log() {
    echo "[pipewire-screencast] $(date '+%H:%M:%S') $*" >&2
}

cleanup() {
    local exit_code=$?

    # Stop the ScreenCast session if we created one
    if [ -n "$SESSION_PATH" ]; then
        log "Stopping ScreenCast session..."
        gdbus call --session \
            --dest org.gnome.Mutter.ScreenCast \
            --object-path "$SESSION_PATH" \
            --method org.gnome.Mutter.ScreenCast.Session.Stop 2>/dev/null || true
    fi

    exit $exit_code
}

trap cleanup EXIT INT TERM

# Validate required environment variables
if [ -z "$WOLF_SESSION_ID" ]; then
    log "ERROR: WOLF_SESSION_ID not set (required for pipewiresrc mode)"
    exit 1
fi

log "Starting PipeWire ScreenCast for lobby: $WOLF_SESSION_ID"
log "Wolf socket: $WOLF_SOCKET"

# Wait for ScreenCast D-Bus service (may take a few seconds after GNOME starts)
log "Waiting for GNOME Mutter ScreenCast D-Bus service..."
if ! gdbus wait --session --timeout 30 org.gnome.Mutter.ScreenCast 2>/dev/null; then
    log "ERROR: GNOME Mutter ScreenCast not available after 30s"
    exit 1
fi
log "ScreenCast service available"

# Create a ScreenCast session
log "Creating ScreenCast session..."
SESSION_RESULT=$(gdbus call --session \
    --dest org.gnome.Mutter.ScreenCast \
    --object-path /org/gnome/Mutter/ScreenCast \
    --method org.gnome.Mutter.ScreenCast.CreateSession \
    "{}")

# Extract session path from result like: (objectpath '/org/gnome/Mutter/ScreenCast/Session/u1',)
SESSION_PATH=$(echo "$SESSION_RESULT" | grep -oP "'/[^']+'")
SESSION_PATH="${SESSION_PATH//\'/}"

if [ -z "$SESSION_PATH" ]; then
    log "ERROR: Failed to create ScreenCast session"
    log "Result was: $SESSION_RESULT"
    exit 1
fi
log "Session created: $SESSION_PATH"

# Record the virtual display (for headless/devkit mode) or primary monitor
# cursor-mode: 0=hidden, 1=embedded (show cursor in capture), 2=metadata
# is-platform: true = treat as real monitor (cache-bust-1767900711) (may improve framerate), available since API v3
log "Recording virtual display..."
STREAM_RESULT=$(gdbus call --session \
    --dest org.gnome.Mutter.ScreenCast \
    --object-path "$SESSION_PATH" \
    --method org.gnome.Mutter.ScreenCast.Session.RecordVirtual \
    "{'cursor-mode': <uint32 1>, 'is-platform': <boolean true>}" 2>/dev/null || \
    # Fallback: try recording XWAYLAND0
    gdbus call --session \
        --dest org.gnome.Mutter.ScreenCast \
        --object-path "$SESSION_PATH" \
        --method org.gnome.Mutter.ScreenCast.Session.RecordMonitor \
        "XWAYLAND0" \
        "{'cursor-mode': <uint32 1>, 'is-platform': <boolean true>}")

# Extract stream path
STREAM_PATH=$(echo "$STREAM_RESULT" | grep -oP "'/[^']+'")
STREAM_PATH="${STREAM_PATH//\'/}"

if [ -z "$STREAM_PATH" ]; then
    log "ERROR: Failed to record display"
    log "Result was: $STREAM_RESULT"
    exit 1
fi
log "Stream created: $STREAM_PATH"

# Get PipeWire node ID
log "Getting PipeWire node ID..."
NODE_RESULT=$(gdbus call --session \
    --dest org.gnome.Mutter.ScreenCast \
    --object-path "$STREAM_PATH" \
    --method org.freedesktop.DBus.Properties.Get \
    org.gnome.Mutter.ScreenCast.Stream PipeWireNodeId)

NODE_ID=$(echo "$NODE_RESULT" | grep -oP 'uint32 \K\d+')

if [ -z "$NODE_ID" ]; then
    log "ERROR: Failed to get PipeWire node ID"
    log "Result was: $NODE_RESULT"
    exit 1
fi
log "PipeWire node ID: $NODE_ID"

# Start the ScreenCast session
log "Starting ScreenCast session..."
gdbus call --session \
    --dest org.gnome.Mutter.ScreenCast \
    --object-path "$SESSION_PATH" \
    --method org.gnome.Mutter.ScreenCast.Session.Start

log "ScreenCast session started successfully"

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
        # Continue anyway - Wolf might not be ready yet
    fi
else
    log "WARNING: Wolf socket not found at $WOLF_SOCKET"
    log "Wolf may not be able to receive the node ID"
fi

# Keep the session alive
# The session must stay running for Wolf's pipewiresrc to read from it
log "Keeping ScreenCast session alive (PID: $$)..."
log "Press Ctrl+C or send SIGTERM to stop"

# Sleep forever (or until killed)
while true; do
    sleep 60
    # Optionally could add health checks here
done
