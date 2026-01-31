#!/bin/bash
# Helix-in-Helix Development Setup Script
# This script configures a Helix desktop for developing Helix itself
#
# Architecture:
# - Inner Docker (default): Hydra's DinD at /var/run/docker.sock
#   → Run the Helix control plane here
# - Outer Docker (host): Available via /var/run/host-docker.sock when privileged mode is enabled
#   → Run test sandboxes here
# - Service Exposure: The inner control plane is exposed via the API's proxy endpoint
#   → Sandboxes on host Docker connect to the exposed URL
#
# Usage: Run this script inside a Helix desktop session with privileged mode enabled
#   ~/helix-dev-setup.sh

set -e

WORKSPACE="${HOME}/helix-workspace"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}  Helix-in-Helix Development Setup   ${NC}"
echo -e "${BLUE}======================================${NC}"
echo ""

# Check if we're running inside a Helix desktop
if [ ! -S /var/run/docker.sock ]; then
    echo -e "${RED}Error: Docker socket not found. Are you running inside a Helix desktop?${NC}"
    exit 1
fi

# Check for privileged mode (host docker socket)
if [ -S /var/run/host-docker.sock ]; then
    echo -e "${GREEN}✓ Privileged mode enabled - host Docker available${NC}"
    HAS_HOST_DOCKER=true
else
    echo -e "${YELLOW}⚠ Privileged mode not enabled - host Docker not available${NC}"
    echo -e "${YELLOW}  To enable: Set HYDRA_PRIVILEGED_MODE_ENABLED=true on the sandbox${NC}"
    HAS_HOST_DOCKER=false
fi

echo ""
echo -e "${GREEN}1. Setting up workspace...${NC}"
mkdir -p "$WORKSPACE"
cd "$WORKSPACE"

# Clone repositories in parallel
echo -e "${GREEN}2. Cloning repositories (in parallel)...${NC}"

# Function to clone or update a repo
clone_or_update() {
    local name="$1"
    local url="$2"
    local branch="${3:-main}"

    if [ ! -d "$name" ]; then
        echo "   Cloning $name..."
        git clone --branch "$branch" "$url" "$name" 2>&1 | sed "s/^/   [$name] /"
        echo -e "   ${GREEN}✓ $name cloned${NC}"
    else
        echo "   $name already exists, pulling latest..."
        (cd "$name" && git pull --ff-only 2>/dev/null || true) | sed "s/^/   [$name] /"
        echo -e "   ${GREEN}✓ $name updated${NC}"
    fi
}

# Clone all repos in parallel
clone_or_update "helix" "https://github.com/helixml/helix.git" "main" &
PID_HELIX=$!

clone_or_update "zed" "https://github.com/helixml/zed.git" "helix" &
PID_ZED=$!

clone_or_update "qwen-code" "https://github.com/helixml/qwen-code.git" "main" &
PID_QWEN=$!

# Wait for all clones to complete
echo "   Waiting for all repositories to finish..."
wait $PID_HELIX $PID_ZED $PID_QWEN
echo -e "   ${GREEN}✓ All repositories ready${NC}"

echo -e "${GREEN}3. Configuring Docker endpoints...${NC}"

# Set up environment variables for two Docker endpoints
cat > "$WORKSPACE/.helix-dev-env" << 'EOF'
# Helix-in-Helix Development Environment
# Source this file: source ~/.helix-dev-env

# Inner Docker (Hydra's DinD) - for running the control plane
export DOCKER_HOST_INNER="unix:///var/run/docker.sock"

# Outer Docker (Host Docker via privileged mode) - for running sandboxes
# Only available when HYDRA_PRIVILEGED_MODE_ENABLED=true on the sandbox
export DOCKER_HOST_OUTER="unix:///var/run/host-docker.sock"

# Helper functions
helix-inner() {
    DOCKER_HOST="$DOCKER_HOST_INNER" "$@"
}

helix-outer() {
    DOCKER_HOST="$DOCKER_HOST_OUTER" "$@"
}

# Aliases for convenience
alias docker-inner='DOCKER_HOST=$DOCKER_HOST_INNER docker'
alias docker-outer='DOCKER_HOST=$DOCKER_HOST_OUTER docker'
alias compose-inner='DOCKER_HOST=$DOCKER_HOST_INNER docker compose'
alias compose-outer='DOCKER_HOST=$DOCKER_HOST_OUTER docker compose'

echo "Helix-in-Helix environment loaded:"
echo "  docker-inner: Control plane Docker (default)"
echo "  docker-outer: Host Docker for sandboxes"
EOF

# Add to shell rc if not already there
for rc_file in ~/.bashrc ~/.zshrc; do
    if [ -f "$rc_file" ] && ! grep -q "source.*helix-dev-env" "$rc_file" 2>/dev/null; then
        echo "" >> "$rc_file"
        echo "# Helix-in-Helix development environment" >> "$rc_file"
        echo "[ -f \"$WORKSPACE/.helix-dev-env\" ] && source \"$WORKSPACE/.helix-dev-env\"" >> "$rc_file"
    fi
done

echo -e "${GREEN}4. Creating helper scripts...${NC}"

# Create helper script to start the inner control plane
cat > "$WORKSPACE/start-inner-stack.sh" << 'EOF'
#!/bin/bash
# Start the Helix control plane on inner Docker (Hydra's DinD)
set -e
cd ~/helix-workspace/helix

# Use inner Docker (default)
export DOCKER_HOST=unix:///var/run/docker.sock

echo "Starting Helix control plane on inner Docker..."
./stack start

echo ""
echo "Control plane started!"
echo ""
echo "Next steps:"
echo "1. Wait for the API to be ready: curl http://localhost:8080/health"
echo "2. Expose the API port: ./expose-inner-api.sh"
echo "3. Start a test sandbox: ./start-outer-sandbox.sh <exposed-url>"
EOF
chmod +x "$WORKSPACE/start-inner-stack.sh"

# Create helper script to expose the inner API
cat > "$WORKSPACE/expose-inner-api.sh" << 'EOF'
#!/bin/bash
# Expose the inner control plane's API port to the outside world
# This allows sandboxes running on host Docker to connect to the inner API
set -e

PORT="${1:-8080}"

# Get session ID from environment or prompt
SESSION_ID="${SESSION_ID:-${HELIX_SESSION_ID:-}}"
if [ -z "$SESSION_ID" ]; then
    echo "Error: SESSION_ID not set"
    echo ""
    echo "Set it with: export SESSION_ID=ses_xxx"
    echo "You can find your session ID in the Helix UI URL"
    exit 1
fi

# Get API credentials
HELIX_API_URL="${HELIX_API_URL:-}"
HELIX_API_KEY="${HELIX_API_KEY:-}"

if [ -z "$HELIX_API_URL" ] || [ -z "$HELIX_API_KEY" ]; then
    echo "Error: HELIX_API_URL and HELIX_API_KEY must be set"
    echo ""
    echo "These should be set automatically in your desktop environment."
    echo "If not, get them from your Helix account settings."
    exit 1
fi

echo "Exposing port $PORT for session $SESSION_ID..."
echo ""

RESPONSE=$(curl -s -X POST "$HELIX_API_URL/api/v1/sessions/$SESSION_ID/expose" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $HELIX_API_KEY" \
    -d "{\"port\": $PORT, \"protocol\": \"http\", \"name\": \"dev-api\"}")

echo "$RESPONSE" | jq .

# Extract the URL
EXPOSED_URL=$(echo "$RESPONSE" | jq -r '.urls[0]')

if [ "$EXPOSED_URL" != "null" ] && [ -n "$EXPOSED_URL" ]; then
    echo ""
    echo "Inner API exposed at: $EXPOSED_URL"
    echo ""
    echo "Use this URL when starting sandboxes on host Docker:"
    echo "  ./start-outer-sandbox.sh $EXPOSED_URL"

    # Save for convenience
    echo "$EXPOSED_URL" > "$HOME/helix-workspace/.inner-api-url"
    echo "(Saved to ~/.helix-workspace/.inner-api-url)"
fi
EOF
chmod +x "$WORKSPACE/expose-inner-api.sh"

# Create helper script to start a sandbox on outer Docker
cat > "$WORKSPACE/start-outer-sandbox.sh" << 'EOF'
#!/bin/bash
# Start a sandbox on outer/host Docker for testing
# The sandbox will connect to the inner control plane via the exposed URL
set -e

# Get inner API URL from argument or saved file
INNER_API_URL="${1:-}"
if [ -z "$INNER_API_URL" ] && [ -f "$HOME/helix-workspace/.inner-api-url" ]; then
    INNER_API_URL=$(cat "$HOME/helix-workspace/.inner-api-url")
fi

if [ -z "$INNER_API_URL" ]; then
    echo "Usage: ./start-outer-sandbox.sh <inner-api-url>"
    echo ""
    echo "First expose the inner API: ./expose-inner-api.sh"
    exit 1
fi

# Check for host docker socket
if [ ! -S /var/run/host-docker.sock ]; then
    echo "Error: Host Docker socket not available"
    echo "Make sure HYDRA_PRIVILEGED_MODE_ENABLED=true is set on the sandbox"
    exit 1
fi

# Generate unique sandbox name
SANDBOX_NAME="helix-sandbox-dev-$(whoami)-$(date +%s)"

echo "Starting sandbox '$SANDBOX_NAME' on host Docker"
echo "Connecting to inner API at: $INNER_API_URL"
echo ""

# Use outer Docker (host)
export DOCKER_HOST=unix:///var/run/host-docker.sock

# Start sandbox
docker run -d \
    --name "$SANDBOX_NAME" \
    --privileged \
    -e HELIX_API_URL="$INNER_API_URL" \
    -e RUNNER_TOKEN="${RUNNER_TOKEN:-oh-hallo-insecure-token}" \
    -e SANDBOX_INSTANCE_ID="$SANDBOX_NAME" \
    -e HYDRA_ENABLED=true \
    -v /var/run/docker.sock:/var/run/docker.sock \
    helix-sandbox:latest

echo ""
echo "Sandbox started: $SANDBOX_NAME"
echo ""
echo "To view logs:"
echo "  docker-outer logs -f $SANDBOX_NAME"
echo ""
echo "To stop:"
echo "  docker-outer stop $SANDBOX_NAME && docker-outer rm $SANDBOX_NAME"
EOF
chmod +x "$WORKSPACE/start-outer-sandbox.sh"

echo -e "${GREEN}5. Verifying Docker access...${NC}"

echo "   Inner Docker (control plane):"
if docker info > /dev/null 2>&1; then
    echo -e "   ${GREEN}✓ Connected${NC}"
else
    echo -e "   ${RED}✗ Cannot connect${NC}"
fi

if [ "$HAS_HOST_DOCKER" = true ]; then
    echo "   Outer Docker (host via privileged mode):"
    if DOCKER_HOST=unix:///var/run/host-docker.sock docker info > /dev/null 2>&1; then
        echo -e "   ${GREEN}✓ Connected${NC}"
    else
        echo -e "   ${RED}✗ Cannot connect${NC}"
    fi
fi

echo ""
echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}  Setup Complete!                     ${NC}"
echo -e "${BLUE}======================================${NC}"
echo ""
echo "Workspace: $WORKSPACE"
echo ""
echo "Quick Start:"
echo "  1. Source the environment: source ~/.helix-dev-env"
echo "  2. Start inner control plane: ./start-inner-stack.sh"
echo "  3. Expose the inner API: ./expose-inner-api.sh"
echo "  4. Start outer sandbox: ./start-outer-sandbox.sh"
echo ""
echo "Docker commands:"
echo "  docker-inner ps           - List containers on inner Docker"
echo "  docker-outer ps           - List containers on host Docker"
echo ""
