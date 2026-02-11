#!/bin/bash
# Start dockerd inside the desktop container if Docker volume is mounted.
# This is the "docker-in-desktop" mode: each desktop container runs its own
# dockerd with a volume-backed /var/lib/docker. This eliminates the need for
# per-session sibling dockerds managed by Hydra and all the bridge/veth/DNS
# infrastructure that connected desktop containers to those sibling dockerds.
#
# When /var/lib/docker is NOT a mountpoint (no volume), dockerd is not started.
# In that case, the container relies on an externally-mounted docker.sock
# (the legacy "sibling dockerd" mode).

if mountpoint -q /var/lib/docker 2>/dev/null; then
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

    # Write daemon.json
    # NOTE: No explicit "dns" setting — Docker inherits DNS from the desktop
    # container's /etc/resolv.conf, which chains through the sandbox's dockerd
    # to the host's DNS. This preserves enterprise DNS resolution.
    mkdir -p /etc/docker
    cat > /etc/docker/daemon.json <<EOF
{
    "storage-driver": "overlay2",
    "log-level": "warn"
}
EOF

    # Add NVIDIA runtime if GPU available
    if [ -e /dev/nvidia0 ] && command -v nvidia-container-runtime &>/dev/null; then
        echo "[dockerd] NVIDIA GPU detected - adding nvidia runtime"
        cat > /etc/docker/daemon.json <<EOF
{
    "storage-driver": "overlay2",
    "log-level": "warn",
    "runtimes": {
        "nvidia": {
            "path": "nvidia-container-runtime",
            "runtimeArgs": []
        }
    }
}
EOF
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
            echo "[dockerd] WARNING: dockerd not ready after 30s, continuing anyway"
        fi
        sleep 1
    done
else
    echo "[dockerd] /var/lib/docker not mounted - dockerd disabled (using external socket)"
fi
