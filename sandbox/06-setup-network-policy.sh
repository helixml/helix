#!/bin/bash
# Host-enforced network boundary for untrusted session containers.
# This script is sourced by the sandbox entrypoint after dockerd is ready and
# before Hydra starts.

set -u

export PATH="/usr/local/sbin/.iptables-legacy:${PATH}"

HELIX_NETWORK_NAME="helix-sandboxes"
HELIX_NETWORK_BRIDGE="helix-sbx0"
HELIX_EGRESS_CHAIN="HELIX_SANDBOX_EGRESS"
HELIX_INPUT_CHAIN="HELIX_SANDBOX_INPUT"

# RFC 5737 TEST-NET-1 is deliberately outside dockerd's depth-specific
# address pools and can be reused safely in each nested network namespace.
HELIX_NETWORK_SUBNET="192.0.2.0/24"
HELIX_NETWORK_GATEWAY="192.0.2.1"

helix_ensure_iptables_rule() {
    local table="$1"
    local chain="$2"
    shift 2
    if ! iptables -t "$table" -C "$chain" "$@" 2>/dev/null; then
        iptables -t "$table" -A "$chain" "$@"
    fi
}

helix_ensure_ip6tables_rule() {
    local chain="$1"
    shift
    if ! ip6tables -C "$chain" "$@" 2>/dev/null; then
        ip6tables -A "$chain" "$@"
    fi
}

helix_ensure_sandbox_network() {
    if ! docker network inspect "$HELIX_NETWORK_NAME" >/dev/null 2>&1; then
        docker network create \
            --driver bridge \
            --subnet "$HELIX_NETWORK_SUBNET" \
            --gateway "$HELIX_NETWORK_GATEWAY" \
            --opt "com.docker.network.bridge.name=${HELIX_NETWORK_BRIDGE}" \
            --opt "com.docker.network.bridge.enable_icc=false" \
            "$HELIX_NETWORK_NAME" >/dev/null
    fi

    local actual_subnet actual_gateway actual_bridge
    actual_subnet="$(docker network inspect "$HELIX_NETWORK_NAME" --format '{{(index .IPAM.Config 0).Subnet}}')"
    actual_gateway="$(docker network inspect "$HELIX_NETWORK_NAME" --format '{{(index .IPAM.Config 0).Gateway}}')"
    actual_bridge="$(docker network inspect "$HELIX_NETWORK_NAME" --format '{{index .Options "com.docker.network.bridge.name"}}')"
    if [ "$actual_subnet" != "$HELIX_NETWORK_SUBNET" ] || \
       [ "$actual_gateway" != "$HELIX_NETWORK_GATEWAY" ] || \
       [ "$actual_bridge" != "$HELIX_NETWORK_BRIDGE" ]; then
        echo "❌ Existing ${HELIX_NETWORK_NAME} configuration does not match the required isolation policy"
        return 1
    fi
}

helix_ensure_sandbox_firewall() {
    iptables -N "$HELIX_EGRESS_CHAIN" 2>/dev/null || true
    iptables -N "$HELIX_INPUT_CHAIN" 2>/dev/null || true

    helix_ensure_iptables_rule filter "$HELIX_EGRESS_CHAIN" \
        -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN

    local allow_cidrs
    allow_cidrs="$(printf '%s' "${HELIX_SANDBOX_EGRESS_ALLOW_CIDRS:-}" | tr ',' ' ')"
    local cidr
    for cidr in $allow_cidrs; do
        helix_ensure_iptables_rule filter "$HELIX_EGRESS_CHAIN" -d "$cidr" -j RETURN
    done

    local blocked_cidr
    for blocked_cidr in \
        0.0.0.0/8 \
        10.0.0.0/8 \
        100.64.0.0/10 \
        127.0.0.0/8 \
        169.254.0.0/16 \
        172.16.0.0/12 \
        192.0.0.0/24 \
        192.168.0.0/16 \
        198.18.0.0/15 \
        224.0.0.0/4 \
        240.0.0.0/4; do
        helix_ensure_iptables_rule filter "$HELIX_EGRESS_CHAIN" \
            -d "$blocked_cidr" -j REJECT --reject-with icmp-net-prohibited
    done
    helix_ensure_iptables_rule filter "$HELIX_EGRESS_CHAIN" -j RETURN

    helix_ensure_iptables_rule filter "$HELIX_INPUT_CHAIN" \
        -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN
    helix_ensure_iptables_rule filter "$HELIX_INPUT_CHAIN" \
        -d "${HELIX_NETWORK_GATEWAY}/32" -p tcp --dport 18080 -j RETURN
    helix_ensure_iptables_rule filter "$HELIX_INPUT_CHAIN" \
        -j REJECT --reject-with icmp-port-unreachable

    # Older project startup scripts use outer-api:8080 to reach the Helix API.
    # Keep that address working, but redirect it to the same constrained proxy
    # rather than exposing a sandbox-host service on port 8080.
    helix_ensure_iptables_rule nat PREROUTING \
        -i "$HELIX_NETWORK_BRIDGE" -d "${HELIX_NETWORK_GATEWAY}/32" \
        -p tcp --dport 8080 -j REDIRECT --to-ports 18080

    if ! iptables -C DOCKER-USER -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_EGRESS_CHAIN" 2>/dev/null; then
        iptables -I DOCKER-USER 1 -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_EGRESS_CHAIN"
    fi
    if ! iptables -C INPUT -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_INPUT_CHAIN" 2>/dev/null; then
        iptables -I INPUT 1 -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_INPUT_CHAIN"
    fi

    # The isolated bridge has no IPv6 subnet. Reject any manually configured
    # IPv6 route instead of allowing it to bypass the IPv4 policy.
    ip6tables -N DOCKER-USER 2>/dev/null || true
    helix_ensure_ip6tables_rule DOCKER-USER -i "$HELIX_NETWORK_BRIDGE" -j REJECT
    helix_ensure_ip6tables_rule INPUT -i "$HELIX_NETWORK_BRIDGE" -j REJECT
}

helix_ensure_sandbox_network
helix_ensure_sandbox_firewall
echo "✅ Isolated sandbox network ready: ${HELIX_NETWORK_SUBNET} via ${HELIX_NETWORK_BRIDGE}"

mkdir -p /var/log/helix-services 2>/dev/null || true
(
    set +e
    while true; do
        if docker info >/dev/null 2>&1; then
            helix_ensure_sandbox_network && helix_ensure_sandbox_firewall
        fi
        sleep 15
    done
) >> /var/log/helix-services/network-policy.log 2>&1 &

echo "✅ Sandbox network-policy supervisor started"
