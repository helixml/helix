#!/bin/bash
set -e

# Sandbox-absorbs-runner pivot: start the compose-manager daemon if the
# sandbox is registered with a control plane. Skip in local dev mode where
# there's no API to fetch assignments from.
#
# NOTE: Use "return" not "exit" — this script is sourced by entrypoint.sh.

if [ -z "$HELIX_API_URL" ] || [ -z "$RUNNER_TOKEN" ]; then
    echo "ℹ️  No HELIX_API_URL set, skipping compose-manager (local mode)"
    return 0
fi

SANDBOX_INSTANCE_ID=${SANDBOX_INSTANCE_ID:-local}
echo "🧱 Starting compose-manager for sandbox: $SANDBOX_INSTANCE_ID"

# Ensure the per-service log dir exists. Hydra's tailServiceLog reads
# /var/log/helix-services/*.log and pushes each line into the LogBuffer
# the admin Runner Logs WS streams from, so we tee into the file here
# AND keep the existing sed-prefixed stdout for `docker logs` viewers
# on docker-compose-style deployments.
mkdir -p /var/log/helix-services

# The compose-manager polls /api/v1/runners/{id}/assignment and applies
# the assigned profile by running `docker compose` against the inner
# dockerd. Auto-restart on crash.
(
    while true; do
        echo "[$(date -Iseconds)] Starting compose-manager..."
        HELIX_RUNNER_ID="$SANDBOX_INSTANCE_ID" \
        HELIX_RUNNER_TOKEN="$RUNNER_TOKEN" \
        /usr/local/bin/compose-manager \
            --api-url "$HELIX_API_URL" \
            --runner-id "$SANDBOX_INSTANCE_ID" \
            --runner-token "$RUNNER_TOKEN"
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  compose-manager exited with $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | tee -a /var/log/helix-services/compose-manager.log | sed -u 's/^/[COMPOSE-MGR] /' &

CM_PID=$!
echo "✅ compose-manager started with auto-restart (wrapper PID: $CM_PID)"
