#!/bin/bash
set -e

# This script is executed by GOW's /entrypoint.sh after all cont-init.d scripts
# At this point:
#   - dockerd is running (started by 04-start-dockerd.sh)
#   - Wolf config is initialized (05-init-wolf-config.sh)
#   - Moonlight Web config is initialized (06-init-moonlight-config.sh)
#   - RevDial clients started (07-start-revdial-clients.sh) - Wolf RevDial only
#
# IMPORTANT: Moonlight Web MUST start AFTER Wolf to avoid "LikelyOffline" errors
# Moonlight Web tries to connect to Wolf on startup to cache host info.
# If Wolf isn't ready, the connection fails and is cached as "offline".

echo "🐺 Starting Wolf (main process with automatic restart)..."

# Clean up stale wayland sockets from XDG_RUNTIME_DIR
# Wolf creates wayland-N sockets for each lobby. When lobbies fail or timeout,
# these sockets aren't always cleaned up, causing ListeningSocketSource::new_auto()
# to eventually fail with a panic.
if [ -n "$XDG_RUNTIME_DIR" ] && [ -d "$XDG_RUNTIME_DIR" ]; then
    WAYLAND_COUNT=$(ls -d $XDG_RUNTIME_DIR/wayland-* 2>/dev/null | wc -l)
    if [ "$WAYLAND_COUNT" -gt 0 ]; then
        echo "🧹 Cleaning up $WAYLAND_COUNT stale wayland sockets from $XDG_RUNTIME_DIR..."
        rm -rf $XDG_RUNTIME_DIR/wayland-* 2>/dev/null || true
        echo "✅ Wayland sockets cleaned"
    fi
fi

# Make sure Wolf config folder and socket directory exist
export WOLF_CFG_FOLDER=$HOST_APPS_STATE_FOLDER/cfg
mkdir -p $WOLF_CFG_FOLDER
mkdir -p /var/run/wolf
export WOLF_CFG_FILE=$WOLF_CFG_FOLDER/config.toml
export WOLF_PRIVATE_KEY_FILE=$WOLF_CFG_FOLDER/key.pem
export WOLF_PRIVATE_CERT_FILE=$WOLF_CFG_FOLDER/cert.pem

# Set default values for environment variables
# Auto-detect software rendering mode based on GPU_VENDOR
if [ "$GPU_VENDOR" = "none" ] || [ -z "$GPU_VENDOR" ] && [ ! -e /dev/dri/renderD128 ]; then
    echo "🖥️  Software rendering mode detected (GPU_VENDOR=$GPU_VENDOR)"
    export WOLF_RENDER_NODE="SOFTWARE"
    export WOLF_ENCODER_NODE="SOFTWARE"
    export GST_GL_DRM_DEVICE=""
    # Force Mesa to use llvmpipe for software rendering
    export LIBGL_ALWAYS_SOFTWARE=1
    export MESA_GL_VERSION_OVERRIDE=4.5
else
    export WOLF_RENDER_NODE=${WOLF_RENDER_NODE:-/dev/dri/renderD128}
    export WOLF_ENCODER_NODE=${WOLF_ENCODER_NODE:-$WOLF_RENDER_NODE}
    export GST_GL_DRM_DEVICE=${GST_GL_DRM_DEVICE:-$WOLF_ENCODER_NODE}
fi
echo "🎮 Wolf render configuration: WOLF_RENDER_NODE=$WOLF_RENDER_NODE"

# Update fake-udev path
export WOLF_DOCKER_FAKE_UDEV_PATH=${WOLF_DOCKER_FAKE_UDEV_PATH:-$HOST_APPS_STATE_FOLDER/fake-udev}
cp /wolf/fake-udev $WOLF_DOCKER_FAKE_UDEV_PATH

# Track restart state
WOLF_RESTART_COUNT=0
MAX_RESTARTS=10
RESTART_WINDOW=300  # 5 minutes
RESTART_TIMESTAMPS=()

# Function to start Wolf with supervision
start_wolf() {
    cd /wolf
    /wolf/wolf &
    WOLF_PID=$!
    echo "🐺 Wolf started (PID: $WOLF_PID)"
}

# Function to wait for Wolf HTTP server
wait_for_wolf() {
    echo "⏳ Waiting for Wolf HTTP server..."
    for i in {1..30}; do
        if timeout 1 bash -c 'cat < /dev/null > /dev/tcp/127.0.0.1/47989' 2>/dev/null; then
            echo "✅ Wolf HTTP server is ready"
            return 0
        fi
        if [ $i -eq 30 ]; then
            echo "⚠️  Wolf HTTP server not ready after 30s"
            return 1
        fi
        sleep 1
    done
}

# Function to start Moonlight Web with auto-restart (called once on first Wolf start)
MOONLIGHT_STARTED=false
start_moonlight_web() {
    if [ "$MOONLIGHT_STARTED" = true ]; then
        return
    fi

    echo "🌙 Starting Moonlight Web with auto-restart..."
    (
        cd /opt/moonlight-web
        while true; do
            echo "[$(date -Iseconds)] Starting Moonlight Web..."
            ./moonlight-web
            EXIT_CODE=$?
            echo "[$(date -Iseconds)] ⚠️  Moonlight Web exited with code $EXIT_CODE, restarting in 2s..."
            sleep 2
        done
    ) 2>&1 | sed -u 's/^/[MOONLIGHT] /' &
    echo "✅ Moonlight Web started with auto-restart"

    # Start Moonlight Web RevDial client with auto-restart (if HELIX_API_URL is set)
    if [ -n "$HELIX_API_URL" ]; then
        echo "🔗 Starting Moonlight Web RevDial client with auto-restart..."
        (
            while true; do
                echo "[$(date -Iseconds)] Starting Moonlight Web RevDial client..."
                /usr/local/bin/revdial-client \
                    -server "$HELIX_API_URL/api/v1/revdial" \
                    -runner-id "moonlight-${WOLF_INSTANCE_ID:-local}" \
                    -token "${RUNNER_TOKEN:-}" \
                    -local "127.0.0.1:8080"
                EXIT_CODE=$?
                echo "[$(date -Iseconds)] ⚠️  Moonlight Web RevDial client exited with code $EXIT_CODE, restarting in 2s..."
                sleep 2
            done
        ) 2>&1 | sed -u 's/^/[MOONLIGHT-REVDIAL] /' &
        echo "✅ Moonlight Web RevDial client started with auto-restart"
    fi

    # Start auto-pairing in background (gives Moonlight Web time to fully initialize)
    (sleep 3 && /opt/moonlight-web/auto-pair.sh) 2>&1 | sed -u 's/^/[PAIRING] /' &

    MOONLIGHT_STARTED=true
}

# Function to check if we should restart (rate limiting)
should_restart() {
    local NOW=$(date +%s)

    # Remove old timestamps outside the window
    local NEW_TIMESTAMPS=()
    for TS in "${RESTART_TIMESTAMPS[@]}"; do
        if [ $((NOW - TS)) -lt $RESTART_WINDOW ]; then
            NEW_TIMESTAMPS+=("$TS")
        fi
    done
    RESTART_TIMESTAMPS=("${NEW_TIMESTAMPS[@]}")

    # Check if we've exceeded max restarts in window
    if [ ${#RESTART_TIMESTAMPS[@]} -ge $MAX_RESTARTS ]; then
        echo "❌ Wolf has crashed $MAX_RESTARTS times in the last $RESTART_WINDOW seconds"
        echo "   Giving up to prevent crash loop. Check logs for root cause."
        return 1
    fi

    # Record this restart
    RESTART_TIMESTAMPS+=("$NOW")
    return 0
}

# Initial Wolf start
start_wolf
wait_for_wolf

# Start Moonlight Web after Wolf is ready
start_moonlight_web

# Start background cleanup daemon for stale wayland sockets
# Wolf doesn't always clean up sockets when lobby creation fails (timeout/panic)
# This runs every 5 minutes to prevent accumulation of stale sockets
(
    while true; do
        sleep 300  # 5 minutes
        if [ -n "$XDG_RUNTIME_DIR" ] && [ -d "$XDG_RUNTIME_DIR" ]; then
            # Get active lobby count from Wolf API
            ACTIVE_LOBBIES=$(curl -s --unix-socket /var/run/wolf/wolf.sock 'http://localhost/api/v1/lobbies' 2>/dev/null | grep -o '"id"' | wc -l || echo "0")
            # Count wayland socket PAIRS (socket + lock file = 2 entries per display)
            SOCKET_COUNT=$(ls $XDG_RUNTIME_DIR/wayland-*.lock 2>/dev/null | wc -l || echo "0")

            # If there are more sockets than active lobbies, we have orphans
            if [ "$SOCKET_COUNT" -gt "$ACTIVE_LOBBIES" ]; then
                ORPHANS=$((SOCKET_COUNT - ACTIVE_LOBBIES))
                echo "[$(date -Iseconds)] Found $ORPHANS orphaned wayland sockets (active: $ACTIVE_LOBBIES, sockets: $SOCKET_COUNT)"
                # Only clean up if there are MANY more sockets than lobbies (safety margin)
                if [ "$ORPHANS" -gt 10 ]; then
                    echo "[$(date -Iseconds)] Cleaning up stale wayland sockets..."
                    # Clean ALL sockets - Wolf will recreate for active lobbies
                    rm -rf $XDG_RUNTIME_DIR/wayland-* 2>/dev/null || true
                    echo "[$(date -Iseconds)] Wayland sockets cleaned"
                fi
            fi
        fi
    done
) 2>&1 | sed -u 's/^/[WAYLAND-CLEANUP] /' &
echo "✅ Wayland socket cleanup daemon started (runs every 5 minutes)"

# Main supervision loop - restart Wolf if it crashes
while true; do
    wait $WOLF_PID
    EXIT_CODE=$?

    echo "⚠️  Wolf exited with code $EXIT_CODE at $(date -Iseconds)"

    # Check if we should restart
    if ! should_restart; then
        echo "🛑 Too many restarts, exiting container"
        exit 1
    fi

    WOLF_RESTART_COUNT=$((WOLF_RESTART_COUNT + 1))
    echo "🔄 Restarting Wolf (restart #$WOLF_RESTART_COUNT)..."

    # Brief pause before restart
    sleep 2

    # Restart Wolf
    start_wolf
    wait_for_wolf || echo "⚠️  Wolf may not be fully ready, continuing supervision..."
done