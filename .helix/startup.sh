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

# =========================================
# Rename numbered repos to canonical names
# =========================================
# The API auto-increments repo names (helix-1, zed-2, etc.) when duplicates exist.
# The ./stack script expects ../zed and ../qwen-code to exist.
# We rename to canonical names and create symlinks for API compatibility on restart.

cd ~/work

for pattern in "helix-" "zed-" "qwen-code-"; do
    canonical="${pattern%-}"  # Remove trailing dash (e.g., "helix-" -> "helix")
    for numbered in ${pattern}[0-9]*; do
        [ -d "$numbered" ] || continue
        # Skip if it's already a symlink
        [ -L "$numbered" ] && continue
        if [ ! -e "$canonical" ]; then
            echo "Renaming $numbered → $canonical"
            mv "$numbered" "$canonical"
            ln -s "$canonical" "$numbered"
            echo "  Created symlink $numbered → $canonical"
        else
            echo "Skipping $numbered (canonical $canonical already exists)"
        fi
    done
done

echo ""

# Ensure tmux is installed (needed for ./stack start)
if ! command -v tmux &> /dev/null; then
    echo "Installing tmux..."
    sudo apt-get update && sudo apt-get install -y tmux
fi

# Ensure yarn is installed (needed for frontend development/testing)
if ! command -v yarn &> /dev/null; then
    echo "Installing yarn..."
    sudo npm install -g yarn
fi

# Ensure mockgen is installed (needed for Go mock generation)
# Pin to v0.4.0 to match go.mod dependency
if ! command -v mockgen &> /dev/null; then
    echo "Installing mockgen v0.4.0..."
    go install go.uber.org/mock/mockgen@v0.4.0
fi

# Check for privileged mode (host docker socket)
# NOTE: We do NOT set DOCKER_HOST here. The ./stack script has its own
# Helix-in-Helix detection (detect_helix_in_helix) that properly handles:
# - Running the control plane on inner Docker (Hydra's DinD)
# - Running the sandbox on host Docker via start_outer_sandbox()
if [ -S /var/run/host-docker.sock ]; then
    echo "✓ Privileged mode enabled - host Docker available for sandbox"
    echo "  The ./stack script will handle Helix-in-Helix mode automatically"
else
    echo "⚠ Warning: Privileged mode not enabled"
    echo "  Host Docker not available. Set UseHostDocker=true on the task/session."
    echo "  The inner Helix won't be able to run sandboxes (DinD-in-DinD-in-DinD doesn't work)"
fi

# Helix repo location - project setup clones it to ~/work/helix
HELIX_DIR=~/work/helix

if [ ! -d "$HELIX_DIR" ]; then
    echo "Error: Could not find helix repository at $HELIX_DIR"
    exit 1
fi

# Check that we have the stack script (ensures we're on main branch, not helix-specs)
if [ ! -f "$HELIX_DIR/stack" ]; then
    echo "Error: ./stack script not found in $HELIX_DIR"
    echo "  This usually means the helix repo is on the wrong branch (e.g., helix-specs instead of main)"
    echo "  The helix-specs branch only contains design docs, not code."
    echo ""
    echo "  To fix, run: cd $HELIX_DIR && git checkout main"
    exit 1
fi

echo "Using helix directory: $HELIX_DIR"
cd "$HELIX_DIR"

# Kill any existing tmux session to ensure idempotency
if tmux has-session -t helix 2>/dev/null; then
    echo "Stopping existing helix tmux session..."
    tmux kill-session -t helix
fi

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
