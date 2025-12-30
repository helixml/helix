#!/bin/bash
# Start RemoteDesktop session for Wolf pipewiresrc video + EIS input
#
# This script creates a persistent RemoteDesktop session that provides:
# 1. ScreenCast for video (PipeWire stream)
# 2. Input injection via D-Bus Notify* methods
#
# Unlike pure ScreenCast, RemoteDesktop allows input injection.
#
# Usage: start-remotedesktop-session.sh
#
# Required environment variables:
#   WOLF_SESSION_ID - The lobby ID to report the node ID to
#   XDG_RUNTIME_DIR - Where sockets are located
#
# The script:
# 1. Waits for GNOME Mutter RemoteDesktop D-Bus service
# 2. Creates a RemoteDesktop session
# 3. Associates a ScreenCast stream (virtual monitor)
# 4. Gets the PipeWire node ID
# 5. Starts the session
# 6. Reports the node ID and session path to Wolf via lobby API
# 7. Starts the input bridge to receive input from Wolf
# 8. Keeps running to maintain the session

set -e

WOLF_SOCKET="${WOLF_LOBBY_SOCKET_PATH:-/var/run/wolf/lobby.sock}"
RD_SESSION_PATH=""
SC_STREAM_PATH=""
INPUT_SOCKET="${XDG_RUNTIME_DIR}/wolf-input.sock"

log() {
    echo "[remotedesktop] $(date '+%H:%M:%S') $*" >&2
}

cleanup() {
    local exit_code=$?

    # Stop the RemoteDesktop session if we created one
    if [ -n "$RD_SESSION_PATH" ]; then
        log "Stopping RemoteDesktop session..."
        gdbus call --session \
            --dest org.gnome.Mutter.RemoteDesktop \
            --object-path "$RD_SESSION_PATH" \
            --method org.gnome.Mutter.RemoteDesktop.Session.Stop 2>/dev/null || true
    fi

    # Remove input socket
    rm -f "$INPUT_SOCKET"

    exit $exit_code
}

trap cleanup EXIT INT TERM

# Validate required environment variables
if [ -z "$WOLF_SESSION_ID" ]; then
    log "ERROR: WOLF_SESSION_ID not set (required for pipewiresrc mode)"
    exit 1
fi

log "Starting RemoteDesktop session for lobby: $WOLF_SESSION_ID"
log "Wolf lobby socket: $WOLF_SOCKET"

# Wait for RemoteDesktop D-Bus service (may take a few seconds after GNOME starts)
log "Waiting for GNOME Mutter RemoteDesktop D-Bus service..."
if ! gdbus wait --session --timeout 30 org.gnome.Mutter.RemoteDesktop 2>/dev/null; then
    log "ERROR: GNOME Mutter RemoteDesktop not available after 30s"
    exit 1
fi
log "RemoteDesktop service available"

# Also wait for ScreenCast service
log "Waiting for GNOME Mutter ScreenCast D-Bus service..."
if ! gdbus wait --session --timeout 30 org.gnome.Mutter.ScreenCast 2>/dev/null; then
    log "ERROR: GNOME Mutter ScreenCast not available after 30s"
    exit 1
fi
log "ScreenCast service available"

# Create a RemoteDesktop session
# This is different from ScreenCast - it allows input injection
log "Creating RemoteDesktop session..."
RD_SESSION_RESULT=$(gdbus call --session \
    --dest org.gnome.Mutter.RemoteDesktop \
    --object-path /org/gnome/Mutter/RemoteDesktop \
    --method org.gnome.Mutter.RemoteDesktop.CreateSession)

# Extract session path from result like: (objectpath '/org/gnome/Mutter/RemoteDesktop/Session/u1',)
RD_SESSION_PATH=$(echo "$RD_SESSION_RESULT" | grep -oP "'/[^']+'")
RD_SESSION_PATH="${RD_SESSION_PATH//\'/}"

if [ -z "$RD_SESSION_PATH" ]; then
    log "ERROR: Failed to create RemoteDesktop session"
    log "Result was: $RD_SESSION_RESULT"
    exit 1
fi
log "RemoteDesktop session created: $RD_SESSION_PATH"

# Get the session ID (last component of the path)
RD_SESSION_ID=$(echo "$RD_SESSION_PATH" | sed 's|.*/||')
log "Session ID: $RD_SESSION_ID"

# Now create a ScreenCast session LINKED to the RemoteDesktop session
# This uses the same session ID so they're associated
log "Creating linked ScreenCast session..."
SC_SESSION_RESULT=$(gdbus call --session \
    --dest org.gnome.Mutter.ScreenCast \
    --object-path /org/gnome/Mutter/ScreenCast \
    --method org.gnome.Mutter.ScreenCast.CreateSession \
    "{'remote-desktop-session-id': <'$RD_SESSION_ID'>}")

# Extract ScreenCast session path
SC_SESSION_PATH=$(echo "$SC_SESSION_RESULT" | grep -oP "'/[^']+'")
SC_SESSION_PATH="${SC_SESSION_PATH//\'/}"

if [ -z "$SC_SESSION_PATH" ]; then
    log "ERROR: Failed to create linked ScreenCast session"
    log "Result was: $SC_SESSION_RESULT"
    exit 1
fi
log "ScreenCast session created: $SC_SESSION_PATH"

# Record the virtual display (for headless/devkit mode)
# cursor-mode: 0=hidden, 1=embedded (show cursor in capture), 2=metadata
log "Recording virtual display..."
STREAM_RESULT=$(gdbus call --session \
    --dest org.gnome.Mutter.ScreenCast \
    --object-path "$SC_SESSION_PATH" \
    --method org.gnome.Mutter.ScreenCast.Session.RecordVirtual \
    "{'cursor-mode': <uint32 1>}" 2>/dev/null || \
    # Fallback: try recording the first monitor
    gdbus call --session \
        --dest org.gnome.Mutter.ScreenCast \
        --object-path "$SC_SESSION_PATH" \
        --method org.gnome.Mutter.ScreenCast.Session.RecordMonitor \
        "" \
        "{'cursor-mode': <uint32 1>}")

# Extract stream path
SC_STREAM_PATH=$(echo "$STREAM_RESULT" | grep -oP "'/[^']+'")
SC_STREAM_PATH="${SC_STREAM_PATH//\'/}"

if [ -z "$SC_STREAM_PATH" ]; then
    log "ERROR: Failed to record display"
    log "Result was: $STREAM_RESULT"
    exit 1
fi
log "Stream created: $SC_STREAM_PATH"

# Get PipeWire node ID
log "Getting PipeWire node ID..."
NODE_RESULT=$(gdbus call --session \
    --dest org.gnome.Mutter.ScreenCast \
    --object-path "$SC_STREAM_PATH" \
    --method org.freedesktop.DBus.Properties.Get \
    org.gnome.Mutter.ScreenCast.Stream PipeWireNodeId)

NODE_ID=$(echo "$NODE_RESULT" | grep -oP 'uint32 \K\d+')

if [ -z "$NODE_ID" ]; then
    log "ERROR: Failed to get PipeWire node ID"
    log "Result was: $NODE_RESULT"
    exit 1
fi
log "PipeWire node ID: $NODE_ID"

# Start the RemoteDesktop session (this also starts the ScreenCast)
log "Starting RemoteDesktop session..."
gdbus call --session \
    --dest org.gnome.Mutter.RemoteDesktop \
    --object-path "$RD_SESSION_PATH" \
    --method org.gnome.Mutter.RemoteDesktop.Session.Start

log "RemoteDesktop session started successfully"

# Give PipeWire a moment to set up the stream
sleep 0.5

# Report the node ID and session path to Wolf via the lobby API
log "Reporting to Wolf..."
if [ -S "$WOLF_SOCKET" ]; then
    RESPONSE=$(curl -s --unix-socket "$WOLF_SOCKET" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "{\"node_id\": $NODE_ID, \"session_path\": \"$RD_SESSION_PATH\"}" \
        "http://localhost/set-pipewire-node-id")

    log "Wolf API response: $RESPONSE"

    if echo "$RESPONSE" | grep -q '"success":true'; then
        log "Successfully reported node ID to Wolf"
    else
        log "WARNING: Failed to report node ID to Wolf (response: $RESPONSE)"
        # Continue anyway - Wolf might not be ready yet
    fi
else
    log "WARNING: Wolf lobby socket not found at $WOLF_SOCKET"
    log "Wolf may not be able to receive the node ID"
fi

# Start the Python input bridge
# This listens on a Unix socket for input commands from Wolf
# and forwards them to GNOME via the RemoteDesktop D-Bus API
# Using Python with GLib D-Bus bindings for better performance
log "Starting input bridge on $INPUT_SOCKET..."

# Remove old socket if exists
rm -f "$INPUT_SOCKET"

# Start the Python input bridge
/opt/gow/input-bridge.py "$RD_SESSION_PATH" "$SC_STREAM_PATH" "$INPUT_SOCKET" &
INPUT_BRIDGE_PID=$!

# Also report the input socket path to Wolf
if [ -S "$WOLF_SOCKET" ]; then
    RESPONSE=$(curl -s --unix-socket "$WOLF_SOCKET" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "{\"input_socket\": \"$INPUT_SOCKET\"}" \
        "http://localhost/set-input-socket")
    log "Reported input socket to Wolf: $RESPONSE"
fi

# Keep the session alive
log "Keeping RemoteDesktop session alive (PID: $$)..."
log "Input bridge PID: $INPUT_BRIDGE_PID"
log "Press Ctrl+C or send SIGTERM to stop"

# Sleep forever (or until killed)
while true; do
    sleep 60
    # Verify the input bridge is still running
    if ! kill -0 $INPUT_BRIDGE_PID 2>/dev/null; then
        log "WARNING: Input bridge died, restarting..."
        rm -f "$INPUT_SOCKET"
        /opt/gow/input-bridge.py "$RD_SESSION_PATH" "$SC_STREAM_PATH" "$INPUT_SOCKET" &
        INPUT_BRIDGE_PID=$!
        log "Input bridge restarted (PID: $INPUT_BRIDGE_PID)"
    fi
done
