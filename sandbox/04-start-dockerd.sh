#!/bin/bash
set -e

echo "🐳 Starting Wolf's isolated dockerd..."

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

# Configure dockerd with NVIDIA runtime
mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'DAEMON_JSON'
{
  "runtimes": {
    "nvidia": {
      "path": "nvidia-container-runtime",
      "runtimeArgs": []
    }
  },
  "storage-driver": "overlay2",
  "log-level": "error"
}
DAEMON_JSON

echo "Configured Wolf's dockerd with nvidia runtime support"

# Start dockerd with auto-restart supervisor loop in background
# This ensures dockerd restarts if it crashes (which would break all sandboxes)
(
    while true; do
        # Clean up stale PID files before each restart attempt
        rm -f /var/run/docker.pid /run/docker/containerd/containerd.pid 2>/dev/null || true

        echo "[$(date -Iseconds)] Starting dockerd..."
        dockerd --config-file /etc/docker/daemon.json \
            --host=unix:///var/run/docker.sock
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  dockerd exited with code $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | sed -u 's/^/[DOCKERD] /' &

DOCKERD_WRAPPER_PID=$!
echo "Started dockerd with auto-restart (wrapper PID: $DOCKERD_WRAPPER_PID)"

# Wait for dockerd to be ready
TIMEOUT=30
ELAPSED=0
until docker info >/dev/null 2>&1; do
    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "❌ ERROR: dockerd failed to start within $TIMEOUT seconds"
        echo "Check dockerd logs above for details"
        return 1  # NOTE: Use "return" not "exit" - this script is sourced by entrypoint.sh!
    fi
    echo "Waiting for dockerd... ($ELAPSED/$TIMEOUT)"
    sleep 1
    ELAPSED=$((ELAPSED + 1))
done

echo "✅ Wolf's dockerd is ready!"
docker info 2>&1 | head -5

# Enable forwarding for nested containers
iptables -P FORWARD ACCEPT
echo "✅ iptables FORWARD policy set to ACCEPT"

# Create helix_default network
if ! docker network inspect helix_default >/dev/null 2>&1; then
    echo "Creating helix_default network (subnet 172.20.0.0/16)..."
    docker network create helix_default --subnet 172.20.0.0/16 --gateway 172.20.0.1
    echo "✅ helix_default network created"
else
    echo "helix_default network already exists"
fi

# Function to load a desktop image into sandbox's dockerd
# Usage: load_desktop_image <name> <required>
#   name: desktop name (sway, zorin, ubuntu)
#   required: "true" if missing tarball is a warning, "false" for info
load_desktop_image() {
    local NAME="$1"
    local REQUIRED="${2:-false}"
    local IMAGE_NAME="helix-${NAME}"
    local TARBALL="/opt/images/${IMAGE_NAME}.tar"
    local VERSION_FILE="/opt/images/${IMAGE_NAME}.version"
    local HASH_FILE="/opt/images/${IMAGE_NAME}.tar.hash"
    local LOG_FILE="/tmp/docker-load-${NAME}.log"

    if [ ! -f "$TARBALL" ]; then
        if [ "$REQUIRED" = "true" ]; then
            echo "⚠️  ${IMAGE_NAME} tarball not found (sandboxes may fail to start)"
        else
            echo "ℹ️  ${IMAGE_NAME} tarball not found (${NAME^} desktop not available)"
        fi
        return 0
    fi

    # Read expected version from embedded metadata
    local VERSION="latest"
    if [ -f "$VERSION_FILE" ]; then
        VERSION=$(cat "$VERSION_FILE")
        echo "📦 ${IMAGE_NAME} version: ${VERSION}"
    fi

    # Check if versioned image already loaded (optimization to skip expensive docker load)
    local SHOULD_LOAD=true
    local EXPECTED_HASH=""
    if [ -f "$HASH_FILE" ]; then
        EXPECTED_HASH=$(cat "$HASH_FILE")
    fi

    # Check for versioned tag first (more reliable than :latest)
    local CURRENT_HASH=$(docker images "${IMAGE_NAME}:${VERSION}" --format '{{.ID}}' 2>/dev/null || echo "")

    if [ "$CURRENT_HASH" = "$EXPECTED_HASH" ] && [ -n "$CURRENT_HASH" ]; then
        echo "✅ ${IMAGE_NAME}:${VERSION} already loaded (hash: $CURRENT_HASH) - skipping docker load"
        SHOULD_LOAD=false
    else
        echo "📦 Loading ${IMAGE_NAME}:${VERSION} into sandbox's dockerd (current: ${CURRENT_HASH:-none}, expected: ${EXPECTED_HASH:-unknown})..."
    fi

    if [ "$SHOULD_LOAD" = true ]; then
        if docker load -i "$TARBALL" 2>&1 | tee "$LOG_FILE"; then
            # Verify versioned tag exists after load
            if docker images "${IMAGE_NAME}:${VERSION}" --format '{{.ID}}' | grep -q .; then
                echo "✅ ${IMAGE_NAME}:${VERSION} loaded successfully"
            else
                # Tarball may be from before versioning - tag it now
                echo "🏷️  Tagging ${IMAGE_NAME}:latest as ${IMAGE_NAME}:${VERSION}"
                docker tag "${IMAGE_NAME}:latest" "${IMAGE_NAME}:${VERSION}" 2>/dev/null || true
            fi
        else
            echo "⚠️  Failed to load ${IMAGE_NAME} tarball (may be corrupted or out of memory)"
            echo "   Container will continue startup - transfer fresh image with './stack build-${NAME}'"
        fi
    fi

    echo "✅ ${IMAGE_NAME}:${VERSION} ready for Wolf executor"
}

# Load desktop images (sway is required, others are optional)
load_desktop_image "sway" "true"
load_desktop_image "zorin" "false"
load_desktop_image "ubuntu" "false"
