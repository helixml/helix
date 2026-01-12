#!/bin/bash
set -e

# Skip if no control plane configured (local dev mode)
# NOTE: Use "return" not "exit" - this script is sourced by entrypoint.sh!
if [ -z "$HELIX_API_URL" ] || [ -z "$RUNNER_TOKEN" ]; then
    echo "ℹ️  No HELIX_API_URL set, skipping RevDial clients (local mode)"
    return 0
fi

SANDBOX_INSTANCE_ID=${WOLF_INSTANCE_ID:-${SANDBOX_INSTANCE_ID:-local}}
echo "🔗 Starting Hydra RevDial client..."
echo "   Control Plane: $HELIX_API_URL"
echo "   Sandbox Instance ID: $SANDBOX_INSTANCE_ID"

# Start Hydra API RevDial client with automatic restart (proxies Hydra Unix socket)
# Use cgo DNS resolver to work around Go DNS issues in Docker
# Use sed -u for unbuffered output (otherwise logs don't appear)
(
    while true; do
        echo "[$(date -Iseconds)] Starting Hydra RevDial client..."
        GODEBUG=netdns=cgo /usr/local/bin/revdial-client \
            -server "$HELIX_API_URL/api/v1/revdial" \
            -runner-id "sandbox-$SANDBOX_INSTANCE_ID" \
            -token "$RUNNER_TOKEN" \
            -local "unix:///var/run/wolf/wolf.sock"
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  Hydra RevDial client exited with code $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | sed -u 's/^/[HYDRA-REVDIAL] /' &
echo "✅ Hydra RevDial client started with auto-restart"
