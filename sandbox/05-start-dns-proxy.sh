#!/bin/bash
# Start DNS proxy AFTER dockerd (this script must run after 04-start-dockerd.sh)
# This forwards DNS from containers on the sandbox's main dockerd to enterprise DNS.
# Enables enterprise DNS resolution: desktop container → dns-proxy → Docker DNS → host DNS → enterprise DNS
#
# IMPORTANT: This binds to 10.213.0.1:53 (sandbox0 bridge gateway) specifically,
# NOT 0.0.0.0:53, to allow Hydra's per-session DNS servers to bind to 10.200.X.1:53
# for container name resolution on per-session dockerd instances.

set -e

echo "🔗 Starting DNS proxy for sandbox main dockerd DNS resolution..."

# The sandbox's dockerd uses sandbox0 bridge with 10.213.0.0/24 for its network.
# Desktop containers on this network have DNS configured to use 10.213.0.1.
# We must wait for sandbox0 bridge to be ready before binding.

# Wait for sandbox0 bridge to have 10.213.0.1 assigned
SANDBOX_BRIDGE="sandbox0"
SANDBOX_GATEWAY="10.213.0.1"
MAX_WAIT=30
for i in $(seq 1 $MAX_WAIT); do
    if ip addr show "$SANDBOX_BRIDGE" 2>/dev/null | grep -q "$SANDBOX_GATEWAY"; then
        echo "✅ $SANDBOX_BRIDGE bridge ready with gateway $SANDBOX_GATEWAY"
        break
    fi
    if [ $i -eq $MAX_WAIT ]; then
        echo "❌ Timeout waiting for $SANDBOX_BRIDGE bridge"
        exit 1
    fi
    sleep 1
done

# Determine upstream DNS server
# Priority: 1) DNS_UPSTREAM env var, 2) K8s DNS from /etc/resolv.conf, 3) Docker DNS fallback
if [ -n "$DNS_UPSTREAM" ]; then
    UPSTREAM_DNS="$DNS_UPSTREAM"
    echo "📌 Using DNS_UPSTREAM env var: $UPSTREAM_DNS"
elif grep -q "nameserver" /etc/resolv.conf; then
    # Use the first nameserver from /etc/resolv.conf (typically K8s DNS)
    UPSTREAM_DNS=$(grep "nameserver" /etc/resolv.conf | head -1 | awk '{print $2}'):53
    echo "📌 Using K8s DNS from resolv.conf: $UPSTREAM_DNS"
else
    # Fallback to Docker's embedded DNS (works in local dev)
    UPSTREAM_DNS="127.0.0.11:53"
    echo "📌 Using Docker DNS fallback: $UPSTREAM_DNS"
fi

# Start dns-proxy bound to the sandbox0 gateway specifically
# This leaves 10.200.X.1:53 addresses free for Hydra's per-session DNS servers
dns-proxy -listen "${SANDBOX_GATEWAY}:53" -upstream "$UPSTREAM_DNS" &
DNS_PROXY_PID=$!
echo "✅ DNS proxy started (PID: $DNS_PROXY_PID)"

# Give it a moment to bind
sleep 0.5

# Verify it's running
if kill -0 $DNS_PROXY_PID 2>/dev/null; then
    echo "✅ DNS proxy is running on ${SANDBOX_GATEWAY}:53 → ${UPSTREAM_DNS}"
else
    echo "❌ DNS proxy failed to start"
    exit 1
fi
