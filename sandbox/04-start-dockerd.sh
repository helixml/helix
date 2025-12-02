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

# Load helix-sway image into sandbox's dockerd (with version-based management)
if [ -f /opt/images/helix-sway.tar ]; then
    # Read expected version from embedded metadata
    SWAY_VERSION="latest"
    if [ -f /opt/images/helix-sway.version ]; then
        SWAY_VERSION=$(cat /opt/images/helix-sway.version)
        echo "📦 helix-sway version: ${SWAY_VERSION}"
    fi

    # Check if versioned image already loaded (optimization to skip expensive docker load)
    SHOULD_LOAD=true
    EXPECTED_HASH=""
    if [ -f /opt/images/helix-sway.tar.hash ]; then
        EXPECTED_HASH=$(cat /opt/images/helix-sway.tar.hash)
    fi

    # Check for versioned tag first (more reliable than :latest)
    CURRENT_HASH=$(docker images "helix-sway:${SWAY_VERSION}" --format '{{.ID}}' 2>/dev/null || echo "")

    if [ "$CURRENT_HASH" = "$EXPECTED_HASH" ] && [ -n "$CURRENT_HASH" ]; then
        echo "✅ helix-sway:${SWAY_VERSION} already loaded (hash: $CURRENT_HASH) - skipping docker load"
        SHOULD_LOAD=false
    else
        echo "📦 Loading helix-sway:${SWAY_VERSION} into sandbox's dockerd (current: ${CURRENT_HASH:-none}, expected: ${EXPECTED_HASH:-unknown})..."
    fi

    if [ "$SHOULD_LOAD" = true ]; then
        if docker load -i /opt/images/helix-sway.tar 2>&1 | tee /tmp/docker-load.log; then
            # Verify both tags exist after load
            if docker images "helix-sway:${SWAY_VERSION}" --format '{{.ID}}' | grep -q .; then
                echo "✅ helix-sway:${SWAY_VERSION} loaded successfully"
            else
                # Tarball may be from before versioning - tag it now
                echo "🏷️  Tagging helix-sway:latest as helix-sway:${SWAY_VERSION}"
                docker tag helix-sway:latest "helix-sway:${SWAY_VERSION}" 2>/dev/null || true
            fi
        else
            echo "⚠️  Failed to load helix-sway tarball (may be corrupted or out of memory)"
            echo "   Container will continue startup - transfer fresh image with './stack build-sway'"
        fi
    fi

    # Note: Wolf executor reads /opt/images/helix-sway.version directly
    echo "✅ helix-sway:${SWAY_VERSION} ready for Wolf executor"
else
    echo "⚠️  helix-sway tarball not found (sandboxes may fail to start)"
fi

# Load helix-zorin image into sandbox's dockerd (optional - with version-based management)
if [ -f /opt/images/helix-zorin.tar ]; then
    # Read expected version from embedded metadata
    ZORIN_VERSION="latest"
    if [ -f /opt/images/helix-zorin.version ]; then
        ZORIN_VERSION=$(cat /opt/images/helix-zorin.version)
        echo "📦 helix-zorin version: ${ZORIN_VERSION}"
    fi

    # Check if versioned image already loaded (optimization to skip expensive docker load)
    SHOULD_LOAD=true
    EXPECTED_HASH=""
    if [ -f /opt/images/helix-zorin.tar.hash ]; then
        EXPECTED_HASH=$(cat /opt/images/helix-zorin.tar.hash)
    fi

    # Check for versioned tag first (more reliable than :latest)
    CURRENT_HASH=$(docker images "helix-zorin:${ZORIN_VERSION}" --format '{{.ID}}' 2>/dev/null || echo "")

    if [ "$CURRENT_HASH" = "$EXPECTED_HASH" ] && [ -n "$CURRENT_HASH" ]; then
        echo "✅ helix-zorin:${ZORIN_VERSION} already loaded (hash: $CURRENT_HASH) - skipping docker load"
        SHOULD_LOAD=false
    else
        echo "📦 Loading helix-zorin:${ZORIN_VERSION} into sandbox's dockerd (current: ${CURRENT_HASH:-none}, expected: ${EXPECTED_HASH:-unknown})..."
    fi

    if [ "$SHOULD_LOAD" = true ]; then
        if docker load -i /opt/images/helix-zorin.tar 2>&1 | tee /tmp/docker-load-zorin.log; then
            # Verify both tags exist after load
            if docker images "helix-zorin:${ZORIN_VERSION}" --format '{{.ID}}' | grep -q .; then
                echo "✅ helix-zorin:${ZORIN_VERSION} loaded successfully"
            else
                # Tarball may be from before versioning - tag it now
                echo "🏷️  Tagging helix-zorin:latest as helix-zorin:${ZORIN_VERSION}"
                docker tag helix-zorin:latest "helix-zorin:${ZORIN_VERSION}" 2>/dev/null || true
            fi
        else
            echo "⚠️  Failed to load helix-zorin tarball (may be corrupted or out of memory)"
            echo "   Container will continue startup - transfer fresh image with './stack build-zorin'"
        fi
    fi

    echo "✅ helix-zorin:${ZORIN_VERSION} ready for Wolf executor"
else
    echo "ℹ️  helix-zorin tarball not found (Zorin desktop not available)"
fi
