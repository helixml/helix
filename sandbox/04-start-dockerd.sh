#!/bin/bash
set -e

echo "🐳 Starting sandbox's isolated dockerd..."

# Clean up stale PID files (common issue with Docker restarts)
if [ -f /var/run/docker.pid ]; then
    echo "🧹 Cleaning up stale docker.pid file"
    rm -f /var/run/docker.pid
fi

# Clean up stale containerd PID file (prevents "timeout waiting for containerd to start")
if [ -f /run/docker/containerd/containerd.pid ]; then
    echo "🧹 Cleaning up stale containerd.pid file"
    rm -f /run/docker/containerd/containerd.pid
fi

# Use iptables-legacy for DinD compatibility
export PATH="/usr/local/sbin/.iptables-legacy:$PATH"
echo "Using iptables-legacy for Docker-in-Docker networking compatibility"

# The dockerd lifecycle owns the sandbox network boundary. Loading the policy
# here lets the restart loop install a fail-closed guard before dockerd restores
# persistent containers, then replace it with the full policy once ready.
# shellcheck source=/usr/local/lib/helix-sandbox-network-policy.sh
source /usr/local/lib/helix-sandbox-network-policy.sh
# shellcheck source=/usr/local/lib/helix-desktop-image-gc.sh
source /usr/local/lib/helix-desktop-image-gc.sh

# ================================================================================
# Configure dockerd with DNS and optional NVIDIA runtime
# With docker-in-desktop mode, per-session dockerds no longer run in the sandbox.
# The sandbox's dockerd only needs to launch desktop containers.
# Each desktop container runs its own dockerd internally.
# ================================================================================
mkdir -p /etc/docker

# Compute non-overlapping address pool based on nesting depth.
# Each depth gets its own /16 from the 10.x.0.0 range:
#   Depth 1 (sandbox):          10.213.0.0/16
#   Depth 2 (desktop):          10.214.0.0/16 (in 17-start-dockerd.sh)
#   Depth 3 (H-in-H sandbox):   10.215.0.0/16
#   Depth N:                     10.(212+N).0.0/16
DEPTH="${HELIX_DOCKER_DEPTH:-1}"
POOL_OCTET=$((212 + DEPTH))
if [ "$POOL_OCTET" -gt 255 ]; then
    echo "⚠️  Nesting depth $DEPTH exceeds address space, clamping to 10.255.0.0/16"
    POOL_OCTET=255
fi
echo "📍 Nesting depth=$DEPTH, address pool=10.${POOL_OCTET}.0.0/16"

# GPU_VENDOR is set in docker-compose.yaml based on the sandbox profile
if [[ "${GPU_VENDOR:-}" == "nvidia" ]]; then
    echo "🎮 GPU_VENDOR=nvidia - configuring NVIDIA container runtime"

    # Rebuild the dynamic-linker cache so libnvidia-container-cli can find
    # the driver libraries that the host's nvidia-container-toolkit mounted
    # into this container via --gpus all. The toolkit consults the ld.so
    # cache at container-create time (not at dockerd start), so refreshing
    # it here — before dockerd is forked further down the script — means
    # every subsequent `docker create` inherits the fresh cache.
    #
    # Without this, libnvidia-container-cli's lib resolution can fail
    # silently in nested-DinD: the inner container ends up with no
    # /dev/nvidia*, HELIX_RENDER_NODE=SOFTWARE, and nvh264enc fails to
    # enter PLAYING.
    #
    # `|| true` is load-bearing under `set -e`: a failed cache rebuild
    # mustn't abort dockerd startup. Stderr is silenced because the
    # usual non-fatal complaints (e.g. unfindable symlinks under
    # /usr/lib/nvidia) aren't actionable.
    ldconfig >/dev/null 2>&1 || true

    cat > /etc/docker/daemon.json <<DAEMON_JSON
{
  "runtimes": {
    "nvidia": {
      "path": "nvidia-container-runtime",
      "runtimeArgs": []
    }
  },
  "storage-driver": "overlay2",
  "log-level": "error",
  "insecure-registries": ["registry:5000"],
  "default-address-pools": [
    {"base": "10.${POOL_OCTET}.0.0/16", "size": 24}
  ]
}
DAEMON_JSON
else
    echo "ℹ️  GPU_VENDOR=${GPU_VENDOR:-unset} - NVIDIA runtime not configured"
    cat > /etc/docker/daemon.json <<DAEMON_JSON
{
  "storage-driver": "overlay2",
  "log-level": "error",
  "insecure-registries": ["registry:5000"],
  "default-address-pools": [
    {"base": "10.${POOL_OCTET}.0.0/16", "size": 24}
  ]
}
DAEMON_JSON
fi

echo "✅ Configured sandbox dockerd"

# Enable cgroup v2 controller delegation for nested containers.
# By default only cpuset/cpu/pids are delegated to subtrees.
# Kind (Kubernetes-in-Docker) needs memory+io for systemd inside node containers.
#
# cgroup v2 has a "no internal processes" constraint: a cgroup can't have both
# processes AND child cgroups with controllers. We must move all root-cgroup
# processes into a child cgroup (init.scope) before enabling controllers.
if [ -f /sys/fs/cgroup/cgroup.subtree_control ]; then
    # Step 1: Create init.scope and move all root cgroup processes there
    mkdir -p /sys/fs/cgroup/init.scope
    for pid in $(cat /sys/fs/cgroup/cgroup.procs 2>/dev/null); do
        echo "$pid" > /sys/fs/cgroup/init.scope/cgroup.procs 2>/dev/null || true
    done

    # Step 2: Enable all available controllers for subtrees
    AVAILABLE=$(cat /sys/fs/cgroup/cgroup.controllers)
    ENABLE=""
    for ctrl in $AVAILABLE; do
        ENABLE="$ENABLE +$ctrl"
    done
    if [ -n "$ENABLE" ]; then
        echo "$ENABLE" > /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || true
    fi
    echo "✅ cgroup v2 subtree controllers: $(cat /sys/fs/cgroup/cgroup.subtree_control)"
fi

# Start dockerd with auto-restart supervisor loop in background
# This ensures dockerd restarts if it crashes (which would break all sandboxes)
#
# tee to /var/log/helix-services/dockerd.log so hydra's tailer surfaces
# dockerd output in the admin Runner Logs WS stream. dockerd's output
# is noisy but invaluable when image pulls fail or the inner daemon
# misbehaves on a YD-allocated host (see T-10 family of issues).
# `|| true` + truncate-on-boot + SIGPIPE trap below: see
# 12-start-compose-manager.sh for the rationale.
mkdir -p /var/log/helix-services 2>/dev/null || true
: > /var/log/helix-services/dockerd.log 2>/dev/null || true

(
    trap '' PIPE
    # Restart loop must survive a non-zero exit of the supervised daemon. The
    # sourced entrypoint's `set -e` leaks into this subshell and would otherwise
    # abort it the moment dockerd crashes, leaving it dead and the runner wedged
    # (same class of bug that took a runner offline via hydra — see 10-start-hydra).
    set +e
    while true; do
        rm -f "$HELIX_NETWORK_READY_FILE"

        # Clean up stale PID files before each restart attempt
        rm -f /var/run/docker.pid /run/docker/containerd/containerd.pid 2>/dev/null || true

        if ! helix_install_sandbox_bootstrap_guard; then
            echo "[$(date -Iseconds)] ❌ Failed to install sandbox bootstrap firewall; retrying in 2s..."
            sleep 2
            continue
        fi

        echo "[$(date -Iseconds)] Starting dockerd..."
        dockerd --config-file /etc/docker/daemon.json \
            --host=unix:///var/run/docker.sock &
        DOCKERD_PID=$!

        DOCKERD_READY=false
        for _ in $(seq 1 30); do
            if docker info >/dev/null 2>&1; then
                DOCKERD_READY=true
                break
            fi
            if ! kill -0 "$DOCKERD_PID" 2>/dev/null; then
                break
            fi
            sleep 1
        done

        if [ "$DOCKERD_READY" != "true" ]; then
            echo "[$(date -Iseconds)] ❌ dockerd did not become ready; restarting in 2s..."
            kill -TERM "$DOCKERD_PID" 2>/dev/null || true
            wait "$DOCKERD_PID" 2>/dev/null
            sleep 2
            continue
        fi

        # Docker's forwarding default is too restrictive for nested session
        # containers. The sandbox-specific chains remain fail-closed.
        if ! iptables -w 10 -P FORWARD ACCEPT || ! helix_apply_sandbox_network_policy; then
            echo "[$(date -Iseconds)] ❌ Failed to install sandbox network policy; restarting dockerd in 2s..."
            kill -TERM "$DOCKERD_PID" 2>/dev/null || true
            wait "$DOCKERD_PID" 2>/dev/null
            sleep 2
            continue
        fi

        wait "$DOCKERD_PID"
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  dockerd exited with code $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | stdbuf -oL tee -a /var/log/helix-services/dockerd.log | sed -u 's/^/[DOCKERD] /' &

DOCKERD_WRAPPER_PID=$!
echo "Started dockerd with auto-restart (wrapper PID: $DOCKERD_WRAPPER_PID)"

# Wait for dockerd and its fail-closed network policy to be ready.
TIMEOUT=60
ELAPSED=0
until docker info >/dev/null 2>&1 && [ -f "$HELIX_NETWORK_READY_FILE" ]; do
    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "❌ ERROR: dockerd failed to start within $TIMEOUT seconds"
        echo "Check dockerd logs above for details"
        exit 1
    fi
    echo "Waiting for dockerd... ($ELAPSED/$TIMEOUT)"
    sleep 1
    ELAPSED=$((ELAPSED + 1))
done

echo "✅ Sandbox dockerd is ready!"
docker info 2>&1 | head -5

# Create /tmp/sockets for runc console sockets (required for docker exec -ti)
mkdir -p /tmp/sockets
echo "✅ Created /tmp/sockets for docker exec -ti support"

# Function to ensure a desktop image is available in sandbox's dockerd
# Supports two sources:
#   1. Registry pull (production): Read .ref file, pull from registry
#   2. Already present (development): Image transferred via local registry
#
# Registry override: Set HELIX_SANDBOX_REGISTRY to use custom registry
#   e.g., HELIX_SANDBOX_REGISTRY=internal-registry.corp.example.com
#
# Usage: load_desktop_image <name> <required>
#   name: desktop name (sway, zorin, ubuntu)
#   required: "true" if missing image is FATAL (return 1), "false" for skip
#
# Return codes:
#   0: image present (already loaded, re-tagged, or freshly pulled), OR
#      image not configured / pull failed AND required="false" (skip silently)
#   1: image not configured or pull failed AND required="true" (caller must abort)
load_desktop_image() {
    local NAME="$1"
    local REQUIRED="${2:-false}"
    local IMAGE_NAME="helix-${NAME}"
    local REF_FILE="/opt/images/${IMAGE_NAME}.ref"
    local VERSION_FILE="/opt/images/${IMAGE_NAME}.version"

    # Read expected version from .version file
    if [ ! -f "$VERSION_FILE" ]; then
        if [ "$REQUIRED" = "true" ]; then
            echo "⚠️  ${IMAGE_NAME} version file missing: ${VERSION_FILE}"
            return 1
        else
            echo "ℹ️  ${IMAGE_NAME} not configured (no version file)"
            return 0  # OK for optional images
        fi
    fi
    local VERSION=$(cat "$VERSION_FILE")

    # Check if the EXACT version already exists
    # We only skip the pull if the specific version tag exists.
    local EXISTING_ID=$(docker images "${IMAGE_NAME}:${VERSION}" --format '{{.ID}}' 2>/dev/null || echo "")
    if [ -n "$EXISTING_ID" ]; then
        echo "✅ ${IMAGE_NAME}:${VERSION} already available (ID: ${EXISTING_ID})"
        return 0
    fi

    # Check for registry-prefixed tag without a corresponding local tag.
    # In Helix-in-Helix, the sandbox's Docker data is seeded from the golden
    # cache and the image sometimes ends up as registry:5000/IMAGE:VERSION
    # without the local IMAGE:VERSION tag that Hydra needs. The root cause
    # isn't fully understood yet, but re-tagging is a cheap fix.
    local REGISTRY_PREFIXED="registry:5000/${IMAGE_NAME}:${VERSION}"
    local PREFIXED_ID=$(docker images "${REGISTRY_PREFIXED}" --format '{{.ID}}' 2>/dev/null || echo "")
    if [ -n "$PREFIXED_ID" ]; then
        echo "🔄 Found ${REGISTRY_PREFIXED} without local tag — re-tagging as ${IMAGE_NAME}:${VERSION}"
        docker tag "${REGISTRY_PREFIXED}" "${IMAGE_NAME}:${VERSION}" 2>/dev/null || true
        docker tag "${REGISTRY_PREFIXED}" "${IMAGE_NAME}:latest" 2>/dev/null || true
        # Clean up the registry-prefixed tag
        docker rmi "${REGISTRY_PREFIXED}" 2>/dev/null || true
        echo "✅ ${IMAGE_NAME}:${VERSION} re-tagged from registry-prefixed image (ID: ${PREFIXED_ID})"
        return 0
    fi

    # Registry pull (production mode - .ref file points to ghcr.io/helixml)
    if [ -f "$REF_FILE" ]; then
        local REGISTRY_REF=$(cat "$REF_FILE")
        echo "📦 ${IMAGE_NAME} registry ref: ${REGISTRY_REF}"

        # Support registry override for enterprise deployments
        if [ -n "$HELIX_SANDBOX_REGISTRY" ]; then
            local ORIGINAL_REF="$REGISTRY_REF"
            REGISTRY_REF=$(echo "$REGISTRY_REF" | sed "s|^[^/]*/|${HELIX_SANDBOX_REGISTRY}/|")
            echo "   Registry override: ${ORIGINAL_REF} -> ${REGISTRY_REF}"
        fi

        # Check if registry image already exists
        local IMAGE_ID=$(docker images "$REGISTRY_REF" --format '{{.ID}}' 2>/dev/null || echo "")
        if [ -n "$IMAGE_ID" ]; then
            echo "✅ ${REGISTRY_REF} already pulled (ID: ${IMAGE_ID})"
            echo "$REGISTRY_REF" > "/opt/images/${IMAGE_NAME}.runtime-ref"
            return 0
        fi

        # Pull from registry
        echo "🔄 Pulling ${REGISTRY_REF} from registry..."
        if docker pull "$REGISTRY_REF" 2>&1; then
            echo "✅ ${REGISTRY_REF} pulled successfully"
            echo "$REGISTRY_REF" > "/opt/images/${IMAGE_NAME}.runtime-ref"
            # Tag as local name for Hydra compatibility
            docker tag "$REGISTRY_REF" "${IMAGE_NAME}:${VERSION}" 2>/dev/null || true
            return 0
        else
            echo "⚠️  Failed to pull ${REGISTRY_REF}"
        fi
    fi

    # Dev/local fallback: the image may be in the local push registry
    # (registry:5000) even without a .ref file — e.g. `./stack build-sandbox`
    # pushed it there but the in-sandbox transfer pull was interrupted (a
    # container restart mid-transfer). Pull it directly instead of crash-looping
    # the whole sandbox host. Harmless in prod: registry:5000 won't resolve / be
    # populated, so this falls through to the FATAL below.
    local LOCAL_REGISTRY_REF="registry:5000/${IMAGE_NAME}:${VERSION}"
    echo "🔄 Local fallback: trying ${LOCAL_REGISTRY_REF} ..."
    if docker pull "$LOCAL_REGISTRY_REF" 2>&1; then
        docker tag "$LOCAL_REGISTRY_REF" "${IMAGE_NAME}:${VERSION}" 2>/dev/null || true
        docker tag "$LOCAL_REGISTRY_REF" "${IMAGE_NAME}:latest" 2>/dev/null || true
        echo "✅ ${IMAGE_NAME}:${VERSION} pulled from local push registry"
        return 0
    fi

    # Image not available
    if [ "$REQUIRED" = "true" ]; then
        echo "❌ FATAL: ${IMAGE_NAME} is a REQUIRED production desktop image and is not available."
        echo "   In development: Run './stack build-${NAME}' to build and transfer"
        echo "   In production: Check .ref file, registry access, and free disk space."
        echo "   Boot will abort so the operator can fix this rather than silently"
        echo "   running a sandbox host that cannot launch any sessions."
        return 1
    fi

    echo "ℹ️  ${IMAGE_NAME} not configured (optional)"
    return 0
}

# Reclaim obsolete, unreferenced versions before pulling. This is what lets a
# nearly-full deployment upgrade into the release containing the GC fix. The
# current configured version, :latest, one previous version by default, and
# every image referenced by a container remain protected.
cleanup_desktop_images "pre-pull"

# Load desktop images.
#
# Production desktops are pulled on every startup. Experimental desktops are
# only pulled when listed in $HELIX_EXPERIMENTAL_DESKTOPS (space-separated,
# e.g. "sway zorin"). Keep this categorization in sync with PRODUCTION_DESKTOPS
# / AVAILABLE_EXPERIMENTAL_DESKTOPS in the top-level `stack` script.
PRODUCTION_DESKTOPS=("ubuntu")
AVAILABLE_EXPERIMENTAL_DESKTOPS=("sway" "zorin" "xfce" "kde")

# Production desktops MUST load successfully. If load_desktop_image returns
# non-zero (image missing or pull failed) we hard-fail the boot here rather
# than continuing and letting hydra fail every sandbox-create later with a
# confusing "No such image" error. See the function's return-code contract
# above. This script is executed (not sourced) by s6-overlay at
# /etc/cont-init.d/40-start-dockerd.sh, so use `exit` to abort the boot.
for desktop in "${PRODUCTION_DESKTOPS[@]}"; do
    if ! load_desktop_image "$desktop" "true"; then
        echo "❌ Aborting sandbox boot: required production desktop '${desktop}' is not available."
        exit 1
    fi
done

declare -A ENABLED_EXPERIMENTAL=()
for desktop in ${HELIX_EXPERIMENTAL_DESKTOPS:-}; do
    ENABLED_EXPERIMENTAL[$desktop]=1
done

for desktop in "${AVAILABLE_EXPERIMENTAL_DESKTOPS[@]}"; do
    if [ -n "${ENABLED_EXPERIMENTAL[$desktop]:-}" ]; then
        load_desktop_image "$desktop" "false"
    else
        echo "ℹ️  helix-${desktop} is experimental; skipping pull (set HELIX_EXPERIMENTAL_DESKTOPS=\"${desktop} ...\" to enable)"
    fi
done

# ================================================================================
# Clean up old desktop images to free disk space
# This removes old versions of helix-sway, helix-ubuntu, etc. that are no longer
# needed after upgrading to new versions.
#
# CRITICAL: Pull new images BEFORE pruning old ones!
# This preserves shared layers and avoids re-downloading the full image.
# The load_desktop_image function above handles this correctly.
#
# Cleanup logic:
# - Read expected version from .version files
# - Read registry refs from .runtime-ref files for operator-visible diagnostics
# - Keep tags matching the expected version or :latest
# - Remove all other versions (old image hashes)
# ================================================================================
echo ""
echo "🧹 Cleaning up old desktop images in nested Docker..."

# Remove stopped leftover containers to allow image removal — but NEVER
# persistent (web-service) containers. Those carry a Docker restart policy so
# dockerd brings them back automatically after a reboot/dockerd restart; if the
# reaper deletes one here (in the window before dockerd has restarted it) the
# hosted service falls off its fast self-heal path and into a slow full
# redeploy. `docker container prune` has no negative label filter, so enumerate
# exited containers and skip the helix.persistent=true ones explicitly.
# ponytail: plain loop, not `mapfile < <(...)` — process substitution needs
# /dev/fd/63 which isn't resolvable in the nested-DinD container (broke the
# whole init under `set -e`, see 2.11.39-.41). Container IDs are hex, so a
# space-joined string with unquoted expansion is safe.
STOPPED_TO_REMOVE=""
for cid in $(docker ps -aq --filter "status=exited" 2>/dev/null); do
    if [ "$(docker inspect -f '{{ index .Config.Labels "helix.persistent" }}' "$cid" 2>/dev/null)" != "true" ]; then
        STOPPED_TO_REMOVE="$STOPPED_TO_REMOVE $cid"
    fi
done
if [ -n "$STOPPED_TO_REMOVE" ]; then
    echo "   Removing stopped container(s) (keeping persistent web-services)..."
    docker rm -f $STOPPED_TO_REMOVE >/dev/null 2>&1 || true
fi

cleanup_desktop_images "post-pull"

# ================================================================================
# Clean up only dangling images. Broad system pruning also removes an empty
# session bridge after the firewall creates it but before Hydra binds its API
# listener, and can discard valuable build cache.
# ================================================================================
echo ""
echo "🧹 Pruning dangling images..."

# Remove dangling images first (faster, targeted cleanup)
DANGLING_COUNT=$(docker images -f "dangling=true" -q 2>/dev/null | wc -l)
if [ "$DANGLING_COUNT" -gt 0 ]; then
    echo "   Removing $DANGLING_COUNT dangling image(s)..."
    docker image prune -f >/dev/null 2>&1 || true
fi

echo "✅ Docker cleanup complete"
