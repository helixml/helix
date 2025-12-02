#!/bin/bash
set -e

# Skip if no control plane configured (local dev mode)
# NOTE: Use "return" not "exit" - this script is sourced by entrypoint.sh!
if [ -z "$HELIX_API_URL" ] || [ -z "$RUNNER_TOKEN" ]; then
    echo "ℹ️  No HELIX_API_URL set, skipping RevDial clients (local mode)"
    return 0
fi

WOLF_INSTANCE_ID=${WOLF_INSTANCE_ID:-local}
echo "🔗 Starting Wolf RevDial client..."
echo "   Control Plane: $HELIX_API_URL"
echo "   Wolf Instance ID: $WOLF_INSTANCE_ID"

# Start Wolf API RevDial client with automatic restart (proxies Wolf Unix socket)
# Use cgo DNS resolver to work around Go DNS issues in Docker
# Use sed -u for unbuffered output (otherwise logs don't appear)
(
    while true; do
        echo "[$(date -Iseconds)] Starting Wolf RevDial client..."
        GODEBUG=netdns=cgo /usr/local/bin/revdial-client \
            -server "$HELIX_API_URL/api/v1/revdial" \
            -runner-id "wolf-$WOLF_INSTANCE_ID" \
            -token "$RUNNER_TOKEN" \
            -local "unix:///var/run/wolf/wolf.sock"
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  Wolf RevDial client exited with code $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | sed -u 's/^/[WOLF-REVDIAL] /' &
echo "✅ Wolf RevDial client started with auto-restart"

# NOTE: Moonlight Web RevDial client is started in startup-app.sh AFTER Moonlight Web starts
# This ensures Moonlight Web is ready to accept connections before RevDial connects
echo "ℹ️  Moonlight Web RevDial will start after Moonlight Web (in startup-app.sh)"
