#!/bin/bash
# Start DNS proxy before dockerd
# This forwards DNS from nested containers (172.17.0.1:53) to outer Docker DNS (127.0.0.11:53)
# Enables enterprise DNS resolution: dev container → dns-proxy → Docker DNS → host DNS → enterprise DNS

set -e

echo "🔗 Starting DNS proxy for nested container DNS resolution..."

# The DNS proxy needs to listen on 172.17.0.1, which is the default bridge gateway
# that dockerd will create. But dockerd hasn't started yet, so 172.17.0.1 doesn't exist.
# Solution: Listen on 0.0.0.0:53 and let iptables/routing handle it, OR wait for bridge.

# Actually, we need to configure the DNS server address that nested dockerd will use.
# Since 172.17.0.1 won't exist until nested dockerd starts its bridge, we need to:
# 1. Pick an IP we control (the sandbox's own IP in the outer network)
# 2. Or use 0.0.0.0 and configure daemon.json to use 127.0.0.1

# Simplest approach: bind to 0.0.0.0:53 so it's reachable from nested containers
# via any IP that routes to the sandbox (including 172.17.0.1 once dockerd starts)

# Start dns-proxy in background
dns-proxy -listen "0.0.0.0:53" -upstream "127.0.0.11:53" &
DNS_PROXY_PID=$!
echo "✅ DNS proxy started (PID: $DNS_PROXY_PID)"

# Give it a moment to bind
sleep 0.5

# Verify it's running
if kill -0 $DNS_PROXY_PID 2>/dev/null; then
    echo "✅ DNS proxy is running on 0.0.0.0:53 → 127.0.0.11:53"
else
    echo "❌ DNS proxy failed to start"
    return 1
fi
