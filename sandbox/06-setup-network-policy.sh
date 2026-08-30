#!/bin/bash
# Network-policy library for the sandbox's nested dockerd. The dockerd lifecycle
# script sources this file before starting the daemon and after every restart.
# It deliberately starts with a subnet-wide fail-closed guard, then replaces
# that guard only after the complete ordered policy has been installed.

HELIX_NETWORK_NAME="helix-sandboxes"
HELIX_NETWORK_BRIDGE="helix-sbx0"
HELIX_NETWORK_SUBNET="192.0.2.0/24"
HELIX_NETWORK_GATEWAY="192.0.2.1"
HELIX_EGRESS_CHAIN="HELIX_SANDBOX_EGRESS"
HELIX_INPUT_CHAIN="HELIX_SANDBOX_INPUT"
HELIX_NAT_CHAIN="HELIX_SANDBOX_NAT"
HELIX_BOOTSTRAP_CHAIN="HELIX_SANDBOX_BOOTSTRAP"
HELIX_IPV6_CHAIN="HELIX_SANDBOX_IPV6"
HELIX_NETWORK_READY_FILE="/run/helix-sandbox-network-ready"

helix_iptables() {
    iptables -w 10 "$@"
}

helix_ip6tables() {
    ip6tables -w 10 "$@"
}

helix_reset_iptables_chain() {
    local table="$1"
    local chain="$2"
    helix_iptables -t "$table" -N "$chain" 2>/dev/null || true
    helix_iptables -t "$table" -F "$chain"
}

helix_remove_all_iptables_rules() {
    local table="$1"
    local chain="$2"
    shift 2
    while helix_iptables -t "$table" -C "$chain" "$@" 2>/dev/null; do
        helix_iptables -t "$table" -D "$chain" "$@"
    done
}

helix_sandbox_dns_gateway() {
    local depth="${HELIX_DOCKER_DEPTH:-1}"
    case "$depth" in
        ''|*[!0-9]*) depth=1 ;;
    esac
    local octet=$((212 + depth))
    if [ "$octet" -gt 255 ]; then
        octet=255
    fi
    printf '10.%d.0.1' "$octet"
}

# Install this before dockerd starts. Containers with restart=unless-stopped can
# otherwise transmit as soon as dockerd restores them, before the full policy is
# ready. Matching the fixed source subnet does not require the bridge to exist.
helix_install_sandbox_bootstrap_guard() {
    rm -f "$HELIX_NETWORK_READY_FILE"

    helix_reset_iptables_chain filter "$HELIX_BOOTSTRAP_CHAIN"
    helix_iptables -A "$HELIX_BOOTSTRAP_CHAIN" -s "$HELIX_NETWORK_SUBNET" \
        -j REJECT --reject-with icmp-net-prohibited

    helix_iptables -N DOCKER-USER 2>/dev/null || true
    helix_remove_all_iptables_rules filter DOCKER-USER -j "$HELIX_BOOTSTRAP_CHAIN"
    helix_iptables -I DOCKER-USER 1 -j "$HELIX_BOOTSTRAP_CHAIN"

    helix_remove_all_iptables_rules filter INPUT -j "$HELIX_BOOTSTRAP_CHAIN"
    helix_iptables -I INPUT 1 -j "$HELIX_BOOTSTRAP_CHAIN"
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

helix_configure_ipv6_policy() {
    if { [ -r /proc/sys/net/ipv6/conf/all/disable_ipv6 ] && \
         [ "$(cat /proc/sys/net/ipv6/conf/all/disable_ipv6)" = "1" ]; } || \
       [ ! -s /proc/net/if_inet6 ]; then
        echo "ℹ️  IPv6 is disabled; skipping IPv6 sandbox rules"
        return 0
    fi
    if ! command -v ip6tables >/dev/null 2>&1; then
        echo "❌ IPv6 is enabled but ip6tables is unavailable"
        return 1
    fi

    helix_ip6tables -N "$HELIX_IPV6_CHAIN" 2>/dev/null || true
    helix_ip6tables -F "$HELIX_IPV6_CHAIN"
    helix_ip6tables -A "$HELIX_IPV6_CHAIN" -j REJECT --reject-with icmp6-port-unreachable

    while helix_ip6tables -C FORWARD -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_IPV6_CHAIN" 2>/dev/null; do
        helix_ip6tables -D FORWARD -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_IPV6_CHAIN"
    done
    helix_ip6tables -I FORWARD 1 -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_IPV6_CHAIN"

    while helix_ip6tables -C INPUT -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_IPV6_CHAIN" 2>/dev/null; do
        helix_ip6tables -D INPUT -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_IPV6_CHAIN"
    done
    helix_ip6tables -I INPUT 1 -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_IPV6_CHAIN"
}

helix_apply_sandbox_network_policy() {
    helix_ensure_sandbox_network

    local dns_gateway
    dns_gateway="$(helix_sandbox_dns_gateway)"

    # Build each chain from scratch while the bootstrap guard still blocks the
    # sandbox subnet. This preserves rule ordering after partial failure/drift.
    helix_reset_iptables_chain filter "$HELIX_EGRESS_CHAIN"
    helix_iptables -A "$HELIX_EGRESS_CHAIN" \
        -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN
    helix_iptables -A "$HELIX_EGRESS_CHAIN" -d "${dns_gateway}/32" \
        -p udp --dport 53 -j RETURN
    helix_iptables -A "$HELIX_EGRESS_CHAIN" -d "${dns_gateway}/32" \
        -p tcp --dport 53 -j RETURN

    # macOS/UTM exposes only the VideoToolbox frame-export listener through
    # QEMU's SLiRP gateway. The installer exports the port on every platform,
    # so require the virtio runtime before opening this exception. Do not allow
    # the rest of 10.0.2.2.
    if [ "${GPU_VENDOR:-}" = "virtio" ]; then
        if [ -z "${HELIX_FRAME_EXPORT_PORT:-}" ]; then
            echo "❌ HELIX_FRAME_EXPORT_PORT is required for virtio frame export"
            return 1
        fi
        case "$HELIX_FRAME_EXPORT_PORT" in
            ''|*[!0-9]*) echo "❌ Invalid HELIX_FRAME_EXPORT_PORT"; return 1 ;;
        esac
        if [ "$HELIX_FRAME_EXPORT_PORT" -lt 1 ] || [ "$HELIX_FRAME_EXPORT_PORT" -gt 65535 ]; then
            echo "❌ HELIX_FRAME_EXPORT_PORT must be between 1 and 65535"
            return 1
        fi
        helix_iptables -A "$HELIX_EGRESS_CHAIN" -d 10.0.2.2/32 \
            -p tcp --dport "$HELIX_FRAME_EXPORT_PORT" -j RETURN
    fi

    local allow_cidrs cidr
    allow_cidrs="$(printf '%s' "${HELIX_SANDBOX_EGRESS_ALLOW_CIDRS:-}" | tr ',' ' ')"
    for cidr in $allow_cidrs; do
        helix_iptables -A "$HELIX_EGRESS_CHAIN" -d "$cidr" -j RETURN
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
        helix_iptables -A "$HELIX_EGRESS_CHAIN" -d "$blocked_cidr" \
            -j REJECT --reject-with icmp-net-prohibited
    done
    helix_iptables -A "$HELIX_EGRESS_CHAIN" -j RETURN

    helix_reset_iptables_chain filter "$HELIX_INPUT_CHAIN"
    helix_iptables -A "$HELIX_INPUT_CHAIN" \
        -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN
    helix_iptables -A "$HELIX_INPUT_CHAIN" -d "${HELIX_NETWORK_GATEWAY}/32" \
        -p tcp --dport 18080 -j RETURN
    helix_iptables -A "$HELIX_INPUT_CHAIN" -d "${dns_gateway}/32" \
        -p udp --dport 53 -j RETURN
    helix_iptables -A "$HELIX_INPUT_CHAIN" -d "${dns_gateway}/32" \
        -p tcp --dport 53 -j RETURN
    helix_iptables -A "$HELIX_INPUT_CHAIN" -j REJECT --reject-with icmp-port-unreachable

    helix_reset_iptables_chain nat "$HELIX_NAT_CHAIN"
    helix_iptables -t nat -A "$HELIX_NAT_CHAIN" -d "${HELIX_NETWORK_GATEWAY}/32" \
        -p tcp --dport 8080 -j REDIRECT --to-ports 18080

    helix_iptables -N DOCKER-USER 2>/dev/null || true
    helix_remove_all_iptables_rules filter DOCKER-USER -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_EGRESS_CHAIN"
    helix_iptables -I DOCKER-USER 1 -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_EGRESS_CHAIN"

    helix_remove_all_iptables_rules filter INPUT -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_INPUT_CHAIN"
    helix_iptables -I INPUT 1 -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_INPUT_CHAIN"

    helix_remove_all_iptables_rules nat PREROUTING -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_NAT_CHAIN"
    helix_iptables -t nat -I PREROUTING 1 -i "$HELIX_NETWORK_BRIDGE" -j "$HELIX_NAT_CHAIN"

    helix_configure_ipv6_policy

    # The full hooks are installed. Removing the guard last prevents an egress
    # gap even when policy reconciliation follows a dockerd restart.
    helix_remove_all_iptables_rules filter DOCKER-USER -j "$HELIX_BOOTSTRAP_CHAIN"
    helix_remove_all_iptables_rules filter INPUT -j "$HELIX_BOOTSTRAP_CHAIN"
    helix_iptables -F "$HELIX_BOOTSTRAP_CHAIN"

    touch "$HELIX_NETWORK_READY_FILE"
    echo "✅ Isolated sandbox network ready: ${HELIX_NETWORK_SUBNET} via ${HELIX_NETWORK_BRIDGE}"
}
