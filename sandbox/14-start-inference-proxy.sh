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

# tee to /var/log/helix-services/inference-proxy.log so hydra's tailer
# can pick these lines up into the admin Runner Logs WS stream (see
# 12-start-compose-manager.sh for the same pattern + rationale, plus
# the `|| true` mkdir guard, truncate-on-boot, and SIGPIPE trap).
mkdir -p /var/log/helix-services 2>/dev/null || true
: > /var/log/helix-services/inference-proxy.log 2>/dev/null || true

(
    trap '' PIPE
    # Restart loop must survive a non-zero exit; the sourced entrypoint's
    # `set -e` leaks into this subshell and would otherwise kill the loop on the
    # first crash, leaving inference-proxy dead (see 10-start-hydra).
    set +e
    while true; do
        echo "[$(date -Iseconds)] Starting inference-proxy..."
        /usr/local/bin/inference-proxy --listen "$LISTEN" --compose "$ACTIVE_YAML"
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  inference-proxy exited with $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | stdbuf -oL tee -a /var/log/helix-services/inference-proxy.log | sed -u 's/^/[INF-PROXY] /' &

PROXY_PID=$!
echo "✅ inference-proxy started with auto-restart (wrapper PID: $PROXY_PID)"
