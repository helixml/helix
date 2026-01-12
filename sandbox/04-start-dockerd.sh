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

echo "Configured sandbox dockerd with nvidia runtime support"

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

# Wait for dockerd to be ready (initial startup)
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

echo "✅ Sandbox dockerd is ready!"
docker info 2>&1 | head -5

# Create /tmp/sockets for runc console sockets (required for docker exec -ti)
mkdir -p /tmp/sockets
echo "✅ Created /tmp/sockets for docker exec -ti support"

# Enable forwarding for nested containers
iptables -P FORWARD ACCEPT
echo "✅ iptables FORWARD policy set to ACCEPT"

# Function to load a desktop image into sandbox's dockerd
# Supports two modes:
#   1. Registry pull (preferred): Read .ref file, pull from registry
#   2. Tarball load (fallback): Load from embedded .tar file
#
# Registry override: Set HELIX_SANDBOX_REGISTRY to use custom registry
#   e.g., HELIX_SANDBOX_REGISTRY=internal-registry.corp.example.com
#
# Usage: load_desktop_image <name> <required>
#   name: desktop name (sway, zorin, ubuntu)
#   required: "true" if missing image is a warning, "false" for info
load_desktop_image() {
    local NAME="$1"
    local REQUIRED="${2:-false}"
    local IMAGE_NAME="helix-${NAME}"
    local REF_FILE="/opt/images/${IMAGE_NAME}.ref"
    local TARBALL="/opt/images/${IMAGE_NAME}.tar"
    local VERSION_FILE="/opt/images/${IMAGE_NAME}.version"
    local LOG_FILE="/tmp/docker-load-${NAME}.log"

    # Read expected version
    local VERSION="latest"
    if [ -f "$VERSION_FILE" ]; then
        VERSION=$(cat "$VERSION_FILE")
    fi

    # =========================================================================
    # Mode 1: Registry pull (if .ref file exists)
    # =========================================================================
    if [ -f "$REF_FILE" ]; then
        local REGISTRY_REF=$(cat "$REF_FILE")
        echo "📦 ${IMAGE_NAME} registry ref: ${REGISTRY_REF}"

        # Support registry override for enterprise deployments
        if [ -n "$HELIX_SANDBOX_REGISTRY" ]; then
            # Replace registry host in ref
            # e.g., registry.helixml.tech/helix/helix-sway:v1.2.3 -> internal.corp/helix/helix-sway:v1.2.3
            local ORIGINAL_REF="$REGISTRY_REF"
            REGISTRY_REF=$(echo "$REGISTRY_REF" | sed "s|^[^/]*/|${HELIX_SANDBOX_REGISTRY}/|")
            echo "   Registry override: ${ORIGINAL_REF} -> ${REGISTRY_REF}"
        fi

        # Check if image already exists locally
        local IMAGE_ID=$(docker images "$REGISTRY_REF" --format '{{.ID}}' 2>/dev/null || echo "")
        if [ -n "$IMAGE_ID" ]; then
            echo "✅ ${REGISTRY_REF} already pulled (ID: ${IMAGE_ID})"
            # Write the ref to a runtime file for Hydra to use
            echo "$REGISTRY_REF" > "/opt/images/${IMAGE_NAME}.runtime-ref"
            return 0
        fi

        # Pull from registry
        echo "🔄 Pulling ${REGISTRY_REF} from registry..."
        if docker pull "$REGISTRY_REF" 2>&1 | tee "$LOG_FILE"; then
            echo "✅ ${REGISTRY_REF} pulled successfully"
            # Write the ref to a runtime file for Hydra to use
            echo "$REGISTRY_REF" > "/opt/images/${IMAGE_NAME}.runtime-ref"
            # Tag as local name for backward compatibility
            docker tag "$REGISTRY_REF" "${IMAGE_NAME}:${VERSION}" 2>/dev/null || true
            docker tag "$REGISTRY_REF" "${IMAGE_NAME}:latest" 2>/dev/null || true
            echo "📋 Available tags:"
            docker images --filter "reference=${IMAGE_NAME}*" --format '   {{.Repository}}:{{.Tag}} ({{.ID}})'
            return 0
        else
            echo "⚠️  Failed to pull ${REGISTRY_REF} - trying tarball fallback..."
        fi
    fi

    # =========================================================================
    # Mode 2: Tarball load (fallback)
    # =========================================================================
    if [ ! -f "$TARBALL" ]; then
        if [ "$REQUIRED" = "true" ]; then
            echo "⚠️  ${IMAGE_NAME} not available (no .ref or .tar file)"
            echo "   Sandboxes using this desktop type may fail to start"
        else
            echo "ℹ️  ${IMAGE_NAME} not available (${NAME^} desktop not configured)"
        fi
        return 0
    fi

    echo "📦 ${IMAGE_NAME} version: ${VERSION} (tarball mode)"

    # Check if versioned image already loaded
    local CURRENT_ID=$(docker images "${IMAGE_NAME}:${VERSION}" --format '{{.ID}}' 2>/dev/null || echo "")
    if [ -n "$CURRENT_ID" ]; then
        echo "✅ ${IMAGE_NAME}:${VERSION} already loaded (ID: $CURRENT_ID) - skipping docker load"
        return 0
    fi

    echo "📦 Loading ${IMAGE_NAME}:${VERSION} from tarball..."
    if docker load -i "$TARBALL" 2>&1 | tee "$LOG_FILE"; then
        # Verify versioned tag exists after load
        if docker images "${IMAGE_NAME}:${VERSION}" --format '{{.ID}}' | grep -q .; then
            echo "✅ ${IMAGE_NAME}:${VERSION} loaded successfully"
        else
            # Tarball may be from before versioning - tag it now
            echo "🏷️  Tagging ${IMAGE_NAME}:latest as ${IMAGE_NAME}:${VERSION}"
            docker tag "${IMAGE_NAME}:latest" "${IMAGE_NAME}:${VERSION}" 2>/dev/null || true
        fi

        echo "📋 Available tags for ${IMAGE_NAME}:"
        docker images "${IMAGE_NAME}" --format '   {{.Repository}}:{{.Tag}} ({{.ID}})'
    else
        echo "⚠️  Failed to load ${IMAGE_NAME} tarball (may be corrupted or out of memory)"
        echo "   Container will continue startup - transfer fresh image with './stack build-${NAME}'"
    fi
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
# CRITICAL: Pull new images BEFORE pruning old ones!
# This preserves shared layers and avoids re-downloading the full image.
# The load_desktop_image function above handles this correctly.
#
# Cleanup logic:
# - Read expected version from .version files
# - Read registry refs from .runtime-ref files (written by load_desktop_image)
# - Keep images matching expected version, :latest, or registry refs
# - Remove all other versions (old image hashes)
# ================================================================================
echo ""
echo "🧹 Cleaning up old desktop images in nested Docker..."

# First, remove ALL stopped containers to allow image removal
# This is safe because Hydra creates fresh containers for each session
# Stopped containers are just leftovers from previous sessions
STOPPED_COUNT=$(docker ps -aq --filter "status=exited" 2>/dev/null | wc -l)
if [ "$STOPPED_COUNT" -gt 0 ]; then
    echo "   Removing $STOPPED_COUNT stopped container(s)..."
    docker container prune -f >/dev/null 2>&1 || true
fi

# Build a list of expected versions and registry refs
declare -A EXPECTED_VERSIONS
declare -A REGISTRY_REFS
DESKTOP_NAMES=""
for version_file in /opt/images/helix-*.version; do
    if [ -f "$version_file" ]; then
        IMAGE_NAME=$(basename "$version_file" .version)
        EXPECTED_VERSIONS[$IMAGE_NAME]=$(cat "$version_file")
        echo "   Expected version for $IMAGE_NAME: ${EXPECTED_VERSIONS[$IMAGE_NAME]}"

        # Check for registry ref (written during registry pull)
        REF_FILE="/opt/images/${IMAGE_NAME}.runtime-ref"
        if [ -f "$REF_FILE" ]; then
            REGISTRY_REFS[$IMAGE_NAME]=$(cat "$REF_FILE")
            echo "   Registry ref for $IMAGE_NAME: ${REGISTRY_REFS[$IMAGE_NAME]}"
        fi

        DESKTOP_NAME="${IMAGE_NAME#helix-}"
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

        # Keep images matching the expected version from .version file OR tagged as :latest
        # Remove everything else (old versions)
        if [ "$IMAGE_TAG" = "$EXPECTED_VERSION" ] || [ "$IMAGE_TAG" = "latest" ]; then
            KEPT_COUNT=$((KEPT_COUNT + 1))
        else
            echo "   Removing old image: $image (expected version: $EXPECTED_VERSION)"
            if docker rmi "$image" 2>/dev/null; then
                REMOVED_COUNT=$((REMOVED_COUNT + 1))
            else
                echo "   ⚠️  Failed to remove $image (may still be in use)"
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

# ================================================================================
# Clean up dangling images and build cache
# This removes:
# - Dangling images (untagged <none> images from failed builds)
# - Build cache (accumulated from docker build operations)
# - Unused networks (orphaned from stopped containers)
# NOTE: We do NOT prune volumes - those contain user data
# ================================================================================
echo ""
echo "🧹 Pruning dangling images and build cache..."

# Remove dangling images first (faster, targeted cleanup)
DANGLING_COUNT=$(docker images -f "dangling=true" -q 2>/dev/null | wc -l)
if [ "$DANGLING_COUNT" -gt 0 ]; then
    echo "   Removing $DANGLING_COUNT dangling image(s)..."
    docker image prune -f >/dev/null 2>&1 || true
fi

# Run system prune to clean build cache and unused networks
# This does NOT remove volumes (no --volumes flag)
PRUNE_OUTPUT=$(docker system prune -f 2>&1) || true
if echo "$PRUNE_OUTPUT" | grep -q "reclaimed"; then
    RECLAIMED=$(echo "$PRUNE_OUTPUT" | grep "reclaimed" | tail -1)
    echo "   $RECLAIMED"
else
    echo "   No additional space to reclaim"
fi

echo "✅ Docker cleanup complete"
