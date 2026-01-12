#!/bin/bash
set -e

# Skip if Hydra is not enabled
# NOTE: Use "return" not "exit" - this script is sourced by entrypoint.sh!
if [ "$HYDRA_ENABLED" != "true" ]; then
    echo "ℹ️  Hydra not enabled (HYDRA_ENABLED != true), skipping multi-Docker isolation"
    return 0
fi

echo "🐉 Starting Hydra multi-Docker isolation daemon..."

# Create required directories
mkdir -p /var/run/hydra/active
mkdir -p /hydra-data

# Start Hydra daemon in background with auto-restart
(
    while true; do
        echo "[$(date -Iseconds)] Starting Hydra daemon..."
        /usr/local/bin/hydra \
            --socket /var/run/hydra/hydra.sock \
            --socket-dir /var/run/hydra/active \
            --data-dir /hydra-data \
            --log-level info
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  Hydra exited with code $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | sed -u 's/^/[HYDRA] /' &

HYDRA_PID=$!
echo "✅ Hydra daemon started (PID: $HYDRA_PID)"

# Wait for Hydra socket to be ready
TIMEOUT=30
ELAPSED=0
until [ -S /var/run/hydra/hydra.sock ]; do
    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "❌ ERROR: Hydra socket not ready within $TIMEOUT seconds"
        return 1
    fi
    echo "Waiting for Hydra socket... ($ELAPSED/$TIMEOUT)"
    sleep 1
    ELAPSED=$((ELAPSED + 1))
done

echo "✅ Hydra socket ready at /var/run/hydra/hydra.sock"

# Log privileged mode status
if [ "$HYDRA_PRIVILEGED_MODE_ENABLED" = "true" ]; then
    echo "⚠️  Hydra PRIVILEGED MODE ENABLED - host Docker access available for Helix development"
else
    echo "ℹ️  Hydra running in normal mode (isolated Docker instances per scope)"
fi

# Note: Hydra includes its own built-in RevDial client
# It reads HELIX_API_URL, RUNNER_TOKEN, SANDBOX_INSTANCE_ID from environment
echo "✅ Hydra daemon ready (RevDial client built-in)"
