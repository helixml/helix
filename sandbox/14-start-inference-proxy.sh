#!/bin/bash
set -e

# Sandbox-absorbs-runner pivot: start the inference-proxy HTTP server.
# Always start it (even in local dev mode) so /v1/models reflects the
# active profile if one is set later.
#
# NOTE: Use "return" not "exit" — this script is sourced by entrypoint.sh.

LISTEN=${HELIX_INFERENCE_PROXY_LISTEN:-0.0.0.0:8090}
ACTIVE_YAML=${HELIX_RUNNER_ACTIVE_YAML:-/etc/helix/active.yaml}

echo "🚦 Starting inference-proxy on $LISTEN (active.yaml: $ACTIVE_YAML)"

(
    while true; do
        echo "[$(date -Iseconds)] Starting inference-proxy..."
        /usr/local/bin/inference-proxy --listen "$LISTEN" --compose "$ACTIVE_YAML"
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  inference-proxy exited with $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | sed -u 's/^/[INF-PROXY] /' &

PROXY_PID=$!
echo "✅ inference-proxy started with auto-restart (wrapper PID: $PROXY_PID)"
