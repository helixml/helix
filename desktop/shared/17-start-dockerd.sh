#!/bin/bash
# Start the per-session container engine. Desktop sessions use rootful Docker;
# unprivileged headless sessions use a rootless Podman compatibility socket.
# Org bot instances run none: their container is unprivileged and has no
# engine storage.
#
# The entrypoint sources this file, so skipping the engine must `return`:
# `exit` would end the entrypoint and stop the container.

if [ "${HELIX_CONTAINER_ENGINE:-}" = "none" ]; then
    echo "[container-engine] None for this session"
    return 0
fi

if [ "${HELIX_ROOTLESS_CONTAINER_ENGINE:-0}" = "1" ]; then
    PODMAN_DATA=/home/retro/.local/share/containers
    PODMAN_RUNTIME=/run/user/1000
    PODMAN_SOCKET=${PODMAN_RUNTIME}/podman/podman.sock
    BUILDKIT_RUNTIME=${PODMAN_RUNTIME}/buildkit
    BUILDKIT_SOCKET=${BUILDKIT_RUNTIME}/buildkitd.sock
    BUILDKIT_ROOTLESSKIT_STATE=${PODMAN_RUNTIME}/buildkit-rootlesskit
    BUILDKIT_DATA=${PODMAN_DATA}/buildkit-state
    BUILDKIT_TMP=${PODMAN_DATA}/buildkit-tmp

    if ! mountpoint -q "${PODMAN_DATA}" 2>/dev/null; then
        echo "[podman] ERROR: ${PODMAN_DATA} is not a volume mount"
        exit 1
    fi
    for device in /dev/fuse /dev/net/tun; do
        if [ ! -c "${device}" ]; then
            echo "[podman] ERROR: required device ${device} is unavailable"
            exit 1
        fi
    done

    # Docker creates missing parents for the nested storage mount as root.
    # The agent also stores Zed state below ~/.local/share, so hand those
    # parent directories back to the unprivileged user before it starts.
    install -d -m 0755 -o retro -g retro /home/retro/.local /home/retro/.local/share
    install -d -m 0700 -o retro -g retro \
        "${PODMAN_DATA}" \
        "${PODMAN_RUNTIME}" \
        "${PODMAN_RUNTIME}/podman" \
        "${BUILDKIT_RUNTIME}" \
        "${BUILDKIT_ROOTLESSKIT_STATE}" \
        "${BUILDKIT_DATA}" \
        "${BUILDKIT_TMP}"
    install -d -m 0755 -o retro -g retro /home/retro/.config/containers
    cp /opt/helix/headless-containers.conf /home/retro/.config/containers/containers.conf
    chown retro:retro /home/retro/.config/containers/containers.conf

    if ! gosu retro env \
        HOME=/home/retro \
        USER=retro \
        XDG_RUNTIME_DIR="${PODMAN_RUNTIME}" \
        /usr/bin/rootlesskit /bin/true; then
        echo "[buildkit] FATAL: the host blocked /usr/bin/rootlesskit from creating and re-executing in a user namespace"
        echo "[buildkit] Enable unprivileged user namespaces and permit /usr/bin/rootlesskit in the host security policy"
        exit 1
    fi

    rm -f "${PODMAN_SOCKET}"
    gosu retro env \
        HOME=/home/retro \
        XDG_RUNTIME_DIR="${PODMAN_RUNTIME}" \
        CONTAINERS_CONF=/home/retro/.config/containers/containers.conf \
        PODMAN_SOCKET="${PODMAN_SOCKET}" \
        bash -c '
        podman_command=(podman)
        if [ "${HELIX_DESKTOP_ROOTLESS:-0}" = "1" ]; then
            # Keep the API service in the subordinate user namespace it manages.
            podman_command+=(unshare podman)
        fi
        while true; do
            echo "[$(date -Iseconds)] Starting rootless Podman API service..."
            env -u CONTAINER_HOST -u DOCKER_HOST \
                "${podman_command[@]}" system service --time=0 "unix://${PODMAN_SOCKET}"
            EXIT_CODE=$?
            echo "[$(date -Iseconds)] Podman API service exited with code ${EXIT_CODE}, restarting in 2s..."
            sleep 2
        done
    ' 2>&1 | gosu retro sed -u 's/^/[ROOTLESS-PODMAN] /' &

    # Polled every 0.1s: the engines are usually up within a second, and every
    # tenth of a second here delays the agent's start.
    echo "[podman] Waiting for Docker-compatible API..."
    for i in $(seq 1 300); do
        if DOCKER_HOST="unix://${PODMAN_SOCKET}" docker info >/dev/null 2>&1; then
            echo "[podman] Rootless container engine is ready (attempt ${i})"
            break
        fi
        if [ "${i}" -eq 300 ]; then
            echo "[podman] FATAL: rootless container engine not ready after 30s"
            exit 1
        fi
        sleep 0.1
    done

    rm -f \
        "${BUILDKIT_SOCKET}" \
        "${BUILDKIT_ROOTLESSKIT_STATE}/api.sock" \
        "${BUILDKIT_ROOTLESSKIT_STATE}/child_pid" \
        "${BUILDKIT_ROOTLESSKIT_STATE}/lock"
    gosu retro env \
        HOME=/home/retro \
        USER=retro \
        XDG_RUNTIME_DIR="${PODMAN_RUNTIME}" \
        TMPDIR="${BUILDKIT_TMP}" \
        BUILDKIT_SOCKET="${BUILDKIT_SOCKET}" \
        BUILDKIT_ROOTLESSKIT_STATE="${BUILDKIT_ROOTLESSKIT_STATE}" \
        BUILDKIT_DATA="${BUILDKIT_DATA}" \
        bash -c '
        while true; do
            echo "[$(date -Iseconds)] Starting rootless BuildKit daemon..."
            env -u BUILDKIT_HOST /usr/bin/rootlesskit \
                --state-dir="${BUILDKIT_ROOTLESSKIT_STATE}" \
                buildkitd \
                --root="${BUILDKIT_DATA}" \
                --addr="unix://${BUILDKIT_SOCKET}" \
                --oci-worker-no-process-sandbox
            EXIT_CODE=$?
            echo "[$(date -Iseconds)] BuildKit exited with code ${EXIT_CODE}, restarting in 2s..."
            rm -f \
                "${BUILDKIT_SOCKET}" \
                "${BUILDKIT_ROOTLESSKIT_STATE}/api.sock" \
                "${BUILDKIT_ROOTLESSKIT_STATE}/child_pid" \
                "${BUILDKIT_ROOTLESSKIT_STATE}/lock"
            sleep 2
        done
    ' 2>&1 | gosu retro sed -u 's/^/[ROOTLESS-BUILDKIT] /' &

    echo "[buildkit] Waiting for rootless BuildKit API..."
    for i in $(seq 1 300); do
        if gosu retro buildctl --addr "unix://${BUILDKIT_SOCKET}" debug workers >/dev/null 2>&1; then
            echo "[buildkit] Rootless BuildKit is ready (attempt ${i})"
            break
        fi
        if [ "${i}" -eq 300 ]; then
            echo "[buildkit] FATAL: rootless BuildKit not ready after 30s"
            exit 1
        fi
        sleep 0.1
    done

    if ! gosu retro env -u BUILDX_BUILDER \
        HOME=/home/retro \
        DOCKER_HOST="unix://${PODMAN_SOCKET}" \
        docker buildx inspect helix-rootless >/dev/null 2>&1; then
        gosu retro env -u BUILDX_BUILDER \
            HOME=/home/retro \
            DOCKER_HOST="unix://${PODMAN_SOCKET}" \
            docker buildx create \
                --name helix-rootless \
                --driver remote \
                "unix://${BUILDKIT_SOCKET}"
    fi
    gosu retro env -u BUILDX_BUILDER \
        HOME=/home/retro \
        DOCKER_HOST="unix://${PODMAN_SOCKET}" \
        docker buildx use helix-rootless --default
    echo "[buildkit] Configured helix-rootless as the default Buildx builder"

    return 0
fi

if ! mountpoint -q /var/lib/docker 2>/dev/null; then
    echo "[dockerd] ERROR: /var/lib/docker is not a volume mount."
    echo "[dockerd] Docker-in-desktop mode requires a Docker volume at /var/lib/docker."
    echo "[dockerd] The container will continue but Docker will not be available."
    return 0
fi

echo "[dockerd] /var/lib/docker is a volume mount - starting dockerd"

    # Prefer iptables-legacy for DinD compatibility, but only if it works.
    # Legacy needs the ip_tables/iptable_nat kernel modules, which the container
    # can't load itself (no /lib/modules). Otherwise stay on nf_tables.
    if command -v iptables-legacy &>/dev/null && iptables-legacy -t nat -L >/dev/null 2>&1; then
        if [ -d /usr/local/sbin/.iptables-legacy ]; then
            export PATH="/usr/local/sbin/.iptables-legacy:$PATH"
        fi
        update-alternatives --set iptables /usr/sbin/iptables-legacy 2>/dev/null || true
        update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy 2>/dev/null || true
    else
        echo "[dockerd] iptables-legacy unavailable or unusable (ip_tables module not loaded?) - using nf_tables"
    fi

    # dockerd and its containers are agent work: they run in the agent CPU
    # tier that 16-cpu-tiers.sh set up (privileged desktops always have it).
    AGENT_CGROUP=/sys/fs/cgroup/desktop/agent
    if [ ! -d "${AGENT_CGROUP}/procs" ] || [ ! -d "${AGENT_CGROUP}/docker" ]; then
        echo "[dockerd] FATAL: agent CPU tier ${AGENT_CGROUP} is missing"
        exit 1
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

    # BuildKit GC policy: size-only, as fractions of the /var/lib/docker
    # filesystem (the session's or golden build's zvol).
    #   reservedSpace 10%: never prune the cache below this
    #   maxUsedSpace  25%: LRU-prune the cache above this
    #   minFreeSpace  10%: LRU-prune while the filesystem has less free space
    # No age-based (keepDuration) rules, unlike dockerd's default, which
    # evicts RUN --mount=type=cache data unused for 48h: a golden snapshot
    # freezes "last used", so age rules would delete a golden's cache mounts
    # at the first GC pass of every session cloned from a golden older than
    # the rule. Golden builds apply this same policy synchronously before the
    # snapshot (helix-workspace-setup.sh), so a golden never carries cache
    # that a session's GC would immediately evict.
    DOCKER_FS_BYTES=$(df -B1 --output=size /var/lib/docker | tail -1 | tr -d ' ')
    GC_LIMITS="\"reservedSpace\": \"$((DOCKER_FS_BYTES / 10))\", \"maxUsedSpace\": \"$((DOCKER_FS_BYTES / 4))\", \"minFreeSpace\": \"$((DOCKER_FS_BYTES / 10))\""

    # Add NVIDIA runtime if GPU available
    RUNTIMES=""
    if [ "${HELIX_HEADLESS}" != "1" ] && [ -e /dev/nvidia0 ] && command -v nvidia-container-runtime &>/dev/null; then
        echo "[dockerd] NVIDIA GPU detected - adding nvidia runtime"
        RUNTIMES=',
    "runtimes": {
        "nvidia": {
            "path": "nvidia-container-runtime",
            "runtimeArgs": []
        }
    }'
    fi

    # Write daemon.json
    # NOTE: No explicit "dns" setting — Docker inherits DNS from the desktop
    # container's /etc/resolv.conf, which chains through the sandbox's dockerd
    # to the host's DNS. This preserves enterprise DNS resolution.
    mkdir -p /etc/docker
    cat > /etc/docker/daemon.json <<EOF
{
    "storage-driver": "overlay2",
    "log-level": "warn",
    "cgroup-parent": "/desktop/agent/docker",
    "default-address-pools": [
        {"base": "10.${POOL_OCTET}.0.0/16", "size": 24}
    ],
    "builder": {
        "gc": {
            "enabled": true,
            "policy": [
                {${GC_LIMITS}},
                {"all": true, ${GC_LIMITS}}
            ]
        }
    }${RUNTIMES}
}
EOF
    echo "[dockerd] BuildKit GC: $(jq -c '.builder.gc.policy[0]' /etc/docker/daemon.json) of ${DOCKER_FS_BYTES} bytes"

    # Enable forwarding so inner containers can reach outer networks.
    # Without this, traffic from inner compose containers can't route
    # through to the sandbox and ultimately to the host/API.
    if command -v iptables &>/dev/null; then
        iptables -P FORWARD ACCEPT 2>/dev/null || true
    fi

    # Start dockerd in background with auto-restart
    # The loop checks /tmp/.dockerd-stop to allow clean shutdown (e.g. golden builds)
    (
        echo 0 > "${AGENT_CGROUP}/procs/cgroup.procs"
        while true; do
            if [ -f /tmp/.dockerd-stop ]; then
                echo "[$(date -Iseconds)] dockerd stop requested, exiting restart loop"
                break
            fi
            # Clean up stale PID files before each restart attempt
            rm -f /var/run/docker.pid /run/docker/containerd/containerd.pid 2>/dev/null || true
            echo "[$(date -Iseconds)] Starting dockerd..."
            dockerd --config-file /etc/docker/daemon.json \
                --host=unix:///var/run/docker.sock 2>&1
            EXIT_CODE=$?
            if [ -f /tmp/.dockerd-stop ]; then
                echo "[$(date -Iseconds)] dockerd exited (stop requested), not restarting"
                break
            fi
            echo "[$(date -Iseconds)] dockerd exited with code $EXIT_CODE, restarting in 2s..."
            sleep 2
        done
    ) | sed -u 's/^/[INNER-DOCKERD] /' &

    # Wait for socket to appear
    echo "[dockerd] Waiting for docker.sock..."
    for i in $(seq 1 300); do
        if docker info &>/dev/null 2>&1; then
            echo "[dockerd] dockerd is ready (attempt $i)"
            break
        fi
        if [ "$i" -eq 300 ]; then
            echo "[dockerd] FATAL: dockerd not ready after 30s"
            exit 1
        fi
        sleep 0.1
    done

    # Add retro user to docker group (created by dockerd)
    if id -u retro >/dev/null 2>&1; then
        usermod -aG docker retro 2>/dev/null || true
        echo "[dockerd] Added retro user to docker group"
    fi

    # Log the BuildKit cache this session starts with (inherited from the
    # golden snapshot, if any) next to what the golden recorded before its
    # snapshot. Delayed past BuildKit's GC pass at dockerd start, so cache the
    # GC evicts on boot shows up as a mismatch. Backgrounded: not on the
    # agent's startup path.
    (
        sleep 10
        echo "[buildkit-cache] session: $(helix-buildkit-cache-stats 2>&1)"
        if [ -f /var/lib/docker/.golden-buildkit-stats.json ]; then
            echo "[buildkit-cache] golden:  $(jq -c '.pre_snapshot' /var/lib/docker/.golden-buildkit-stats.json)"
        fi
    ) &

    # Sandboxes build through their per-session inner daemon.
    BUILDER_NAME="default"
    docker buildx use default --default
    echo "[dockerd] Using per-session Docker builder"

    echo "BUILDX_BUILDER=${BUILDER_NAME}" >> /etc/environment
    cat > /etc/profile.d/helix-buildkit.sh << PROFILE_EOF
export BUILDX_BUILDER=${BUILDER_NAME}
PROFILE_EOF
    echo "[dockerd] Set BUILDX_BUILDER=${BUILDER_NAME} globally"

    # Install the docker wrapper. It is transparent for the local Docker driver.
    if [ -f /opt/helix/docker-wrapper ]; then
        cp /opt/helix/docker-wrapper /usr/local/bin/docker
        chmod +x /usr/local/bin/docker
        echo "[dockerd] Installed docker wrapper at /usr/local/bin/docker"
    fi

    # Copy buildx builder config from root to retro user.
    # Root selected the builder above, storing instance metadata in
    # /root/.docker/buildx/. Retro needs the same metadata. We also pre-create the
    # activity directory so buildx doesn't create it as root later.
    if id -u retro >/dev/null 2>&1; then
        mkdir -p /home/retro/.docker/buildx/activity
        if [ -d /root/.docker/buildx/instances ]; then
            cp -a /root/.docker/buildx/instances /home/retro/.docker/buildx/
        fi
        if [ -f /root/.docker/buildx/current ]; then
            cp -a /root/.docker/buildx/current /home/retro/.docker/buildx/
        fi
        chown -R retro:retro /home/retro/.docker
        # Also add to retro's .bashrc so interactive shells pick it up immediately
        if ! grep -q 'BUILDX_BUILDER' /home/retro/.bashrc 2>/dev/null; then
            echo "export BUILDX_BUILDER=${BUILDER_NAME}" >> /home/retro/.bashrc
        fi
        echo "[dockerd] Copied buildx config to retro user and fixed ownership"
    fi
