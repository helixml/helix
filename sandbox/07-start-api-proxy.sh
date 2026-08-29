#!/bin/bash
# Supervise the only host service exposed to the isolated sandbox bridge.

set -u

if [ -z "${HELIX_API_URL:-}" ]; then
    echo "❌ HELIX_API_URL is required for the sandbox API proxy"
    return 1
fi

LISTEN_ADDR="192.0.2.1:18080"
echo "🔗 Starting sandbox API proxy on ${LISTEN_ADDR} → ${HELIX_API_URL}"

mkdir -p /var/log/helix-services 2>/dev/null || true
(
    set +e
    while true; do
        sandbox-api-proxy -listen "$LISTEN_ADDR" -upstream "$HELIX_API_URL"
        echo "[$(date -Iseconds)] sandbox-api-proxy exited (code $?); restarting in 2s..."
        sleep 2
    done
) >> /var/log/helix-services/sandbox-api-proxy.log 2>&1 &

echo "✅ Sandbox API proxy supervisor started"
