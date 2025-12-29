#!/bin/bash
# GNOME Headless → Wolf Bridge via GStreamer + PipeWire
#
# Uses existing GStreamer elements:
# - pipewiresrc: Consumes PipeWire screen-cast stream (DMA-BUF)
# - waylandsink: Outputs to Wolf's Wayland compositor
#
# This replaces the need for a custom bridge binary.

set -e

# Configuration
WOLF_DISPLAY="${WOLF_WAYLAND_DISPLAY:-wayland-1}"
WIDTH="${GAMESCOPE_WIDTH:-1920}"
HEIGHT="${GAMESCOPE_HEIGHT:-1080}"

log() {
    echo "[gnome-bridge] $(date '+%H:%M:%S') $*"
}

# Wait for D-Bus session
wait_for_dbus() {
    log "Waiting for D-Bus session bus..."
    for i in $(seq 1 30); do
        if dbus-send --session --print-reply --dest=org.freedesktop.DBus / org.freedesktop.DBus.ListNames >/dev/null 2>&1; then
            log "D-Bus session bus ready"
            return 0
        fi
        sleep 0.5
    done
    log "ERROR: D-Bus session bus not available"
    return 1
}

# Wait for PipeWire
wait_for_pipewire() {
    log "Waiting for PipeWire..."
    for i in $(seq 1 30); do
        if pw-cli info >/dev/null 2>&1; then
            log "PipeWire ready"
            return 0
        fi
        sleep 0.5
    done
    log "ERROR: PipeWire not available"
    return 1
}

# Wait for GNOME Mutter ScreenCast
wait_for_screencast() {
    log "Waiting for GNOME Mutter ScreenCast..."
    if gdbus wait --session --timeout 30 org.gnome.Mutter.ScreenCast; then
        log "GNOME Mutter ScreenCast ready"
        return 0
    fi
    log "ERROR: GNOME Mutter ScreenCast not available"
    return 1
}

# Create screen-cast session and get PipeWire node ID
create_screencast_session() {
    log "Creating GNOME screen-cast session..."

    # Create session
    SESSION_PATH=$(gdbus call --session \
        --dest org.gnome.Mutter.ScreenCast \
        --object-path /org/gnome/Mutter/ScreenCast \
        --method org.gnome.Mutter.ScreenCast.CreateSession \
        "{}" | grep -oP "'/[^']+'" | tr -d "'")

    if [ -z "$SESSION_PATH" ]; then
        log "ERROR: Failed to create session"
        return 1
    fi
    log "Session: $SESSION_PATH"

    # Record virtual display (headless mode - no user interaction!)
    STREAM_PATH=$(gdbus call --session \
        --dest org.gnome.Mutter.ScreenCast \
        --object-path "$SESSION_PATH" \
        --method org.gnome.Mutter.ScreenCast.Session.RecordVirtual \
        "{'cursor-mode': <uint32 1>}" | grep -oP "'/[^']+'" | tr -d "'")

    if [ -z "$STREAM_PATH" ]; then
        log "ERROR: Failed to record virtual display"
        return 1
    fi
    log "Stream: $STREAM_PATH"

    # Get PipeWire node ID
    NODE_ID=$(gdbus call --session \
        --dest org.gnome.Mutter.ScreenCast \
        --object-path "$STREAM_PATH" \
        --method org.freedesktop.DBus.Properties.Get \
        org.gnome.Mutter.ScreenCast.Stream PipeWireNodeId | grep -oP 'uint32 \K\d+')

    if [ -z "$NODE_ID" ]; then
        log "ERROR: Failed to get PipeWire node ID"
        return 1
    fi
    log "PipeWire node ID: $NODE_ID"

    # Start the session
    gdbus call --session \
        --dest org.gnome.Mutter.ScreenCast \
        --object-path "$SESSION_PATH" \
        --method org.gnome.Mutter.ScreenCast.Session.Start

    log "Screen-cast session started"
    echo "$NODE_ID"
}

# Run GStreamer bridge
run_gstreamer_bridge() {
    local node_id=$1

    log "Starting GStreamer bridge: pipewiresrc($node_id) → waylandsink($WOLF_DISPLAY)"

    # Set Wolf's Wayland display for waylandsink
    export WAYLAND_DISPLAY="$WOLF_DISPLAY"

    # Run the pipeline
    # - pipewiresrc: Consume PipeWire stream (prefers DMA-BUF)
    # - videoconvert: Handle format conversion if needed
    # - waylandsink: Output to Wolf's Wayland compositor
    exec gst-launch-1.0 -v \
        pipewiresrc path="$node_id" do-timestamp=true ! \
        videoconvert ! \
        waylandsink sync=false
}

# Main
main() {
    log "Starting GNOME → Wolf bridge"
    log "Wolf display: $WOLF_DISPLAY, Resolution: ${WIDTH}x${HEIGHT}"

    wait_for_dbus || exit 1
    wait_for_pipewire || exit 1
    wait_for_screencast || exit 1

    NODE_ID=$(create_screencast_session) || exit 1

    run_gstreamer_bridge "$NODE_ID"
}

main "$@"
