#!/bin/bash
# Helix-in-Helix Development Startup Script
# Builds and starts the Helix development stack
#
# Prerequisites:
# - Session must have UseHostDocker=true (HYDRA_PRIVILEGED_MODE_ENABLED=true)
# - Repos already cloned to ~/work/{helix,zed,qwen-code} by project setup

set -xeuo pipefail

echo "========================================"
echo "  Helix-in-Helix Development Setup"
echo "========================================"
echo ""

# Ensure tmux is installed (needed for ./stack start)
if ! command -v tmux &> /dev/null; then
    echo "Installing tmux..."
    sudo apt-get update && sudo apt-get install -y tmux
fi

# Check for privileged mode (host docker socket)
if [ -S /var/run/host-docker.sock ]; then
    echo "✓ Privileged mode enabled - host Docker available"
    # Use host Docker for running sandboxes
    export DOCKER_HOST=unix:///var/run/host-docker.sock
else
    echo "⚠ Warning: Privileged mode not enabled"
    echo "  Host Docker not available. Set UseHostDocker=true on the task/session."
    echo "  Continuing with inner Docker only..."
fi

# Find the helix repo - check common locations
HELIX_DIR=""
for dir in ~/work/helix ~/code/helix ~/helix-workspace/helix ~/helix; do
    if [ -d "$dir" ]; then
        HELIX_DIR="$dir"
        break
    fi
done

if [ -z "$HELIX_DIR" ]; then
    echo "Error: Could not find helix repository"
    echo "Expected in one of: ~/work/helix, ~/code/helix, ~/helix-workspace/helix, ~/helix"
    exit 1
fi

echo "Using helix directory: $HELIX_DIR"
cd "$HELIX_DIR"

# Build and start the stack
echo ""
echo "Building Helix components..."
echo "  This will build: API, Zed IDE, and Sandbox"
echo ""

# Build everything, then start the stack in tmux
./stack build && \
./stack build-zed release && \
./stack build-sandbox && \
./stack start

echo ""
echo "========================================"
echo "  Helix stack started in tmux!"
echo "========================================"
echo ""
echo "Commands:"
echo "  tmux attach        - View the running stack"
echo "  ./stack stop       - Stop the stack"
echo "  ./stack logs       - View logs"
echo ""
