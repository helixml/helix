#!/bin/bash
# Start dockerd inside the desktop container.
# Each desktop container runs its own dockerd with a volume-backed /var/lib/docker.

if ! mountpoint -q /var/lib/docker 2>/dev/null; then
    echo "[dockerd] ERROR: /var/lib/docker is not a volume mount."
    echo "[dockerd] Docker-in-desktop mode requires a Docker volume at /var/lib/docker."
    echo "[dockerd] The container will continue but Docker will not be available."
    exit 0
fi

echo "[dockerd] /var/lib/docker is a volume mount - starting dockerd"

    # Use iptables-legacy for DinD compatibility
    if [ -d /usr/local/sbin/.iptables-legacy ]; then
        export PATH="/usr/local/sbin/.iptables-legacy:$PATH"
    fi
    # Prefer iptables-legacy if available (Docker requires it in nested containers)
    if command -v iptables-legacy &>/dev/null; then
        update-alternatives --set iptables /usr/sbin/iptables-legacy 2>/dev/null || true
        update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy 2>/dev/null || true
    fi

    # Enable cgroup v2 controller delegation for Kind/systemd containers.
    # Move all root-cgroup processes to init.scope (required by cgroup v2's
    # "no internal processes" rule), then enable all controllers for subtrees.
    if [ -f /sys/fs/cgroup/cgroup.subtree_control ]; then
        mkdir -p /sys/fs/cgroup/init.scope
        for pid in $(cat /sys/fs/cgroup/cgroup.procs 2>/dev/null); do
            echo "$pid" > /sys/fs/cgroup/init.scope/cgroup.procs 2>/dev/null || true
        done
        AVAILABLE=$(cat /sys/fs/cgroup/cgroup.controllers 2>/dev/null)
        ENABLE=""
        for ctrl in $AVAILABLE; do
            ENABLE="$ENABLE +$ctrl"
        done
        if [ -n "$ENABLE" ]; then
            echo "$ENABLE" > /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || true
        fi
        echo "[dockerd] cgroup v2 subtree controllers: $(cat /sys/fs/cgroup/cgroup.subtree_control)"
    fi

    # Compute non-overlapping address pool based on nesting depth.
    # Each depth gets its own /16 from the 10.x.0.0 range:
    #   Depth 1 (sandbox):          10.213.0.0/16 (in 04-start-dockerd.sh)
    #   Depth 2 (desktop):          10.214.0.0/16
    #   Depth 3 (H-in-H sandbox):   10.215.0.0/16
    #   Depth N:                     10.(212+N).0.0/16
    DEPTH="${HELIX_DOCKER_DEPTH:-2}"
    POOL_OCTET=$((212 + DEPTH))
    if [ "$POOL_OCTET" -gt 255 ]; then
        echo "[dockerd] WARNING: nesting depth $DEPTH exceeds address space, clamping to 10.255.0.0/16"
        POOL_OCTET=255
    fi
    echo "[dockerd] Nesting depth=$DEPTH, address pool=10.${POOL_OCTET}.0.0/16"

    # Write daemon.json
    # NOTE: No explicit "dns" setting — Docker inherits DNS from the desktop
    # container's /etc/resolv.conf, which chains through the sandbox's dockerd
    # to the host's DNS. This preserves enterprise DNS resolution.
    mkdir -p /etc/docker
    cat > /etc/docker/daemon.json <<EOF
{
    "storage-driver": "overlay2",
    "log-level": "warn",
    "default-address-pools": [
        {"base": "10.${POOL_OCTET}.0.0/16", "size": 24}
    ]
}
EOF

    # Add NVIDIA runtime if GPU available
    if [ -e /dev/nvidia0 ] && command -v nvidia-container-runtime &>/dev/null; then
        echo "[dockerd] NVIDIA GPU detected - adding nvidia runtime"
        cat > /etc/docker/daemon.json <<EOF
{
    "storage-driver": "overlay2",
    "log-level": "warn",
    "default-address-pools": [
        {"base": "10.${POOL_OCTET}.0.0/16", "size": 24}
    ],
    "runtimes": {
        "nvidia": {
            "path": "nvidia-container-runtime",
            "runtimeArgs": []
        }
    }
}
EOF
    fi

    # Enable forwarding so inner containers can reach outer networks.
    # Without this, traffic from inner compose containers can't route
    # through to the sandbox and ultimately to the host/API.
    if command -v iptables &>/dev/null; then
        iptables -P FORWARD ACCEPT 2>/dev/null || true
    fi

    # Start dockerd in background with auto-restart
    (
        while true; do
            # Clean up stale PID files before each restart attempt
            rm -f /var/run/docker.pid /run/docker/containerd/containerd.pid 2>/dev/null || true
            echo "[$(date -Iseconds)] Starting dockerd..."
            dockerd --config-file /etc/docker/daemon.json \
                --host=unix:///var/run/docker.sock 2>&1
            EXIT_CODE=$?
            echo "[$(date -Iseconds)] dockerd exited with code $EXIT_CODE, restarting in 2s..."
            sleep 2
        done
    ) | sed -u 's/^/[INNER-DOCKERD] /' &

    # Wait for socket to appear
    echo "[dockerd] Waiting for docker.sock..."
    for i in $(seq 1 30); do
        if docker.real info &>/dev/null 2>&1; then
            echo "[dockerd] dockerd is ready (attempt $i)"
            break
        fi
        if [ "$i" -eq 30 ]; then
            echo "[dockerd] FATAL: dockerd not ready after 30s"
            exit 1
        fi
        sleep 1
    done

    # Add retro user to docker group (created by dockerd)
    if id -u retro >/dev/null 2>&1; then
        usermod -aG docker retro 2>/dev/null || true
        echo "[dockerd] Added retro user to docker group"
    fi
