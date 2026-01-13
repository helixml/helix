#!/bin/bash
# Fast build of gst-pipewire-zerocopy plugin for iterative development
#
# Usage:
#   ./scripts/build-zerocopy-plugin.sh              # Build only
#   ./scripts/build-zerocopy-plugin.sh deploy       # Build + deploy to sandbox
#   ./scripts/build-zerocopy-plugin.sh deploy ses_xxx  # Build + deploy to specific session container
#
# The plugin .so file is built using the same Ubuntu 25.10 base as helix-ubuntu
# to ensure ABI compatibility with the target GStreamer version.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELIX_DIR="$(dirname "$SCRIPT_DIR")"
PLUGIN_DIR="$HELIX_DIR/desktop/gst-pipewire-zerocopy"
OUTPUT_DIR="$HELIX_DIR/sandbox-images"

# Build in container with same base as helix-ubuntu
echo "Building gst-pipewire-zerocopy plugin..."
docker run --rm \
    -v "$PLUGIN_DIR:/build/gst-pipewire-zerocopy" \
    -v "$HELIX_DIR/desktop/wayland-display-core:/build/wayland-display-core" \
    -v "$OUTPUT_DIR:/output" \
    -w /build \
    ubuntu:25.10 bash -c '
        set -e
        export DEBIAN_FRONTEND=noninteractive

        # Install dependencies (minimal for plugin build only)
        apt-get update -qq
        apt-get install -y -qq \
            curl build-essential pkg-config \
            libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev libgstreamer-plugins-bad1.0-dev \
            gstreamer1.0-plugins-base libgstreamer-plugins-bad1.0-0 \
            libpipewire-0.3-dev libspa-0.2-dev \
            libegl1-mesa-dev libdrm-dev libclang-dev \
            libinput-dev libxkbcommon-dev libwayland-dev libudev-dev libgbm-dev >/dev/null

        # libgstcuda symlink for CUDA support (may not exist on AMD, but link anyway)
        ln -sf /usr/lib/x86_64-linux-gnu/libgstcuda-1.0.so.0 /usr/lib/x86_64-linux-gnu/libgstcuda-1.0.so 2>/dev/null || true

        # Install Rust (same version as Dockerfile)
        curl --proto "=https" --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain 1.85.0
        source /root/.cargo/env

        # Build the plugin
        cd /build/gst-pipewire-zerocopy
        cargo build --release 2>&1 | tail -20

        # Copy to output
        cp target/release/libgstpipewirezerocopy.so /output/
        echo ""
        echo "✅ Plugin built: sandbox-images/libgstpipewirezerocopy.so"
        ls -lh /output/libgstpipewirezerocopy.so
    '

echo ""

# Deploy if requested
if [ "$1" = "deploy" ]; then
    SANDBOX_SERVICE="${SANDBOX_SERVICE:-sandbox-nvidia}"
    TARGET_CONTAINER="$2"

    echo "Deploying plugin to sandbox..."

    # Copy to sandbox container
    docker compose cp "$OUTPUT_DIR/libgstpipewirezerocopy.so" "$SANDBOX_SERVICE:/opt/images/"

    if [ -n "$TARGET_CONTAINER" ]; then
        # Deploy to specific desktop container
        echo "Deploying to desktop container: $TARGET_CONTAINER"
        docker compose exec -T "$SANDBOX_SERVICE" docker cp \
            /opt/images/libgstpipewirezerocopy.so \
            "$TARGET_CONTAINER:/usr/lib/x86_64-linux-gnu/gstreamer-1.0/"

        # Restart screenshot-server to reload plugin
        docker compose exec -T "$SANDBOX_SERVICE" docker exec "$TARGET_CONTAINER" \
            pkill -f screenshot-server || true

        echo "✅ Plugin deployed and screenshot-server restarted"
    else
        echo "✅ Plugin copied to sandbox at /opt/images/"
        echo "   Start a new session to use the updated plugin"
    fi
fi
