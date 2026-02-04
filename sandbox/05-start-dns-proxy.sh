#!/bin/bash
# Start DNS proxy AFTER dockerd (this script must run after 04-start-dockerd.sh)
# This forwards DNS from containers on the sandbox's main dockerd to enterprise DNS.
# Enables enterprise DNS resolution: desktop container → dns-proxy → Docker DNS → host DNS → enterprise DNS
#
# IMPORTANT: This binds to 10.213.0.1:53 (sandbox docker0 gateway) specifically,
# NOT 0.0.0.0:53, to allow Hydra's per-session DNS servers to bind to 10.200.X.1:53
# for container name resolution on per-session dockerd instances.

set -e

echo "🔗 Starting DNS proxy for sandbox main dockerd DNS resolution..."

# The sandbox's dockerd uses 10.213.0.0/24 for its bridge network.
# Desktop containers on this network have DNS configured to use 10.213.0.1.
# We must wait for docker0 bridge to be ready before binding.

# Wait for docker0 bridge to have 10.213.0.1 assigned
DOCKER0_GATEWAY="10.213.0.1"
MAX_WAIT=30
for i in $(seq 1 $MAX_WAIT); do
    if ip addr show docker0 2>/dev/null | grep -q "$DOCKER0_GATEWAY"; then
        echo "✅ docker0 bridge ready with gateway $DOCKER0_GATEWAY"
        break
    fi
    if [ $i -eq $MAX_WAIT ]; then
        echo "❌ Timeout waiting for docker0 bridge"
        exit 1
    fi
    sleep 1
done

# Start dns-proxy bound to the sandbox docker0 gateway specifically
# This leaves 10.200.X.1:53 addresses free for Hydra's per-session DNS servers
dns-proxy -listen "${DOCKER0_GATEWAY}:53" -upstream "127.0.0.11:53" &
DNS_PROXY_PID=$!
echo "✅ DNS proxy started (PID: $DNS_PROXY_PID)"

# Give it a moment to bind
sleep 0.5

# Verify it's running
if kill -0 $DNS_PROXY_PID 2>/dev/null; then
    echo "✅ DNS proxy is running on ${DOCKER0_GATEWAY}:53 → 127.0.0.11:53"
else
    echo "❌ DNS proxy failed to start"
    exit 1
fi
