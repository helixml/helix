#!/bin/bash
# GNOME ScreenCast-based screenshot for GNOME 49+
#
# Uses org.gnome.Mutter.ScreenCast D-Bus API instead of org.gnome.Shell.Screenshot
# The ScreenCast API is NOT blocked by GNOME 49's security restrictions.
#
# Usage: gnome-screencast-screenshot.sh /path/to/output.png
#
# The script:
# 1. Creates a ScreenCast session
# 2. Records the primary monitor
# 3. Captures one frame via GStreamer pipewiresrc
# 4. Saves as PNG

set -e

OUTPUT_FILE="${1:-/tmp/screenshot-$$.png}"
TIMEOUT_SECS=5

log() {
    echo "[screencast-screenshot] $(date '+%H:%M:%S') $*" >&2
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

# Wait for ScreenCast D-Bus service
log "Waiting for GNOME Mutter ScreenCast..."
if ! gdbus wait --session --timeout 5 org.gnome.Mutter.ScreenCast 2>/dev/null; then
    log "ERROR: GNOME Mutter ScreenCast not available"
    exit 1
fi

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
log "Session: $SESSION_PATH"

# Record the primary monitor
# cursor-mode: 0=hidden, 1=embedded (show cursor in capture), 2=metadata
log "Recording monitor..."
STREAM_RESULT=$(gdbus call --session \
    --dest org.gnome.Mutter.ScreenCast \
    --object-path "$SESSION_PATH" \
    --method org.gnome.Mutter.ScreenCast.Session.RecordMonitor \
    "XWAYLAND0" \
    "{'cursor-mode': <uint32 1>}" 2>/dev/null || \
    # Fallback: try recording the virtual display for headless mode
    gdbus call --session \
        --dest org.gnome.Mutter.ScreenCast \
        --object-path "$SESSION_PATH" \
        --method org.gnome.Mutter.ScreenCast.Session.RecordVirtual \
        "{'cursor-mode': <uint32 1>}")

# Extract stream path
STREAM_PATH=$(echo "$STREAM_RESULT" | grep -oP "'/[^']+'")
STREAM_PATH="${STREAM_PATH//\'/}"

if [ -z "$STREAM_PATH" ]; then
    log "ERROR: Failed to record monitor"
    log "Result was: $STREAM_RESULT"
    exit 1
fi
log "Stream: $STREAM_PATH"

# Get PipeWire node ID
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

# Start the session
log "Starting ScreenCast session..."
gdbus call --session \
    --dest org.gnome.Mutter.ScreenCast \
    --object-path "$SESSION_PATH" \
    --method org.gnome.Mutter.ScreenCast.Session.Start

# Give PipeWire a moment to set up the stream
sleep 0.3

# Capture one frame using GStreamer
# pipewiresrc reads from the PipeWire node
# num-buffers=1 captures exactly one frame
# pngenc encodes to PNG
log "Capturing frame from PipeWire node $NODE_ID..."
if timeout "$TIMEOUT_SECS" gst-launch-1.0 -q \
    pipewiresrc path="$NODE_ID" num-buffers=1 do-timestamp=true ! \
    videoconvert ! \
    pngenc ! \
    filesink location="$OUTPUT_FILE"; then

    if [ -f "$OUTPUT_FILE" ] && [ -s "$OUTPUT_FILE" ]; then
        log "Screenshot saved to $OUTPUT_FILE ($(stat -c%s "$OUTPUT_FILE") bytes)"
        echo "$OUTPUT_FILE"
        exit 0
    else
        log "ERROR: Output file is empty or missing"
        exit 1
    fi
else
    log "ERROR: GStreamer capture failed"
    exit 1
fi
