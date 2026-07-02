#!/bin/bash
set -e

# Default-expand every env var below (${VAR:-...}). This script is SOURCED by
# entrypoint.sh into the same shell as the other cont-init scripts, and a
# sibling (05-start-dns-proxy.sh) runs `set -u` at top level - which leaks in
# and makes any bare unset reference here a fatal "unbound variable". Only
# install.sh-launched runners set HYDRA_PRIVILEGED_MODE_ENABLED; YD-launched
# runners do not, so a bare reference aborted container init on them. Defaulting
# keeps the script correct regardless of which caller set which var.

# Skip if Hydra is not enabled
# NOTE: Use "return" not "exit" - this script is sourced by entrypoint.sh!
if [ "${HYDRA_ENABLED:-false}" != "true" ]; then
    echo "ℹ️  Hydra not enabled (HYDRA_ENABLED != true), skipping multi-Docker isolation"
    return 0
fi

echo "🐉 Starting Hydra multi-Docker isolation daemon..."

# Create required directories
mkdir -p /var/run/hydra/active
mkdir -p /hydra-data

# Start Hydra daemon in background with auto-restart.
(
    # CRITICAL: disable errexit inside the restart loop. This script is sourced
    # by entrypoint.sh with `set -e` active (and it leaks into this subshell),
    # so a non-zero hydra exit — e.g. a panic — would abort the subshell BEFORE
    # `EXIT_CODE=$?`, killing the loop and leaving hydra dead until someone
    # manually intervenes (this took a whole runner offline in prod). With
    # `set +e` the loop always restarts hydra.
    set +e
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
# Hydra waits up to 60s for Docker, so we need to wait longer here
TIMEOUT=90
ELAPSED=0
until [ -S /var/run/hydra/hydra.sock ]; do
    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "❌ ERROR: Hydra socket not ready within $TIMEOUT seconds"
        echo "ℹ️  Hydra runs in a restart loop, so it will keep trying. Continuing..."
        # Don't fail - Hydra will restart and eventually succeed when Docker is ready
        break
    fi
    echo "Waiting for Hydra socket... ($ELAPSED/$TIMEOUT)"
    sleep 1
    ELAPSED=$((ELAPSED + 1))
done

echo "✅ Hydra socket ready at /var/run/hydra/hydra.sock"

# Log privileged mode status
if [ "${HYDRA_PRIVILEGED_MODE_ENABLED:-false}" = "true" ]; then
    echo "⚠️  Hydra PRIVILEGED MODE ENABLED - host Docker access available for Helix development"
else
    echo "ℹ️  Hydra running in normal mode (isolated Docker instances per scope)"
fi

# Note: Hydra includes its own built-in RevDial client
# It reads HELIX_API_URL, RUNNER_TOKEN, SANDBOX_INSTANCE_ID from environment
echo "✅ Hydra daemon ready (RevDial client built-in)"
