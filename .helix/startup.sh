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

# Note: Rust is NOT needed on the host - Zed builds inside a Docker container
# (zed-builder:ubuntu25) which has its own Rust installation.

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

# =========================================
# =========================================
# Create stable hostname for outer API
# =========================================
# The inner compose stack shadows the "api" hostname with its own API service.
# Resolve the outer API IP now and create an "outer-api" /etc/hosts entry that
# survives the inner stack startup. Also used by Zed for LLM access.
if [[ -n "${HELIX_API_URL:-}" ]]; then
    OUTER_API_IP=$(getent hosts api | awk '{print $1}' | head -1)
    if [[ -n "$OUTER_API_IP" ]]; then
        if ! grep -q "outer-api" /etc/hosts 2>/dev/null; then
            echo "$OUTER_API_IP outer-api" | sudo tee -a /etc/hosts >/dev/null
        fi
        echo "✅ outer-api → $OUTER_API_IP (preserves access after inner stack starts)"
        export HELIX_API_URL="http://outer-api:8080"
        export OUTER_API_IP
    fi
fi

# Create .env file for inner Helix
# =========================================
# Route inference through the outer Helix using the same API key the agent uses.
# This is "meta" - the inner Helix uses the outer Helix for LLM inference.
if [ ! -f "$HELIX_DIR/.env" ]; then
    echo ""
    echo "Creating .env file for inner Helix..."

    # Get the outer Helix URL and API key from environment
    OUTER_API_URL="${HELIX_API_URL:-}"
    OUTER_API_KEY="${USER_API_TOKEN:-${HELIX_API_KEY:-}}"

    if [[ -n "$OUTER_API_URL" && -n "$OUTER_API_KEY" ]]; then
        echo "  📡 Routing inference through outer Helix: $OUTER_API_URL"
        cat > "$HELIX_DIR/.env" << EOF
# Generated by Helix-in-Helix startup script
# Routes inference through outer Helix (meta!)

# Use outer Helix as OpenAI-compatible provider
OPENAI_API_KEY=${OUTER_API_KEY}
OPENAI_BASE_URL=${OUTER_API_URL}/v1
EOF
        echo "  ✅ .env created - inner Helix will use outer Helix for inference"
    else
        echo "  ⚠️  No outer Helix context (HELIX_API_URL/USER_API_TOKEN not set)"
        echo "     You'll need to create .env manually with your LLM provider"
    fi
else
    echo "  ℹ️  .env already exists, skipping creation"
fi

# Kill any existing tmux session to ensure idempotency
if tmux has-session -t helix 2>/dev/null; then
    echo "Stopping existing helix tmux session..."
    tmux kill-session -t helix
fi

# =========================================
# Bypass docker-shim for builds
# =========================================
# The Helix desktop environment uses a docker-shim that requires a shared BuildKit
# server (helix-buildkit at tcp://10.213.0.2:1234). This server may not be running
# or reachable in all environments. We bypass the shim by using docker.real directly.
if [ -x /usr/bin/docker.real ]; then
    echo "📦 Using docker.real to bypass docker-shim for builds"

    # Unset BUILDKIT_HOST to prevent remote BuildKit usage
    unset BUILDKIT_HOST

    # Remove the helix-shared buildx instance that points to unreachable server
    rm -f ~/.docker/buildx/instances/helix-shared 2>/dev/null

    # Create a temporary directory with symlinks to docker.real
    mkdir -p /tmp/docker-bypass
    ln -sf /usr/bin/docker.real /tmp/docker-bypass/docker
    ln -sf /usr/bin/docker.real /tmp/docker-bypass/docker-buildx
    export PATH="/tmp/docker-bypass:$PATH"

    # Ensure default buildx builder is used
    /usr/bin/docker.real buildx use default 2>/dev/null || true
fi

# Build and start the stack
echo ""
echo "Building Helix components..."
echo "  This will build: API, Zed IDE, and Sandbox"
echo ""

# Build API and frontend first
echo "Step 1/4: Building API and frontend..."
if ! ./stack build; then
    echo "❌ Error: Failed to build API/frontend"
    exit 1
fi

# Build Zed IDE
echo "Step 2/4: Building Zed IDE..."
if ! ./stack build-zed release; then
    echo "❌ Error: Failed to build Zed IDE"
    exit 1
fi

# Build sandbox
echo "Step 3/4: Building sandbox..."
if ! ./stack build-sandbox; then
    echo "❌ Error: Failed to build sandbox"
    exit 1
fi

# Start the stack in tmux
echo "Step 4/4: Starting the stack..."
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
