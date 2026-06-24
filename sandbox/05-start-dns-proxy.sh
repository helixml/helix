#!/bin/bash
# Start (and SUPERVISE) the sandbox DNS proxy. Runs as a cont-init after
# 04-start-dockerd.sh. Forwards DNS from nested containers on the sandbox's
# main dockerd bridge to the outer Docker/host/enterprise DNS:
#   desktop/sandbox container → dns-proxy (10.213.0.1:53) → Docker DNS → host DNS
#
# It binds the gateway IP 10.213.0.1:53 specifically (NOT 0.0.0.0:53) so Hydra's
# per-session DNS servers stay free to bind 10.200.X.1:53.
#
# Reliability notes (this whole thing was previously a single unsupervised `&`
# process that, once dead, took down DNS for every sandbox with nothing to
# restart it — see 099156501 which also dropped it from the image):
#   - bind by GATEWAY IP, not a hard-coded bridge NAME (sandbox0 in prod,
#     docker0 in local dev), so it works in every environment;
#   - a supervisor loop restarts dns-proxy if it ever exits;
#   - the supervisor also covers startup ordering: if the gateway IP isn't up
#     yet, dns-proxy just exits and is retried until it is.

set -u

GATEWAY="10.213.0.1"
echo "🔗 Starting supervised DNS proxy on ${GATEWAY}:53 ..."

# Determine upstream DNS: explicit env > first resolv.conf nameserver > Docker DNS.
if [ -n "${DNS_UPSTREAM:-}" ]; then
    UPSTREAM_DNS="$DNS_UPSTREAM"
elif grep -q "nameserver" /etc/resolv.conf 2>/dev/null; then
    UPSTREAM_DNS="$(grep -m1 nameserver /etc/resolv.conf | awk '{print $2}'):53"
else
    UPSTREAM_DNS="127.0.0.11:53"
fi
echo "📌 DNS proxy upstream: ${UPSTREAM_DNS}"

# Best-effort wait for the gateway IP to appear (don't hard-fail boot if it
# doesn't — the supervisor retries regardless).
for _ in $(seq 1 30); do
    ip -4 addr show 2>/dev/null | grep -q "inet ${GATEWAY}/" && break
    sleep 1
done

# Supervisor: keep dns-proxy alive for the lifetime of the container.
(
    while true; do
        dns-proxy -listen "${GATEWAY}:53" -upstream "${UPSTREAM_DNS}"
        code=$?
        echo "[dns-proxy] exited (code ${code}); restarting in 2s..."
        sleep 2
    done
) | sed -u 's/^/[DNS-PROXY] /' &

echo "✅ DNS proxy supervisor started (${GATEWAY}:53 → ${UPSTREAM_DNS})"
