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

            # Log available tags for debugging (helps verify docker ps will show names)
            echo "📋 Available tags for ${IMAGE_NAME}:"
            docker images "${IMAGE_NAME}" --format '   {{.Repository}}:{{.Tag}} ({{.ID}})'
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
load_desktop_image "kde" "false"

# ================================================================================
# Clean up old desktop images to free disk space
# This removes old versions of helix-sway, helix-ubuntu, etc. that are no longer
# needed after upgrading to new versions.
#
# How versioning works:
# - Each desktop image is tagged with the first 6 chars of its Docker image ID
#   (content-addressable hash), e.g., helix-sway:5874ee
# - The .version file contains this same hash (e.g., "5874ee")
# - When a new sandbox image is deployed, it contains new tarballs with new hashes
# - Old images (e.g., helix-sway:abc123) remain in nested Docker and waste space
#
# Cleanup logic:
# - Read the expected hash from each .version file
# - Keep images matching that hash OR tagged as :latest
# - Remove all other versions (old image hashes)
# ================================================================================
echo ""
echo "🧹 Cleaning up old desktop images in nested Docker..."

# First, build a list of expected versions from the embedded .version files
# These are the versions we just loaded (or already had loaded)
# This also tells us which desktop types exist (no hardcoded list needed)
declare -A EXPECTED_VERSIONS
DESKTOP_NAMES=""
for version_file in /opt/images/helix-*.version; do
    if [ -f "$version_file" ]; then
        # Extract image name from filename (e.g., helix-sway from helix-sway.version)
        IMAGE_NAME=$(basename "$version_file" .version)
        EXPECTED_VERSIONS[$IMAGE_NAME]=$(cat "$version_file")
        echo "   Expected version for $IMAGE_NAME: ${EXPECTED_VERSIONS[$IMAGE_NAME]}"
        # Build list of desktop names for grep pattern
        DESKTOP_NAME="${IMAGE_NAME#helix-}"  # Remove "helix-" prefix
        if [ -z "$DESKTOP_NAMES" ]; then
            DESKTOP_NAMES="$DESKTOP_NAME"
        else
            DESKTOP_NAMES="$DESKTOP_NAMES|$DESKTOP_NAME"
        fi
    fi
done

# Skip cleanup if no version files found (nothing to clean)
if [ -z "$DESKTOP_NAMES" ]; then
    echo "   No desktop version files found - skipping cleanup"
else
    # Get all helix-* desktop images matching known desktop types
    # Pattern is built dynamically from .version files (e.g., "sway|ubuntu|kde")
    ALL_DESKTOP_IMAGES=$(docker images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null | grep -E "^helix-($DESKTOP_NAMES):" | sort -u)

    REMOVED_COUNT=0
    KEPT_COUNT=0

    for image in $ALL_DESKTOP_IMAGES; do
        # Skip images with <none> tags
        if [[ "$image" == *":<none>"* ]]; then
            continue
        fi

        # Parse image name and tag
        IMAGE_NAME=$(echo "$image" | cut -d: -f1)
        IMAGE_TAG=$(echo "$image" | cut -d: -f2)

        # Get expected version for this desktop type
        EXPECTED_VERSION="${EXPECTED_VERSIONS[$IMAGE_NAME]:-}"

        # Safety: skip if we don't know the expected version for this desktop
        if [ -z "$EXPECTED_VERSION" ]; then
            KEPT_COUNT=$((KEPT_COUNT + 1))
            continue
        fi

        # Keep images matching the expected version from .version file
        # Remove everything else (old versions)
        if [ "$IMAGE_TAG" = "$EXPECTED_VERSION" ]; then
            KEPT_COUNT=$((KEPT_COUNT + 1))
        else
            echo "   Removing old image: $image (expected version: $EXPECTED_VERSION)"
            if docker rmi "$image" 2>/dev/null; then
                REMOVED_COUNT=$((REMOVED_COUNT + 1))
            fi
        fi
    done

    if [ "$REMOVED_COUNT" -gt 0 ]; then
        echo "✅ Cleaned up $REMOVED_COUNT old desktop image(s), kept $KEPT_COUNT current image(s)"
    else
        if [ "$KEPT_COUNT" -gt 0 ]; then
            echo "   No old desktop images to clean up (all $KEPT_COUNT images are current)"
        else
            echo "   No desktop images found to clean up"
        fi
    fi
fi

echo "✅ Desktop image cleanup complete"
