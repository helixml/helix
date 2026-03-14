#!/bin/bash
# Helix-in-Helix Development Startup Script
# Builds and starts the Helix development stack inside a desktop container.
#
# Docker-in-desktop mode: the desktop container runs its own dockerd with a
# volume-backed /var/lib/docker. The inner Helix stack (API, postgres, sandbox)
# runs on this local dockerd. No host Docker access needed — nesting works
# arbitrarily deep because each level's /var/lib/docker is backed by ext4.
#
# Prerequisites:
# - Repos already cloned to ~/work/{helix,zed,qwen-code} by project setup
#
# REQUIRED OUTER ENVIRONMENT UPDATES:
# For this script to work, the OUTER Helix must have:
# 1. Docker-shim fix for Compose 5.0+ (helix/desktop/docker-shim/)
# 2. Init script that adds retro to docker group (helix/desktop/ubuntu-config/cont-init.d/17-start-dockerd.sh)
# 3. Updated desktop image built and deployed (./stack build-ubuntu)
# Without these, builds will fail with "removed --set flag" or permission errors.

set -xeuo pipefail

# Add Go bin to PATH permanently for tools like swag, mockgen
if ! grep -q 'export PATH=.*go/bin' ~/.bashrc 2>/dev/null; then
    echo '' >> ~/.bashrc
    echo '# Go bin path for swag, mockgen, etc.' >> ~/.bashrc
    echo 'export PATH="$PATH:$HOME/go/bin"' >> ~/.bashrc
    echo "Added ~/go/bin to ~/.bashrc"
fi
# Also set for current script
export PATH="$PATH:$HOME/go/bin"

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

# Check that Docker is available (docker-in-desktop mode)
if sudo docker info &>/dev/null; then
    echo "✓ Docker available (docker-in-desktop mode)"
else
    echo "⚠ Warning: Docker not available"
    echo "  The desktop container's dockerd may not have started yet."
    echo "  Check: sudo docker info"
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
# Create stable hostname for outer API
# =========================================
# The inner compose stack shadows the "api" hostname with its own API service.
# Hydra sets "outer-api:host-gateway" as a Docker ExtraHost on the desktop
# container, which resolves to the compose network gateway (the host machine).
# Since the outer API publishes port 8080 on the host, outer-api:8080 reaches
# the API dynamically — Docker updates iptables DNAT when the API restarts.
#
# DOCKER-IN-DOCKER NETWORKING:
# The inner API container is on a separate Docker network (10.214.x.x) and
# cannot reach the outer API's network (172.18.x.x) directly. The inner
# container CAN reach the desktop via host-gateway (host.docker.internal),
# but port 8080 on the desktop is shadowed by the inner API's published port.
#
# Solution: run a socat proxy on port 8081 on the desktop that forwards to
# outer-api:8080. Inner containers use http://host.docker.internal:8081 to
# reach the outer API without port conflicts.
OUTER_PROXY_PORT=8081

if [[ -n "${HELIX_API_URL:-}" ]]; then
    OUTER_API_IP=$(getent hosts outer-api | awk '{print $1}' | head -1)
    if [[ -n "$OUTER_API_IP" ]]; then
        echo "outer-api → $OUTER_API_IP (via host-gateway, survives API restarts)"
        export HELIX_API_URL="http://outer-api:8080"
        export OUTER_API_IP

        # Start socat proxy for Docker-in-Docker networking
        # Inner containers can't reach outer-api:8080 directly (different Docker network)
        # and host-gateway:8080 is shadowed by the inner API's published port.
        # This proxy on port 8081 bridges the gap.
        if ! pgrep -f "socat.*TCP-LISTEN:${OUTER_PROXY_PORT}" > /dev/null 2>&1; then
            echo "  Starting outer API proxy on port ${OUTER_PROXY_PORT}..."
            nohup socat TCP-LISTEN:${OUTER_PROXY_PORT},fork,reuseaddr TCP:outer-api:8080 > /dev/null 2>&1 &
            echo "  ✅ Proxy: host.docker.internal:${OUTER_PROXY_PORT} → outer-api:8080"
        else
            echo "  ✅ Outer API proxy already running on port ${OUTER_PROXY_PORT}"
        fi
    else
        echo "ERROR: outer-api hostname not set. Rebuild sandbox with latest code."
        echo "  Hydra should set outer-api:host-gateway as a Docker ExtraHost."
    fi
fi

# Create/update .env file for inner Helix
# =========================================
# Route inference through the outer Helix using the same API key the agent uses.
# This is "meta" - the inner Helix uses the outer Helix for LLM inference.
#
# ALWAYS update API keys, even if .env exists — on session restart the outer
# Helix issues a new API key but the old .env persists with the stale one,
# breaking all LLM calls with "error getting API key: not found".
echo ""

# Get the outer Helix URL and API key from environment
OUTER_API_URL="${HELIX_API_URL:-}"
OUTER_API_KEY="${USER_API_TOKEN:-${HELIX_API_KEY:-}}"

if [[ -n "$OUTER_API_URL" && -n "$OUTER_API_KEY" ]]; then
    # Inner containers use host.docker.internal:PROXY_PORT to reach outer API
    # via the socat proxy (avoids port 8080 shadowing in Docker-in-Docker)
    INNER_OUTER_URL="http://host.docker.internal:${OUTER_PROXY_PORT}"

    if [ -f "$HELIX_DIR/.env" ]; then
        # .env exists — update API keys in-place, preserve everything else
        # (user may have added custom env vars like FRONTEND_URL)
        OLD_KEY=$(grep '^OPENAI_API_KEY=' "$HELIX_DIR/.env" | cut -d= -f2-)
        if [[ "$OLD_KEY" != "$OUTER_API_KEY" ]]; then
            echo "  🔑 API key changed (session restart detected), updating .env..."
            sed -i "s|^OPENAI_API_KEY=.*|OPENAI_API_KEY=${OUTER_API_KEY}|" "$HELIX_DIR/.env"
            sed -i "s|^ANTHROPIC_API_KEY=.*|ANTHROPIC_API_KEY=${OUTER_API_KEY}|" "$HELIX_DIR/.env"
            # Also update LICENSE_KEY in case it changed
            sed -i "s|^LICENSE_KEY=.*|LICENSE_KEY=${LICENSE_KEY:-}|" "$HELIX_DIR/.env"
            echo "  ✅ API keys updated in .env"
        else
            echo "  ✅ .env exists, API key unchanged"
        fi
    else
        echo "Creating .env file for inner Helix..."
        echo "  📡 Routing inference through outer Helix: $OUTER_API_URL"

        cat > "$HELIX_DIR/.env" << EOF
# Generated by Helix-in-Helix startup script
# Routes inference through outer Helix (meta!)
#
# Inner containers reach the outer API via a socat proxy on port ${OUTER_PROXY_PORT}
# running on the desktop container. This avoids the port 8080 shadowing issue
# where the inner API's published port intercepts traffic meant for the outer API.

# Use outer Helix as OpenAI-compatible provider
OPENAI_API_KEY=${OUTER_API_KEY}
OPENAI_BASE_URL=${INNER_OUTER_URL}/v1

# Use outer Helix as Anthropic-compatible provider
# Note: No /v1 suffix - Anthropic SDK appends /v1/messages
ANTHROPIC_API_KEY=${OUTER_API_KEY}
ANTHROPIC_BASE_URL=${INNER_OUTER_URL}

# Use the outer Helix's license (hides "Get your free Community License Key" banner)
# LICENSE_KEY is passed from the outer Helix via HydraExecutor
LICENSE_KEY=${LICENSE_KEY:-}
EOF
        echo "  ✅ .env created - inner Helix will use outer Helix for inference"
    fi
else
    echo "  ⚠️  No outer Helix context (HELIX_API_URL/USER_API_TOKEN not set)"
    if [ ! -f "$HELIX_DIR/.env" ]; then
        echo "     You'll need to create .env manually with your LLM provider"
    fi
fi

# Kill any existing tmux session to ensure idempotency
if tmux has-session -t helix 2>/dev/null; then
    echo "Stopping existing helix tmux session..."
    tmux kill-session -t helix
fi

# Ensure buildx directory has correct permissions
# The docker-shim tries to create the helix-shared builder and needs write access
mkdir -p ~/.docker/buildx
sudo chown -R retro:retro ~/.docker 2>/dev/null || true

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
